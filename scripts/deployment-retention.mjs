import { execFileSync } from 'node:child_process';
import { lstatSync, readdirSync, readFileSync, realpathSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const projectPattern = /^dvn-test-\d{13}$/;

/** dockerOutput 執行有界 Docker 命令；錯誤必須上拋，不能把查詢失敗當成沒有資源。 */
export function dockerOutput(args) {
  return execFileSync('docker', args, { encoding: 'utf8', timeout: 30000, maxBuffer: 8 * 1024 * 1024 }).trim();
}

function lines(value) { return value.split(/\r?\n/).filter(Boolean); }
function samePath(left, right) { return path.resolve(left) === path.resolve(right); }

/** inspectTestFolder 核對 root 下的單一測試目錄，拒絕連結、掛載跨界與非預期產物。 */
export function inspectTestFolder(root, name) {
  if (!projectPattern.test(name)) throw new Error('不是部署測試專案名稱');
  const workspace = realpathSync(root);
  const base = path.join(workspace, 'data', 'deployment-tests');
  if (!samePath(realpathSync(base), base)) throw new Error('測試資料根目錄不可是連結');
  const directory = path.join(base, name);
  const stat = lstatSync(directory);
  if (!stat.isDirectory() || stat.isSymbolicLink() || !samePath(realpathSync(directory), directory)) throw new Error('測試目錄不可是連結');
  const files = [];
  const visit = current => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const absolute = path.join(current, entry.name);
      const info = lstatSync(absolute);
      if (info.isSymbolicLink() || info.dev !== stat.dev || !samePath(realpathSync(absolute), absolute)) throw new Error(`連結或跨磁碟項目：${absolute}`);
      if (info.isDirectory()) {
        if (absolute !== path.join(directory, 'media')) throw new Error(`未知子目錄：${absolute}`);
        visit(absolute);
      } else if (info.isFile()) {
        const relative = path.relative(directory, absolute);
        const expected = ['video-node.json', 'test.env', 'result.json', 'run.json'].includes(relative)
          || /^media[\\/][^\\/]+\.(mp4|offline)$/.test(relative);
        if (!expected) throw new Error(`非預期檔案：${absolute}`);
        files.push({ path: relative, size: info.size, modified: info.mtimeMs, inode: String(info.ino) });
      } else throw new Error(`非一般檔案：${absolute}`);
    }
  };
  visit(directory);
  const manifestPath = path.join(directory, files.some(file => file.path === 'run.json') ? 'run.json' : 'result.json');
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
  if (manifest.project !== name) throw new Error('測試紀錄的專案不符');
  if (manifest.kind && (manifest.kind !== 'dvn-deployment-test' || !samePath(manifest.workspace, workspace))) throw new Error('測試標記不屬於目前工作區');
  const config = JSON.parse(readFileSync(path.join(directory, 'video-node.json'), 'utf8'));
  if (config.dataDirectory !== '/data/acceptance') throw new Error('不是獨立驗收資料配置');
  const environment = readFileSync(path.join(directory, 'test.env'), 'utf8');
  const envValue = key => environment.split(/\r?\n/).find(line => line.startsWith(key + '='))?.slice(key.length + 1);
  if (!samePath(envValue('DVN_MEDIA_PATH') || '', path.join(directory, 'media')) || !samePath(envValue('DVN_NODE_CONFIG') || '', path.join(directory, 'video-node.json'))) throw new Error('測試掛載路徑不符');
  let active = false;
  if (manifest.status === 'running') {
    if (manifest.host !== os.hostname() || !Number.isInteger(manifest.pid) || manifest.pid < 1) active = true;
    else {
      try { process.kill(manifest.pid, 0); active = true; }
      catch (error) { if (error.code !== 'ESRCH') active = true; }
    }
  }
  files.sort((left, right) => left.path.localeCompare(right.path));
  return { project: name, directory, timestamp: Number(name.slice(9)), files, active };
}

/** testResources 只讀取 dvn-test 資源，並保留完整 inspect 資料供來源與共用關係核對。 */
export function testResources(docker = dockerOutput) {
  const containers = lines(docker(['ps', '-a', '--format', '{{.ID}} {{.Names}} {{.Label "com.docker.compose.project"}}']))
    .map(line => line.split(' ')).filter(([, name, project]) => projectPattern.test(project || '') || /^dvn-test-\d{13}(?:-|$)/.test(name)).map(([id]) => id);
  const volumes = lines(docker(['volume', 'ls', '--format', '{{.Name}}'])).filter(name => /^dvn-test-\d{13}_node-data$/.test(name));
  const networks = lines(docker(['network', 'ls', '--format', '{{.Name}}'])).filter(name => /^dvn-test-\d{13}_default$/.test(name));
  return {
    containers: containers.length ? JSON.parse(docker(['container', 'inspect', ...containers])) : [],
    volumes: volumes.length ? JSON.parse(docker(['volume', 'inspect', ...volumes])) : [],
    networks: networks.length ? JSON.parse(docker(['network', 'inspect', ...networks])) : [],
  };
}

/** ownedResources 拒絕執行中、外部共用或不屬於本工作區的 Docker 資源。 */
export function ownedResources(root, candidate, inventory, docker = dockerOutput) {
  const project = candidate.project;
  const containers = inventory.containers.filter(item => item.Config.Labels?.['com.docker.compose.project'] === project || item.Name?.startsWith('/' + project + '-'));
  const volumes = inventory.volumes.filter(item => item.Labels?.['com.docker.compose.project'] === project || item.Name === project + '_node-data');
  const networks = inventory.networks.filter(item => item.Labels?.['com.docker.compose.project'] === project || item.Name === project + '_default');
  const ids = new Set(containers.map(item => item.Id));
  for (const item of containers) {
    const labels = item.Config.Labels;
    if (!['exited', 'created'].includes(item.State.Status)) throw new Error('容器仍在執行或狀態不明');
    if (labels['com.docker.compose.project'] !== project || labels['com.docker.compose.service'] !== 'video-node'
      || !samePath(labels['com.docker.compose.project.working_dir'] || '', root)
      || !samePath(labels['com.docker.compose.project.config_files'] || '', path.join(root, 'compose.yaml'))
      || !samePath(labels['com.docker.compose.project.environment_file'] || '', path.join(candidate.directory, 'test.env'))) throw new Error('容器來源不符');
    for (const mount of item.Mounts || []) {
      if (mount.Type === 'volume' && mount.Name !== project + '_node-data') throw new Error('容器掛載未知資料卷');
      if (mount.Type !== 'volume' && (mount.Type !== 'bind' || !['/media', '/app/config.json'].includes(mount.Destination))) throw new Error('容器掛載未知來源');
    }
  }
  for (const volume of volumes) {
    if (volume.Name !== project + '_node-data' || volume.Labels?.['com.docker.compose.project'] !== project || volume.Labels['com.docker.compose.volume'] !== 'node-data' || volume.Driver !== 'local' || (volume.Options && Object.keys(volume.Options).length)) throw new Error('資料卷來源不符');
    const users = lines(docker(['ps', '-aq', '--filter', 'volume=' + volume.Name]));
    if (users.some(id => ![...ids].some(owned => owned.startsWith(id)))) throw new Error('資料卷被其他容器共用');
  }
  for (const network of networks) {
    if (network.Name !== project + '_default' || network.Labels?.['com.docker.compose.project'] !== project || network.Labels['com.docker.compose.network'] !== 'default') throw new Error('網路來源不符');
    if (Object.keys(network.Containers || {}).some(id => !ids.has(id))) throw new Error('網路被其他容器共用');
  }
  return { containers: containers.map(item => item.Id), volumes: volumes.map(item => item.Name), networks: networks.map(item => item.Id) };
}

/** retentionPlan 由新至舊保留 keep 組；活動或來源不明的項目跳過並另外列明。 */
export function retentionPlan(root, keep, inventory, docker = dockerOutput) {
  if (!Number.isInteger(keep) || keep < 1) throw new Error('keep 必須是正整數');
  const base = path.join(realpathSync(root), 'data', 'deployment-tests');
  let entries;
  try { entries = readdirSync(base, { withFileTypes: true }); }
  catch (error) { if (error.code === 'ENOENT') return { retained: [], remove: [], skipped: [] }; throw error; }
  const candidates = [], skipped = [];
  for (const entry of entries) {
    if (!projectPattern.test(entry.name)) continue;
    try {
      const candidate = inspectTestFolder(root, entry.name);
      const resources = ownedResources(root, candidate, inventory, docker);
      if (candidate.active) throw new Error('測試程序仍在執行');
      candidates.push({ ...candidate, resources });
    } catch (error) { skipped.push({ project: entry.name, reason: error.message }); }
  }
  candidates.sort((left, right) => right.timestamp - left.timestamp);
  const known = new Set(entries.map(entry => entry.name));
  for (const project of new Set([
    ...inventory.containers.map(item => item.Config.Labels?.['com.docker.compose.project']),
    ...inventory.volumes.map(item => item.Labels?.['com.docker.compose.project']),
    ...inventory.networks.map(item => item.Labels?.['com.docker.compose.project']),
  ])) {
    if (projectPattern.test(project || '') && !known.has(project)) skipped.push({ project, reason: '沒有對應測試目錄，無法確認來源，保留 Docker 資源' });
  }
  return { retained: candidates.slice(0, keep), remove: candidates.slice(keep), skipped };
}
