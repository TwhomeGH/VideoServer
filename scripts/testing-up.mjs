import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { buildInputDigest } from './build-inputs.mjs';
import { ensureWorkspace } from './workspaces.mjs';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
ensureWorkspace('testing');

/** readEnvFile 讀取 testing/.env 的簡單 KEY=VALUE，讓提示與 Compose 使用同一個值。 */
function readEnvFile(file) {
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

const commit = (() => {
  try { return execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim() || 'unknown'; } catch { return 'unknown'; }
})();
const dirty = (() => {
  try { return String(Boolean(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim())); } catch { return 'true'; }
})();
const env = {
  ...readEnvFile('testing/.env'),
  ...process.env,
  DVN_COMMIT: commit,
  DVN_DIRTY: dirty,
  DVN_BUILD_TIME: new Date().toISOString(),
  DVN_SOURCE_DIGEST: buildInputDigest(),
};
if (env.DVN_ADMIN_PASSWORD && [...env.DVN_ADMIN_PASSWORD].length < 12) {
  console.error('[testing] DVN_ADMIN_PASSWORD 少於 12 個字元；請修改 testing/.env 或移除該行。');
  process.exit(1);
}

console.log(`[testing] 建置並啟動測試環境（${env.DVN_BUILD_TIME}，來源 ${env.DVN_SOURCE_DIGEST.slice(0, 12)}）`);
if (!env.DVN_ADMIN_PASSWORD) {
  console.log('[testing] 未設定 DVN_ADMIN_PASSWORD；首次啟動的密碼在 video-node 日誌，已存在憑證時沿用已保存的密碼。');
}
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
  console.error('[testing] 啟動失敗，最近日誌：');
  try {
    execFileSync('docker', ['compose', ...composeFiles, 'logs', '--tail', '80'], { stdio: 'inherit', env });
  } catch { /* 顯示日誌失敗時仍保留原始錯誤。 */ }
  process.exit(typeof error.status === 'number' ? error.status : 1);
}

try {
  execFileSync(process.execPath, ['scripts/check-testing.mjs'], { stdio: 'inherit', env });
} catch (error) {
  process.exitCode = typeof error.status === 'number' ? error.status : 1;
}
