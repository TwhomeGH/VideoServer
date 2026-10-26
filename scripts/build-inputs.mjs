import { createHash } from 'node:crypto';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repository = fileURLToPath(new URL('..', import.meta.url));

/** buildInputs 是 Dockerfile 實際會複製進 image 的來源，供版本摘要與新舊判斷使用。 */
const buildInputs = [
  'Dockerfile',
  '.dockerignore',
  'go.mod',
  'go.sum',
  'apps',
  'internal',
  'pkg',
  'deployments/docker',
  'web/admin',
];

/** skipDirectories 是建置時會重新產生或不安裝的目錄，不列入來源摘要。 */
const skipDirectories = new Set(['node_modules', 'dist', '.git']);

/** ignoredFile 對齊 .dockerignore 的檔案排除規則，避免本機暫存檔造成誤判。 */
function ignoredFile(name) {
  return name === '.env' || name.startsWith('.env.') || /\.(exe|sqlite[^/]*|mp4|mkv|webm)$/i.test(name);
}

/** collectFiles 走訪指定來源，回傳相對於專案根目錄、排序後的檔案清單。 */
function collectFiles(root) {
  const files = [];
  const visit = relative => {
    const absolute = path.join(root, relative);
    const stat = statSync(absolute, { throwIfNoEntry: false });
    if (!stat) return;
    if (stat.isFile()) {
      files.push(relative.split(path.sep).join('/'));
      return;
    }
    if (!stat.isDirectory()) return;
    for (const entry of readdirSync(absolute, { withFileTypes: true }).sort((left, right) =>
      left.name < right.name ? -1 : left.name > right.name ? 1 : 0)) {
      if (entry.isDirectory() && skipDirectories.has(entry.name)) continue;
      if (entry.isFile() && ignoredFile(entry.name)) continue;
      if (entry.isFile() || entry.isDirectory()) visit(path.join(relative, entry.name));
    }
  };
  for (const input of buildInputs) {
    const name = path.basename(input);
    if (!ignoredFile(name)) visit(input);
  }
  return files.sort();
}

/** buildInputDigest 對建置輸入的內容計算穩定摘要；註解與檔案時間不影響結果。 */
export function buildInputDigest(root = repository) {
  const hash = createHash('sha256');
  for (const relative of collectFiles(root)) {
    hash.update(relative);
    hash.update('\0');
    hash.update(readFileSync(path.join(root, relative)));
    hash.update('\0');
  }
  return hash.digest('hex');
}

/** newestBuildInput 回傳最近修改的建置輸入，用於說明差異來源而非判定新舊。 */
export function newestBuildInput(root = repository) {
  let newest = { path: '', mtimeMs: 0 };
  for (const relative of collectFiles(root)) {
    const stat = statSync(path.join(root, relative));
    if (stat.mtimeMs > newest.mtimeMs) newest = { path: relative, mtimeMs: stat.mtimeMs };
  }
  return newest;
}
