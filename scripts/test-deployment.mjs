import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
const project = `dvn-test-${Date.now()}`;
const directory = path.resolve('data', 'deployment-tests', project);
const media = path.join(directory, 'media');
const config = path.join(directory, 'video-node.json');
const envFile = path.join(directory, 'test.env');
const sample = path.join(media, '範例影片.mp4');
const steps = [];
let started = false;

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
    ...(body ? { method, headers: { 'Content-Type': 'application/json', Origin: base }, body: JSON.stringify(body) } : {}),
  });
  const result = await response.json();
  assert.ok(response.ok, `${route}: ${response.status} ${JSON.stringify(result)}`);
  return result;
}

mkdirSync(media, { recursive: true });
writeFileSync(config, JSON.stringify({ host: '0.0.0.0', preferredPort: 18610, fallbackPorts: [], webDirectory: '/app/web', dataDirectory: '/data/acceptance' }, null, 2));
writeFileSync(envFile, `DVN_BIND_ADDRESS=127.0.0.1\nDVN_NODE_PORT=${port}\nDVN_MEDIA_PATH=${media.replaceAll('\\', '/')}\nDVN_NODE_CONFIG=${config.replaceAll('\\', '/')}\n`);

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
  const status = await api('status');
  assert.equal(status.build.service, 'video-node');
  const html = await (await fetch(base)).text();
  const asset = html.match(/src="([^"]+\.js)"/)?.[1];
  assert.ok(asset, '找不到打包網頁的 JavaScript');
  assert.equal((await fetch(base + asset)).status, 200);
  pass('打包管理頁面與 API 可透過主機映射端口存取');
  const before = await api('library');
  const pool = await api('pools', { name: '測試影片庫', path: '/media' });
  assert.equal(pool.online, true);
  const details = { title: '部署測試影片', description: '測試專用，自行產生的色塊與聲音。', category: '測試', tags: ['Docker', 'docker', '部署測試'],
    workType: 'original', creators: [{ name: 'DVN 測試產生器', url: '' }], language: '無語音', adult: false, license: '測試素材' };
  const video = await api('videos/import', { poolId: pool.id, path: '範例影片.mp4', ...details });
  assert.deepEqual(video.tags, ['Docker', '部署測試']);
  const digest = createHash('sha256').update(readFileSync(sample)).digest('hex');
  assert.equal(video.fileHash, digest);
  pass('註冊 /media、匯入中文檔名、計算雜湊並保存 Tag 與作者');
  const content = `${base}/api/v1/videos/${video.id}/content`;
  const range = await fetch(content, { headers: { Range: 'bytes=0-1023' } });
  assert.equal(range.status, 206);
  assert.equal((await range.arrayBuffer()).byteLength, 1024);
  assert.equal((await fetch(content, { method: 'HEAD' })).status, 200);
  const full = await fetch(content);
  assert.equal(createHash('sha256').update(Buffer.from(await full.arrayBuffer())).digest('hex'), digest);
  pass('播放傳輸支援 Range / HEAD，完整影片內容雜湊一致');
  await api(`videos/${video.id}/metadata`, { ...details, title: '已編輯的測試影片' }, 'PUT');
  compose('exec', '-T', 'video-node', 'test', '-f', '/data/acceptance/library.sqlite');
  pass('編輯資料成功，資料庫位置由外掛 config 控制');
  const previousContainer = compose('ps', '-q', 'video-node');
  compose('up', '-d', '--force-recreate', '--no-build', '--pull', 'never', '--wait', '--wait-timeout', '60');
  assert.notEqual(compose('ps', '-q', 'video-node'), previousContainer);
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
    assert.equal((await fetch(content)).status, 503);
  } finally { renameSync(sample + '.offline', sample); }
  assert.equal((await api('library')).videos[0].available, true);
  pass('來源離線保留索引並拒絕播放，恢復檔案後重新可用');
  writeFileSync(path.join(directory, 'result.json'), JSON.stringify({ project, completedAt: new Date().toISOString(), steps, nodeId: before.nodeId, videoId: video.id }, null, 2));
  console.log(`測試紀錄與範例資料：${directory}`);
} catch (error) {
  if (started) console.error(compose('logs', '--tail', '50'));
  throw error;
} finally {
  // 保留測試資料卷與檔案供檢查，只停止這次建立的專案。
  if (started) compose('stop');
}
