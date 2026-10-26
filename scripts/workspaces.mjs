import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

/** services 是三服務共用的預設端口，供工作區初始化使用。 */
export const services = { 'video-node': 18610, central: 18620, 'transcode-worker': 18630 };
const repository = fileURLToPath(new URL('..', import.meta.url));

function writeNew(file, content) {
  try { writeFileSync(file, content, { flag: 'wx' }); }
  catch (error) { if (error.code !== 'EEXIST') throw error; }
}

/** ensureWorkspace 建立 development/testing 工作區，保留所有已存在的設定與資料。
 * @param mode - development 或 testing。
 * @param root - 專案根目錄，測試可指定隔離目錄。
 * @returns 工作區根目錄與三服務設定檔位置。
 * @throws 模式無效、目錄無法建立或既有設定 JSON 無效時拋出錯誤。
 */
export function ensureWorkspace(mode, root = repository) {
  if (!['development', 'testing'].includes(mode)) throw new Error('工作區必須為 development 或 testing');
  const directory = mode === 'development' ? path.join(root, 'local', mode) : path.join(root, 'testing', 'local');
  const configDirectory = path.join(directory, 'config');
  const mediaDirectory = path.join(directory, 'media');
  mkdirSync(configDirectory, { recursive: true });
  mkdirSync(mediaDirectory, { recursive: true });
  const configs = {};
  for (const [service, port] of Object.entries(services)) {
    const legacy = path.join(root, 'data');
    const dataDirectory = mode === 'testing' ? '/data' :
      service === 'video-node' && existsSync(path.join(legacy, 'library.sqlite')) ? legacy : path.join(directory, 'data', service);
    if (mode === 'development') mkdirSync(dataDirectory, { recursive: true });
    const config = {
      host: '0.0.0.0', preferredPort: port,
      fallbackPorts: mode === 'testing' ? [] : [port + 1, port + 2, port + 3],
      rememberLastSuccessfulPort: mode !== 'testing',
      webDirectory: mode === 'testing' ? '/app/web' : path.join(root, 'web', 'admin', 'dist'),
      dataDirectory,
      autoScan: true, scanIntervalSeconds: 30, fileStableSeconds: 10,
    };
    const file = path.join(configDirectory, `${service}.json`);
    writeNew(file, JSON.stringify(config, null, 2) + '\n');
    JSON.parse(readFileSync(file, 'utf8'));
    configs[service] = file;
  }
  if (mode === 'testing') {
    writeNew(path.join(root, 'testing', '.env'), 'DVN_TEST_BIND_ADDRESS=0.0.0.0\nDVN_TEST_NODE_PORT=18710\nDVN_TEST_CENTRAL_PORT=18720\nDVN_TEST_WORKER_PORT=18730\n');
  }
  return { directory, mediaDirectory, configs };
}
