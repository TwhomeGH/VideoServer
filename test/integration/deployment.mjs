import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

process.chdir(fileURLToPath(new URL('../..', import.meta.url)));
const project = `dvn-test-${Date.now()}`;
const directory = path.resolve('data', 'deployment-tests', project);
const media = path.join(directory, 'media');
const config = path.join(directory, 'video-node.json');
const envFile = path.join(directory, 'test.env');
const sample = path.join(media, '範例影片.mp4');
const steps = [];
let started = false;
let completed = false;
let cookie = '';
const adminPassword = 'deployment-test-password';

/** freePort 取得本機測試端口；正式端口與其他開發服務不受影響。 */
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}

/** compose 對獨立測試專案執行命令，環境值不繼承正式部署的 DVN 設定。 */
function compose(...args) {
  const env = { ...process.env };
  for (const key of Object.keys(env)) if (key.startsWith('DVN_') || key.startsWith('COMPOSE_')) delete env[key];
  return execFileSync('docker', ['compose', '-f', 'compose.yaml', '--env-file', envFile, '-p', project, ...args], {
    encoding: 'utf8', env,
  }).trim();
}

function pass(label) { steps.push(label); console.log(`通過：${label}`); }

const port = await freePort();
const base = `http://127.0.0.1:${port}`;

/** api 發出同源 JSON 管理請求，非預期狀態會保留服務回應以便診斷。 */
async function api(route, body, method = 'POST') {
  const response = await fetch(`${base}/api/v1/${route}`, {
    signal: AbortSignal.timeout(15000),
    ...(body ? { method, headers: { 'Content-Type': 'application/json', 'X-DVN-Protocol-Version': '1', Origin: base, ...(cookie ? { Cookie: cookie } : {}) }, body: JSON.stringify(body) } : {
      headers: { Origin: base, 'X-DVN-Protocol-Version': '1', ...(cookie ? { Cookie: cookie } : {}) },
    }),
  });
  const result = await response.json();
  assert.ok(response.ok, `${route}: ${response.status} ${JSON.stringify(result)}`);
  return result;
}

/** login 取得管理 session cookie；後續 API 與影片傳輸都帶入。 */
async function login() {
  const response = await fetch(`${base}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: base },
    body: JSON.stringify({ password: adminPassword }),
  });
  assert.equal(response.status, 200, `login: ${await response.text()}`);
  const value = response.headers.get('set-cookie');
  assert.ok(value, '登入沒有回傳 session cookie');
  cookie = value.split(';')[0];
}

/** authorizedFetch 對影片內容請求帶入 session cookie。 */
function authorizedFetch(route, init = {}) {
  return fetch(route, { ...init, headers: { ...(init.headers || {}), ...(cookie ? { Cookie: cookie } : {}) } });
}

/** waitJob 等待背景工作成功或失敗，逾時時保留最近狀態供診斷。 */
async function waitJob(id) {
  let job;
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    job = await api(`import-jobs/${id}`);
    if (job.status === 'completed' || job.status === 'failed') return job;
    await new Promise(resolve => setTimeout(resolve, 250));
  }
  assert.fail(`工作逾時：${JSON.stringify(job)}`);
}

mkdirSync(media, { recursive: true });
writeFileSync(config, JSON.stringify({ host: '0.0.0.0', preferredPort: 18610, fallbackPorts: [], webDirectory: '/app/web', dataDirectory: '/data/acceptance' }, null, 2));
writeFileSync(envFile, `DVN_BIND_ADDRESS=127.0.0.1\nDVN_NODE_PORT=${port}\nDVN_MEDIA_PATH=${media.replaceAll('\\', '/')}\nDVN_NODE_CONFIG=${config.replaceAll('\\', '/')}\nDVN_ADMIN_PASSWORD=${adminPassword}\n`);
const run = { kind: 'dvn-deployment-test', schemaVersion: 1, project, workspace: process.cwd(), pid: process.pid, host: os.hostname(), status: 'running', createdAt: new Date().toISOString() };
writeFileSync(path.join(directory, 'run.json'), JSON.stringify(run, null, 2));

try {
  if (!process.argv.includes('--skip-build')) {
    execFileSync(process.execPath, ['scripts/image.mjs', 'video-node'], { stdio: 'inherit' });
    pass('本機從原始碼建置 image');
  }
  execFileSync('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-f', 'lavfi', '-i', 'color=c=blue:s=320x180:r=24',
    '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=44100', '-t', '2', '-c:v', 'libx264', '-pix_fmt', 'yuv420p',
    '-c:a', 'aac', '-movflags', '+faststart', sample], { stdio: 'inherit' });
  pass('產生獨立 H.264/AAC 範例影片');
  compose('config', '--quiet');
  started = true;
  compose('up', '-d', '--no-build', '--pull', 'never', '--wait', '--wait-timeout', '60');
  pass('Compose 以外掛 config、資料卷及唯讀影片目錄啟動');
  assert.equal((await fetch(`${base}/api/v1/import-jobs/nonexistent`)).status, 401);
  assert.equal((await fetch(`${base}/api/v1/monitor`)).status, 401);
  assert.equal((await fetch(`${base}/api/v1/openapi.json`)).status, 401);
  const protocol = await (await fetch(`${base}/api/v1/protocol`)).json();
  assert.equal(protocol.protocolVersion, 1);
  assert.ok(protocol.capabilities.includes('library.query'));
  assert.equal((await fetch(`${base}/api/v1/videos/nonexistent/probe`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })).status, 401);
  assert.equal((await fetch(`${base}/api/v1/videos/import`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })).status, 401);
  pass('匯入工作與建立工作 API 均要求登入');
  await login();
  pass('管理員登入並取得 session cookie');
  const status = await api('status');
  assert.equal(status.build.service, 'video-node');
  const html = await (await fetch(base)).text();
  const asset = html.match(/src="([^"]+\.js)"/)?.[1];
  assert.ok(asset, '找不到打包網頁的 JavaScript');
  assert.equal((await fetch(base + asset)).status, 200);
  pass('打包管理頁面與 API 可透過主機映射端口存取');
  const before = await api('library');
  const spec = await api('openapi.json');
  assert.equal(spec.openapi, '3.1.1');
  assert.ok(spec.paths['/api/v1/videos/{id}/probe']);
  const rejectedPool = await authorizedFetch(`${base}/api/v1/pools`, {
    method: 'POST', headers: { 'Content-Type': 'application/json', 'X-DVN-Protocol-Version': '2', Origin: base },
    body: JSON.stringify({ name: '不應建立', path: '/media' }),
  });
  assert.equal(rejectedPool.status, 409);
  assert.equal((await rejectedPool.json()).code, 'protocol_incompatible');
  assert.equal((await api('library')).pools.length, 0);
  pass('公開版本能力、已授權 OpenAPI 與協定不相容時阻止寫入');
  const pool = await api('pools', { name: '測試影片庫', path: '/media' });
  assert.equal(pool.online, true);
  const details = { title: '部署測試影片', description: '測試專用，自行產生的色塊與聲音。', category: '測試', tags: ['Docker', 'docker', '部署測試'],
    workType: 'original', creators: [{ name: 'DVN 測試產生器', url: '' }], language: '無語音', adult: false, license: '測試素材' };
  const job = await api('videos/import', { poolId: pool.id, path: '範例影片.mp4', ...details });
  const finished = await waitJob(job.id);
  assert.equal(finished?.status, 'completed', finished?.error || '匯入工作逾時');
  const video = (await api('library')).videos.find(item => item.id === finished.videoId);
  assert.ok(video, '完成工作後找不到影片');
  assert.equal(video.media.width, 320);
  assert.equal(video.media.height, 180);
  assert.equal(video.media.videoCodec, 'h264');
  assert.equal(video.media.audioCodec, 'aac');
  assert.ok(video.media.durationSeconds >= 1.9 && video.media.durationSeconds <= 2.2);
  assert.deepEqual(video.tags, ['Docker', '部署測試']);
  const digest = createHash('sha256').update(readFileSync(sample)).digest('hex');
  assert.equal(video.fileHash, digest);
  pass('註冊 /media、匯入中文檔名、計算雜湊並保存 Tag 與作者');
  const content = `${base}/api/v1/videos/${video.id}/content`;
  const range = await authorizedFetch(content, { headers: { Range: 'bytes=0-1023' } });
  assert.equal(range.status, 206);
  assert.equal((await range.arrayBuffer()).byteLength, 1024);
  assert.equal((await authorizedFetch(content, { method: 'HEAD' })).status, 200);
  const full = await authorizedFetch(content);
  assert.equal(createHash('sha256').update(Buffer.from(await full.arrayBuffer())).digest('hex'), digest);
  pass('播放傳輸支援 Range / HEAD，完整影片內容雜湊一致');
  await api(`videos/${video.id}/metadata`, { ...details, title: '已編輯的測試影片' }, 'PUT');
  compose('exec', '-T', 'video-node', 'test', '-f', '/data/acceptance/library.sqlite');
  pass('編輯資料成功，資料庫位置由外掛 config 控制');
  const probeJob = await api(`videos/${video.id}/probe`, {});
  const probed = await waitJob(probeJob.id);
  assert.equal(probed.status, 'completed', probed.error);
  const refreshed = (await api('library')).videos.find(item => item.id === video.id);
  assert.equal(refreshed.title, '已編輯的測試影片');
  assert.equal(refreshed.fileHash, video.fileHash);
  assert.deepEqual(refreshed.tags, video.tags);
  assert.deepEqual(refreshed.creators, video.creators);
  assert.equal(refreshed.media.width, 320);
  pass('重新讀取影片資訊並保留影片識別、雜湊、標題與描述');
  let monitor = await api('monitor');
  for (let attempt = 0; monitor.cpuPercent === null && attempt < 10; attempt++) {
    await new Promise(resolve => setTimeout(resolve, 500));
    monitor = await api('monitor');
  }
  assert.equal(monitor.service, 'video-node');
  assert.equal(typeof monitor.cpuPercent, 'number');
  assert.ok(monitor.cpuPercent >= 0);
  assert.ok(monitor.heapAllocBytes > 0);
  assert.ok(monitor.sentBytes >= 1024);
  assert.ok(monitor.receivedBytes > 0);
  assert.ok(monitor.tasks.jobs.counts.completed >= 2);
  assert.equal(typeof monitor.tasks.scan.running, 'boolean');
  pass('已授權監看 API 提供 Linux CPU、Go 記憶體、HTTP 流量與工作統計');
  const previousContainer = compose('ps', '-q', 'video-node');
  compose('up', '-d', '--force-recreate', '--no-build', '--pull', 'never', '--wait', '--wait-timeout', '60');
  assert.notEqual(compose('ps', '-q', 'video-node'), previousContainer);
  await login();
  const after = await api('library');
  assert.equal(after.nodeId, before.nodeId);
  assert.equal(after.pools[0].id, pool.id);
  assert.equal(after.videos[0].id, video.id);
  assert.equal(after.videos[0].title, '已編輯的測試影片');
  assert.equal(after.videos[0].fileHash, digest);
  assert.deepEqual(after.videos[0].tags, ['Docker', '部署測試']);
  pass('強制重建容器後 NodeID、儲存池、影片與編輯資料完整保留');
  renameSync(sample, sample + '.offline');
  try {
    const offline = await api('library');
    assert.equal(offline.videos.length, 1);
    assert.equal(offline.videos[0].available, false);
    assert.equal((await authorizedFetch(content)).status, 503);
    const offlineProbe = await api(`videos/${video.id}/probe`, {});
    const failedProbe = await waitJob(offlineProbe.id);
    assert.equal(failedProbe.status, 'failed');
    assert.deepEqual((await api('library')).videos[0].media, refreshed.media);
  } finally { renameSync(sample + '.offline', sample); }
  assert.equal((await api('library')).videos[0].available, true);
  await api(`import-jobs/${probeJob.id}/retry`, {});
  assert.equal((await waitJob(probeJob.id)).status, 'completed');
  pass('來源離線時探測失敗保留技術資訊，恢復後可重試');
  pass('來源離線保留索引並拒絕播放，恢復檔案後重新可用');
  writeFileSync(path.join(directory, 'result.json'), JSON.stringify({ project, completedAt: new Date().toISOString(), steps, nodeId: before.nodeId, videoId: video.id }, null, 2));
  console.log(`測試紀錄與範例資料：${directory}`);
  completed = true;
} catch (error) {
  if (started) console.error(compose('logs', '--tail', '50'));
  throw error;
} finally {
  // 保留測試資料卷與檔案供檢查，只停止這次建立的專案。
  if (started) compose('stop');
  writeFileSync(path.join(directory, 'run.json'), JSON.stringify({ ...run, status: completed ? 'completed' : 'failed', finishedAt: new Date().toISOString() }, null, 2));
}
