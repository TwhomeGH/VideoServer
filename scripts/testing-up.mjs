import { execFileSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { buildInputDigest } from './build-inputs.mjs';
import { ensureWorkspace } from './workspaces.mjs';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
ensureWorkspace('testing');

const commit = (() => {
  try { return execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim() || 'unknown'; } catch { return 'unknown'; }
})();
const dirty = (() => {
  try { return String(Boolean(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim())); } catch { return 'true'; }
})();
const env = {
  ...process.env,
  DVN_COMMIT: commit,
  DVN_DIRTY: dirty,
  DVN_BUILD_TIME: new Date().toISOString(),
  DVN_SOURCE_DIGEST: buildInputDigest(),
};

console.log(`[testing] 建置並啟動測試環境（${env.DVN_BUILD_TIME}，來源 ${env.DVN_SOURCE_DIGEST.slice(0, 12)}）`);
const composeFiles = ['-f', 'testing/compose.yaml'];
if (existsSync('testing/compose.local.yaml')) {
  composeFiles.push('-f', 'testing/compose.local.yaml');
  console.log('[testing] 疊加 testing/compose.local.yaml 的本機掛載');
}
try {
  execFileSync('docker', [
    'compose', ...composeFiles, '--profile', 'all',
    'up', '-d', '--build', '--pull', 'never', '--wait', '--wait-timeout', '60',
  ], { stdio: 'inherit', env });
} catch (error) {
  console.error('[testing] 啟動失敗，請查看上方 Docker 輸出。');
  process.exit(typeof error.status === 'number' ? error.status : 1);
}

try {
  execFileSync(process.execPath, ['scripts/check-testing.mjs'], { stdio: 'inherit', env });
} catch (error) {
  process.exitCode = typeof error.status === 'number' ? error.status : 1;
}
