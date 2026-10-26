import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildInputDigest, newestBuildInput } from './build-inputs.mjs';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
const asJson = process.argv.includes('--json');
const project = process.env.DVN_TEST_PROJECT || 'dvn-testing';
const env = readDotEnv(path.join('testing', '.env'));
const timeFormat = new Intl.DateTimeFormat('zh-TW', { dateStyle: 'medium', timeStyle: 'medium', hour12: false });

/** readDotEnv 只讀取測試工作區自動產生的簡單 KEY=VALUE，不引入額外相依。 */
function readDotEnv(file) {
  if (!existsSync(file)) return {};
  const values = {};
  for (const line of readFileSync(file, 'utf8').split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const separator = trimmed.indexOf('=');
    if (separator > 0) values[trimmed.slice(0, separator)] = trimmed.slice(separator + 1);
  }
  return values;
}

/** run 執行外部命令並保留 stdout；失敗時回傳錯誤內容供判斷，不直接中止。 */
function run(command, args) {
  try {
    return { ok: true, stdout: execFileSync(command, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim() };
  } catch (error) {
    return { ok: false, stdout: '', stderr: String(error.stderr || error.message).trim() };
  }
}

/** dockerJson 回傳 docker inspect 的第一筆 JSON，找不到時回傳 null。 */
function dockerJson(args) {
  const result = run('docker', args);
  if (!result.ok || !result.stdout) return null;
  try { return JSON.parse(result.stdout)[0] || null; } catch { return null; }
}

function localTime(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? String(value || '未提供') : timeFormat.format(date);
}

function portFor(key, fallback) {
  return Number(process.env[key] || env[key] || fallback);
}

const services = [
  { name: 'video-node', port: portFor('DVN_TEST_NODE_PORT', 18710) },
  { name: 'central', port: portFor('DVN_TEST_CENTRAL_PORT', 18720) },
  { name: 'transcode-worker', port: portFor('DVN_TEST_WORKER_PORT', 18730) },
];

if (!run('docker', ['version', '--format', '{{.Server.Version}}']).ok) {
  console.error('無法連線 Docker Engine；請確認 Docker Desktop 已啟動，且目前使用者可執行 docker。');
  process.exit(2);
}

const git = {
  head: run('git', ['rev-parse', 'HEAD']).stdout || 'unknown',
  dirty: Boolean(run('git', ['status', '--porcelain']).stdout),
};

const sourceDigest = buildInputDigest();
const newest = newestBuildInput();
const report = { generatedAt: new Date().toISOString(), project, git, source: { digest: sourceDigest, newest }, services: [] };

for (const service of services) {
  const tag = `dvn/${service.name}:local`;
  const image = dockerJson(['image', 'inspect', tag]);
  const labels = image?.Config?.Labels || {};
  const containerId = run('docker', ['ps', '-a', '--filter', `label=com.docker.compose.project=${project}`,
    '--filter', `label=com.docker.compose.service=${service.name}`, '--format', '{{.ID}}']).stdout.split(/\s+/).filter(Boolean)[0];
  const container = containerId ? dockerJson(['inspect', containerId]) : null;
  const running = container?.State?.Status === 'running';
  let status = null;
  if (running) {
    try {
      const response = await fetch(`http://127.0.0.1:${service.port}/api/v1/status`, { signal: AbortSignal.timeout(2000) });
      if (response.ok) status = await response.json();
    } catch { /* 服務尚未就緒時只顯示無法取得，不影響 image 判斷。 */ }
  }

  const builtAt = labels['org.opencontainers.image.created'] || 'unknown';
  const imageDigest = labels['dev.dvn.source-digest'] || 'unknown';
  const revision = labels['org.opencontainers.image.revision'] || 'unknown';
  const imageDirty = labels['dev.dvn.dirty'] || 'unknown';
  const issues = [];
  let warning = '';
  if (!image) {
    issues.push('找不到 image，請先執行 npm run image -- all');
  } else {
    if (builtAt === 'unknown') issues.push('image 沒有建置時間標籤，請用 npm run image -- all 重建');
    if (imageDigest === 'unknown') issues.push('image 沒有來源摘要，無法判斷是否為目前原始碼');
    else if (imageDigest !== sourceDigest) issues.push('image 來源摘要與目前原始碼不同');
    if (revision !== 'unknown' && git.head !== 'unknown' && revision !== git.head && imageDigest === sourceDigest) {
      warning = `image 記錄的 Commit ${revision.slice(0, 12)} 與目前 HEAD ${git.head.slice(0, 12)} 不同，但來源內容相同`;
    }
    if (running && container.Image !== image.Id) issues.push('執行中的容器仍使用舊 image，重新啟動測試環境即可套用');
  }
  const result = {
    service: service.name, tag, image: image ? {
      id: image.Id, created: image.Created, builtAt, revision, dirty: imageDirty, sourceDigest: imageDigest,
    } : null,
    container: container ? { id: container.Id, state: container.State?.Status, imageId: container.Image, created: container.Created } : null,
    status: status ? {
      version: status.build?.version, commit: status.build?.commit, buildTime: status.build?.buildTime,
      imageBuildTime: status.build?.imageBuildTime, imageSourceDigest: status.build?.imageSourceDigest,
    } : null,
    issues, warning,
  };
  report.services.push(result);
}

report.ok = report.services.every(service => service.issues.length === 0);

if (asJson) {
  console.log(JSON.stringify(report, null, 2));
} else {
  console.log('DVN 測試環境版本檢查');
  console.log(`專案      ${project}`);
  console.log(`Git HEAD  ${git.head.slice(0, 12)}${git.dirty ? '（有未提交變更）' : ''}`);
  console.log(`原始碼    ${sourceDigest.slice(0, 12)}（最新檔案 ${newest.path || '無'} · ${localTime(newest.mtimeMs)}）`);
  for (const service of report.services) {
    const image = service.image;
    console.log(`\n${service.service}  ${service.tag}`);
    if (!image) {
      console.log('  image       找不到');
    } else {
      const digestLabel = image.sourceDigest === 'unknown' ? '未提供'
        : `${image.sourceDigest.slice(0, 12)}${image.sourceDigest === sourceDigest ? '（相符）' : '（不同）'}`;
      console.log(`  image       ${image.id.slice(0, 19)}…`);
      console.log(`  建置時間    ${image.builtAt === 'unknown' ? '未提供' : localTime(image.builtAt)}`);
      console.log(`  來源摘要    ${digestLabel}`);
      console.log(`  Commit      ${image.revision === 'unknown' ? '未提供' : image.revision.slice(0, 12)}${image.dirty === 'true' ? ' · dirty' : ''}`);
    }
    if (!service.container) console.log('  容器        未啟動');
    else {
      console.log(`  容器        ${service.container.id.slice(0, 12)} · ${service.container.state}`);
      if (image) console.log(`  容器 image  ${service.container.imageId === image.id ? '與目前 image 相符' : '仍是舊 image'}`);
    }
    if (service.status) {
      console.log(`  服務回報    程式建置 ${service.status.buildTime} · Commit ${String(service.status.commit).slice(0, 12)}`);
    }
    if (service.issues.length) console.log(`  判斷        需要處理：${service.issues.join('；')}`);
    else console.log('  判斷        可用');
    if (service.warning) console.log(`  提醒        ${service.warning}`);
  }
  console.log(`\n結果：${report.ok ? '全部相符，測試環境使用的是目前原始碼。' : '有需要處理的項目。'}`);
  if (!report.ok) {
    console.log('建議：npm run testing:up');
  }
}

process.exitCode = report.ok ? 0 : 1;
