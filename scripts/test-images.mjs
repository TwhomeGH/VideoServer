import { execFileSync } from 'node:child_process';

const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8' }).trim();

/** read 重試啟動中的容器端點，逾時時保留最後一次連線錯誤。 */
async function read(container, port, path) {
  let failure;
  for (let attempt = 0; attempt < 20; attempt++) {
    try { return docker('exec', container, 'wget', '-qO-', `http://127.0.0.1:${port}${path}`); }
    catch (error) { failure = error; await new Promise(resolve => setTimeout(resolve, 250)); }
  }
  throw failure;
}

for (const [service, port] of [['video-node', 18610], ['central', 18620], ['transcode-worker', 18630]]) {
  // 僅啟動與停止本測試建立的容器；不掛載使用者資料或公開端口。
  const container = docker('run', '--rm', '-d', '--network', 'none', `dvn/${service}:local`);
  try {
    const status = JSON.parse(await read(container, port, '/api/v1/status'));
    if (status.build.service !== service) throw new Error(`${service} 身分不符`);
    if (!(await read(container, port, '/')).includes('<html')) throw new Error('缺少打包管理頁面');
    if (service === 'video-node') {
      const before = JSON.parse(await read(container, port, '/api/v1/library'));
      docker('restart', container);
      const after = JSON.parse(await read(container, port, '/api/v1/library'));
      if (!before.nodeId || before.nodeId !== after.nodeId) throw new Error('重啟後 NodeID 遺失');
    }
    console.log(`${service}: Linux 啟動、API 與靜態頁面通過`);
  } finally { docker('stop', container); }
}
