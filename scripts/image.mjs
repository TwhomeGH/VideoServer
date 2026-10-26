import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
const service = process.argv[2] || 'video-node';
if (!['video-node', 'central', 'transcode-worker'].includes(service)) {
  throw new Error('請指定 video-node、central 或 transcode-worker');
}

// 從實際工作目錄取得識別；未能取得 Git 時明確保留 unknown 與 dirty。
let commit = 'unknown';
let dirty = 'true';
try {
  commit = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
  dirty = String(Boolean(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim()));
} catch { console.warn('無 Git 資訊，image 將標示 commit=unknown、dirty=true'); }

execFileSync('docker', [
  'build', '--target', service, '-t', `dvn/${service}:local`,
  '--build-arg', `VCS_REF=${commit}`, '--build-arg', `DIRTY=${dirty}`, '.',
], { stdio: 'inherit' });
