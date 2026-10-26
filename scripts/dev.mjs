import { spawn } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { createInterface } from 'node:readline';
import { fileURLToPath } from 'node:url';
import { createServer } from '../web/admin/node_modules/vite/dist/node/index.js';
import { ensureWorkspace } from './workspaces.mjs';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
const children = new Set();
let vite;
let stopping = false;
let workspace;
let currentTarget = '';
const input = createInterface({ input: process.stdin });
input.on('line', line => { if (line.trim().toLowerCase() === 'q') void stop(); });

/** 集中關閉本次啟動的子程序與前端，不影響其他開發工作階段。 */
async function stop(code = 0) {
  if (stopping) return;
  stopping = true;
  input.close();
  process.stdin.pause();
  // 先停止後端，讓代理的 keep-alive 連線結束，避免前端關閉等待閒置逾時。
  const frontendClosed = vite?.close();
  await Promise.all([...children].map(child => new Promise(resolve => {
    child.once('exit', resolve);
    child.kill();
  })));
  await frontendClosed;
  process.exitCode = code;
}

/** 啟動直接子程序並追蹤退出；不透過 shell 或 go run 留下孫程序。 */
function launch(name, command, args, supervised = true, onExit = null) {
  const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'] });
  children.add(child);
  child.once('exit', code => {
    children.delete(child);
    if (stopping || onExit?.(code)) return;
    if (supervised) {
      console.error(`[${name}] 非預期退出：${code}`);
      void stop(1);
    }
  });
  child.once('error', error => {
    children.delete(child);
    console.error(`[${name}] ${error.message}`);
    void stop(1);
  });
  createInterface({ input: child.stderr }).on('line', line => console.error(`[${name}] ${line}`));
  return child;
}

/** 建置固定路徑執行檔，成功後才啟動服務；失敗時保留編譯器診斷。 */
async function build(service, output) {
  const child = launch(`build:${service}`, 'go', ['build', '-o', output, `./apps/${service}`], false);
  child.stdout.pipe(process.stdout);
  await new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', code => code === 0 ? resolve() : reject(new Error(
      `${service} 建置失敗；若執行檔正在使用，請先關閉先前的 ${service}，再重新啟動。請參閱上方建置訊息。`,
    )));
  });
}

/** 從啟動日誌取得實際遞補端口，避免猜測或連到其他占用服務。 */
async function startService(service, onReady = null, buildService = true) {
  // 與正式建置共用穩定路徑，避免 Windows 防火牆將每次啟動視為新程式。
  const output = `bin/${service}${process.platform === 'win32' ? '.exe' : ''}`;
  if (buildService) await build(service, output);
  if (stopping) throw new Error('啟動已取消');
  const config = workspace.configs[service];
  console.log(`[${service}] 設定檔：${config}`);
  let ready = false;
  const scheduleRestart = () => {
    if (stopping) return;
    setTimeout(() => {
      if (stopping) return;
      console.log(`[${service}] 設定已儲存，重新啟動以套用…`);
      void startService(service, onReady, false).catch(error => {
        console.error(`[${service}] 重新啟動失敗：${error.message}`);
        void stop(1);
      });
    }, 1000);
  };
  const child = launch(service, `./${output}`, ['-config', config], false, code => {
    if (!ready) return false;
    if (code === 75) {
      scheduleRestart();
      return true;
    }
    console.error(`[${service}] 非預期退出：${code}`);
    void stop(1);
    return true;
  });
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`${service} 啟動逾時`));
    }, 15000);
    const fail = () => { clearTimeout(timer); reject(new Error(`${service} 啟動失敗`)); };
    child.once('exit', fail);
    child.once('error', fail);
    createInterface({ input: child.stdout }).on('line', line => {
      console.log(`[${service}] ${line}`);
      try {
        const entry = JSON.parse(line);
        if (entry.msg !== '服務啟動') return;
        const port = entry.listenAddress.match(/:(\d+)$/)?.[1];
        if (!port) return;
        clearTimeout(timer);
        child.removeListener('exit', fail);
        child.removeListener('error', fail);
        ready = true;
        const target = `http://127.0.0.1:${port}`;
        onReady?.(target);
        resolve(target);
      } catch { /* 非結構化輸出只作日誌，不用於判斷就緒。 */ }
    });
  });
}

process.on('SIGINT', () => void stop());
process.on('SIGTERM', () => void stop());

/** startFrontend 依目前後端位址建立 Vite；重啟端口改變時重建代理。 */
async function startFrontend(target) {
  if (vite) await vite.close();
  vite = await createServer({
    root: 'web/admin',
    server: {
      host: '0.0.0.0',
      proxy: {
        '/api': { target, changeOrigin: false },
        '/healthz': { target, changeOrigin: false },
      },
    },
  });
  await vite.listen();
  console.log(`\n[dev] API → ${target}；按 Ctrl+C 或輸入 q 關閉整組服務。`);
  vite.printUrls();
}

try {
  workspace = ensureWorkspace('development');
  console.log(`[dev] 工作區：${workspace.directory}；設定路徑與資料目錄可在「服務資訊」查看。`);
  mkdirSync('bin', { recursive: true });
  const onVideoTarget = target => {
    if (target === currentTarget) return;
    currentTarget = target;
    if (vite) {
      void startFrontend(target).catch(error => {
        console.error(`[dev] 重新建立前端代理失敗：${error.message}`);
        void stop(1);
      });
    }
  };
  const target = await startService('video-node', onVideoTarget);
  const response = await fetch(`${target}/api/v1/status`, { signal: AbortSignal.timeout(5000) });
  if (!response.ok || (await response.json()).build?.service !== 'video-node') {
    throw new Error(`Video Node 就緒檢查失敗：${target}`);
  }
  if (process.argv.includes('--all')) {
    await startService('central');
    await startService('transcode-worker');
  }
  if (!stopping) await startFrontend(target);
} catch (error) {
  console.error(`[dev] ${error.message}`);
  await stop(1);
}
