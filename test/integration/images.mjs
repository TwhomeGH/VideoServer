import { execFileSync } from 'node:child_process';
import net from 'node:net';

const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8' }).trim();
const adminPassword = 'image-test-password';

/** freePort 取得 loopback 空閒端口，讓容器重啟後仍使用相同映射。 */
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}

/** sleep 讓容器啟動與重啟的等待保持簡單可讀。 */
function sleep(milliseconds) {
  return new Promise(resolve => setTimeout(resolve, milliseconds));
}

/** waitFor 重試同源請求，直到服務就緒或逾時。 */
async function waitFor(url, init = {}) {
  let failure;
  for (let attempt = 0; attempt < 80; attempt++) {
    try {
      const response = await fetch(url, { ...init, signal: AbortSignal.timeout(1500) });
      if (response.ok) return response;
      failure = new Error(`${url} 回應 ${response.status}`);
    } catch (error) {
      failure = error;
    }
    await sleep(250);
  }
  throw failure;
}

/** login 取得管理 session；image 測試不繞過正式登入流程。 */
async function login(base) {
  const response = await fetch(`${base}/api/v1/auth/login`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password: adminPassword }),
  });
  if (!response.ok) throw new Error(`登入失敗：${response.status}`);
  const value = response.headers.get('set-cookie');
  if (!value) throw new Error('登入沒有回傳 session cookie');
  return value.split(';')[0];
}

for (const [service, port] of [['video-node', 18610], ['central', 18620], ['transcode-worker', 18630]]) {
  // 只映射到主機 loopback，不掛載使用者資料或公開測試端口。
  const hostPort = await freePort();
  const container = docker('run', '--rm', '-d', '-p', `127.0.0.1:${hostPort}:${port}`, '-e', `DVN_ADMIN_PASSWORD=${adminPassword}`, `dvn/${service}:local`);
  try {
    const base = `http://127.0.0.1:${hostPort}`;
    const status = await (await waitFor(`${base}/api/v1/status`)).json();
    if (status.build.service !== service) throw new Error(`${service} 身分不符`);
    if (!(await (await waitFor(base)).text()).includes('<html')) throw new Error('缺少打包管理頁面');
    if (service === 'video-node') {
      const cookie = await login(base);
      const before = await (await waitFor(`${base}/api/v1/library`, { headers: { Cookie: cookie } })).json();
      docker('restart', container);
      const after = await (await waitFor(`${base}/api/v1/library`, { headers: { Cookie: cookie } })).json();
      if (!before.nodeId || before.nodeId !== after.nodeId) throw new Error('重啟後 NodeID 遺失');
    }
    console.log(`${service}: Linux 啟動、登入、API 與靜態頁面通過`);
  } catch (error) {
    console.error(`${service}: ${error.message}`);
    console.error(docker('logs', container, '--tail', '30'));
    throw error;
  } finally { docker('stop', container); }
}
