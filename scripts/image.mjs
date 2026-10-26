import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { buildInputDigest } from './build-inputs.mjs';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
const requested = process.argv[2] || 'video-node';
const services = ['video-node', 'central', 'transcode-worker'];
if (requested !== 'all' && !services.includes(requested)) {
  throw new Error('請指定 video-node、central、transcode-worker 或 all');
}

// 從實際工作目錄取得識別；未能取得 Git 時明確保留 unknown 與 dirty。
let commit = 'unknown';
let dirty = 'true';
try {
  commit = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
  dirty = String(Boolean(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim()));
} catch { console.warn('無 Git 資訊，image 將標示 commit=unknown、dirty=true'); }

// 建置時間與來源摘要寫入 image 標籤與環境變數；即使層快取命中也能辨識本次建置。
const buildTime = new Date().toISOString();
const sourceDigest = buildInputDigest();
for (const service of requested === 'all' ? services : [requested]) {
  execFileSync('docker', [
    'build', '--target', service, '-t', `dvn/${service}:local`,
    '--build-arg', `VCS_REF=${commit}`,
    '--build-arg', `DIRTY=${dirty}`,
    '--build-arg', `BUILD_TIME=${buildTime}`,
    '--build-arg', `SOURCE_DIGEST=${sourceDigest}`,
    '.',
  ], { stdio: 'inherit' });
}
console.log(`已建置 ${requested === 'all' ? '三個服務' : requested}：${buildTime}，來源摘要 ${sourceDigest.slice(0, 12)}`);
