import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { cleanupDeploymentTests } from './cleanup-deployment-tests.mjs';

const repository = fileURLToPath(new URL('..', import.meta.url));

/** runDeploymentTest 執行驗收及獨立保留階段，保留原測試失敗碼並回報清理失敗。
 * @param {string[]} args - 轉交驗收腳本的參數，例如 --skip-build。
 * @param {object} options - run／cleanup 可注入測試替身，避免驗證入口時建立真實容器。
 * @returns {Promise<number>} 最終程序結束碼。
 */
export async function runDeploymentTest(args = [], { root = repository, run = spawnSync, cleanup = cleanupDeploymentTests } = {}) {
  console.log('[deployment] 執行獨立部署測試；完成後依 test/deployment-retention.json 的保留與回收政策處理產物。');
  const result = run(process.execPath, [path.join(root, 'test/integration/deployment.mjs'), ...args], { cwd: root, stdio: 'inherit' });
  if (result.error) console.error(result.error.message);
  if (result.signal) console.error(`[deployment] 測試被 ${result.signal} 結束`);

  // 清理是明確的獨立階段，不藏在測試 finally；即使驗收失敗也保留最新一組供診斷。
  let cleanupFailed = false;
  try {
    const report = await cleanup({ root, apply: true });
    cleanupFailed = report.items.some(item => ['blocked', 'failed'].includes(item.status));
  } catch (error) { cleanupFailed = true; console.error(`[retention] 清理未完成：${error.message}`); }
  return result.status && result.status !== 0 ? result.status : result.error || result.signal || cleanupFailed ? 1 : 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  process.exitCode = await runDeploymentTest(process.argv.slice(2));
}
