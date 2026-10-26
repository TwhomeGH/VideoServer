import { ensureWorkspace } from './workspaces.mjs';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const mode = process.argv[2] || 'development';
const workspace = ensureWorkspace(mode);
console.log(`工作區：${workspace.directory}`);
for (const [service, config] of Object.entries(workspace.configs)) console.log(`${service} 設定：${config}`);
console.log(`影片測試目錄：${workspace.mediaDirectory}`);
console.log('已存在的設定與資料保持原樣。');
if (mode === 'development') {
  const config = JSON.parse(readFileSync(workspace.configs['video-node'], 'utf8'));
  console.log(`Video Node 資料目錄：${config.dataDirectory}`);
  if (path.resolve(config.dataDirectory) === path.resolve('data')) console.log('沿用既有 data/ 資料庫，不自動搬移或複製。');
}
