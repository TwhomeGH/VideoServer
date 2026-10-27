import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
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
/** checkTestingTLS 在 Compose 之前確認 config 的 TLS 憑證與 /certs 掛載都存在。 */
function checkTestingTLS() {
  const directory = 'testing/local/config';
  if (!existsSync(directory)) return;
  const entries = [];
  for (const name of readdirSync(directory).filter(value => value.endsWith('.json'))) {
    let config;
    try {
      config = JSON.parse(readFileSync(join(directory, name), 'utf8'));
    } catch {
      continue;
    }
    if (config.tlsCertFile || config.tlsKeyFile) {
      entries.push({ name, cert: config.tlsCertFile || '', key: config.tlsKeyFile || '' });
    }
  }
  if (entries.length === 0) return;
  const overlay = 'testing/compose.local.yaml';
  const overlayText = existsSync(overlay) ? readFileSync(overlay, 'utf8') : '';
  const hasCertMount = /target:\s*["']?\/certs(?:\/|\b)/m.test(overlayText);
  const missing = [];
  for (const entry of entries) {
    for (const [kind, containerPath] of [['cert', entry.cert], ['key', entry.key]]) {
      if (!containerPath.startsWith('/certs/')) continue;
      const hostPath = join('testing/certs', containerPath.slice('/certs/'.length));
      if (!existsSync(hostPath)) missing.push(`${hostPath}（${entry.name} ${kind}）`);
    }
  }
  if (!hasCertMount) {
    console.error('[testing] 偵測到 TLS 設定，但 testing/compose.local.yaml 沒有掛載 /certs。');
    console.error('[testing] 請加上 cert/key 的 bind mount，或移除 config 的 tlsCertFile／tlsKeyFile。');
    process.exit(1);
  }
  if (missing.length > 0) {
    console.error('[testing] 偵測到 TLS 設定，但找不到憑證：');
    for (const item of missing) console.error(`  - ${item}`);
    console.error('[testing] 請建立 cert/key，或移除 config 的 tlsCertFile／tlsKeyFile。');
    console.error('[testing] 測試環境不會自動退回 HTTP。');
    process.exit(1);
  }
}

checkTestingTLS();
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
