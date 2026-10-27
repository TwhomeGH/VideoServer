import { lstatSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { dockerOutput, inspectTestFolder, ownedResources, retentionPlan, testResources } from './deployment-retention.mjs';

const repository = fileURLToPath(new URL('..', import.meta.url));

/** recycleFolder 只使用系統回收站；工具失敗或來源仍存在時回報錯誤，不降級為永久刪除。 */
async function recycleFolder(directory) {
  const { default: trash } = await import('trash');
  await trash(directory, { glob: false });
  try { lstatSync(directory); }
  catch (error) { if (error.code === 'ENOENT') return; throw error; }
  throw new Error('回收操作未完成，原始目錄仍存在');
}

/** cleanupDeploymentTests 預設只預覽；apply 時按政策處理超額的已停止測試並留下逐項紀錄。
 * @param {object} options - root 指定工作區；docker／recycle 可注入隔離的測試替身。
 * @returns {Promise<object>} 保留、略過、處理結果與執行紀錄路徑。
 */
export async function cleanupDeploymentTests({ root = repository, apply = false, allowDockerRemoval, hostDataMode, docker = dockerOutput, recycle = recycleFolder, removeHost = directory => rmSync(directory, { recursive: true, force: false }) } = {}) {
  const policy = JSON.parse(readFileSync(path.join(root, 'test/deployment-retention.json'), 'utf8'));
  if (typeof policy.allowDockerRemoval !== 'boolean') throw new Error('清理政策格式無效');
  const permanent = allowDockerRemoval ?? policy.allowDockerRemoval;
  const hostMode = hostDataMode ?? policy.hostDataMode ?? 'recycle';
  if (typeof permanent !== 'boolean' || !['recycle', 'permanent'].includes(hostMode)) throw new Error('清理政策格式無效');
  const plan = retentionPlan(root, policy.keep, testResources(docker), docker);
  const report = { createdAt: new Date().toISOString(), keep: policy.keep, allowDockerRemoval: permanent, hostDataMode: hostMode, retained: plan.retained.map(item => item.project), skipped: plan.skipped, items: plan.remove.map(item => ({ project: item.project, directory: item.directory, files: item.files.length, bytes: item.files.reduce((sum, file) => sum + file.size, 0), resources: item.resources, status: 'planned' })) };
  for (const item of report.items) console.log(`[retention] ${item.project}：${item.directory}；${item.files} 個檔案，容器 ${item.resources.containers.length}／網路 ${item.resources.networks.length}／資料卷 ${item.resources.volumes.length}`);
  for (const item of report.skipped) console.warn(`[retention] 保留 ${item.project}：${item.reason}`);
  console.log(`[retention] 保留最近 ${policy.keep} 組；超額 ${report.items.length} 組${apply ? '，開始按政策處理' : '（唯讀預覽）'}`);
  if (!apply || report.items.length === 0) return report;
  console.log(`[retention] Docker 永久移除：${permanent ? '允許' : '停用'}；本機目錄：${hostMode === 'permanent' ? '明確允許永久刪除' : '系統回收站（失敗即停止）'}`);

  const logDirectory = path.join(root, 'data', 'deployment-retention');
  mkdirSync(logDirectory, { recursive: true });
  report.record = path.join(logDirectory, 'last-run.json');
  const save = () => writeFileSync(report.record, JSON.stringify(report, null, 2));
  save();
  for (let index = 0; index < plan.remove.length; index++) {
    const candidate = plan.remove[index], item = report.items[index];
    item.steps = [];
    try {
      const current = inspectTestFolder(root, candidate.project);
      if (current.active || JSON.stringify(current.files) !== JSON.stringify(candidate.files)) throw new Error('預覽後測試資料已改變，保留原資料');
      const resources = ownedResources(root, current, testResources(docker), docker);
      if (JSON.stringify(resources) !== JSON.stringify(candidate.resources)) throw new Error('預覽後 Docker 資源已改變，保留原資料');
      if (!permanent && Object.values(resources).some(values => values.length)) {
        item.status = 'blocked'; item.error = 'Docker 容器／網路／資料卷需明確允許永久移除；本機資料一併保留';
        console.warn(`[retention] 保留 ${item.project}：${item.error}`); save(); continue;
      }
      // 不使用 force、prune 或 compose down，讓 Docker 拒絕新啟動或被其他容器共用的資源。
      for (const [kind, ids] of [['container', resources.containers], ['network', resources.networks], ['volume', resources.volumes]]) {
        if (!ids.length) continue;
        docker([kind, 'rm', ...ids]);
        item.steps.push({ action: kind + '-removed', targets: ids }); save();
      }
      const after = ownedResources(root, current, testResources(docker), docker);
      if (Object.values(after).some(values => values.length)) throw new Error('Docker 資源仍存在，保留本機資料');
      if (JSON.stringify(inspectTestFolder(root, candidate.project).files) !== JSON.stringify(candidate.files)) throw new Error('回收前資料已變更，保留本機資料');
      if (hostMode === 'permanent') {
        await removeHost(candidate.directory);
        try { lstatSync(candidate.directory); throw new Error('目錄仍存在'); }
        catch (error) { if (error.code !== 'ENOENT') throw error; }
        item.status = 'removed'; item.steps.push({ action: 'permanent-host-removal', target: candidate.directory });
        console.log(`[retention] 已永久移除 ${item.project}`);
      } else {
        await recycle(candidate.directory);
        item.status = 'recycled'; item.steps.push({ action: 'system-trash', target: candidate.directory });
        console.log(`[retention] 已回收 ${item.project}`);
      }
    } catch (error) {
      item.status = 'failed'; item.error = error.message;
      console.error(`[retention] 未完成 ${item.project}：${error.message}`);
      save(); break;
    }
    save();
  }
  console.log(`[retention] 執行紀錄：${report.record}`);
  return report;
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const allowed = new Set(['--apply', '--allow-docker-removal', '--permanently-delete-host-data']);
  if (process.argv.slice(2).some(argument => !allowed.has(argument))) throw new Error('用法：npm run test:cleanup -- [--apply] [--allow-docker-removal] [--permanently-delete-host-data]');
  const report = await cleanupDeploymentTests({ apply: process.argv.includes('--apply'), allowDockerRemoval: process.argv.includes('--allow-docker-removal') ? true : undefined, hostDataMode: process.argv.includes('--permanently-delete-host-data') ? 'permanent' : undefined });
  if (report.items.some(item => ['blocked', 'failed'].includes(item.status))) process.exitCode = 1;
}
