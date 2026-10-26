import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { ensureWorkspace } from './workspaces.mjs';

test('開發設定獨立保存且重跑不覆寫', () => {
  const root = mkdtempSync(path.join(os.tmpdir(), 'dvn-config-'));
  const workspace = ensureWorkspace('development', root);
  const node = JSON.parse(readFileSync(workspace.configs['video-node'], 'utf8'));
  const central = JSON.parse(readFileSync(workspace.configs.central, 'utf8'));
  assert.notEqual(node.dataDirectory, central.dataDirectory);
  node.preferredPort = 19010;
  writeFileSync(workspace.configs['video-node'], JSON.stringify(node));
  ensureWorkspace('development', root);
  assert.equal(JSON.parse(readFileSync(workspace.configs['video-node'], 'utf8')).preferredPort, 19010);
});

test('偵測既有 Node 資料時沿用原路徑，不搬移資料', () => {
  const root = mkdtempSync(path.join(os.tmpdir(), 'dvn-legacy-'));
  mkdirSync(path.join(root, 'data'));
  writeFileSync(path.join(root, 'data', 'library.sqlite'), 'existing data');
  const workspace = ensureWorkspace('development', root);
  const node = JSON.parse(readFileSync(workspace.configs['video-node'], 'utf8'));
  assert.equal(node.dataDirectory, path.join(root, 'data'));
  assert.equal(readFileSync(path.join(root, 'data', 'library.sqlite'), 'utf8'), 'existing data');
});

test('測試設定使用容器路徑，重跑保留個人 .env', () => {
  const root = mkdtempSync(path.join(os.tmpdir(), 'dvn-testing-'));
  const workspace = ensureWorkspace('testing', root);
  const node = JSON.parse(readFileSync(workspace.configs['video-node'], 'utf8'));
  assert.equal(node.dataDirectory, '/data');
  assert.deepEqual(node.fallbackPorts, []);
  const env = path.join(root, 'testing', '.env');
  writeFileSync(env, 'DVN_TEST_NODE_PORT=19910\n');
  ensureWorkspace('testing', root);
  assert.equal(readFileSync(env, 'utf8'), 'DVN_TEST_NODE_PORT=19910\n');
});
