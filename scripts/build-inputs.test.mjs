import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { buildInputDigest } from './build-inputs.mjs';

function workspace() {
  const root = mkdtempSync(path.join(os.tmpdir(), 'dvn-build-'));
  mkdirSync(path.join(root, 'web/admin/src'), { recursive: true });
  writeFileSync(path.join(root, 'Dockerfile'), 'FROM scratch\n');
  writeFileSync(path.join(root, 'web/admin/src/app.ts'), 'export const value = 1;\n');
  return root;
}

test('建置摘要只隨來源內容改變', () => {
  const root = workspace();
  const initial = buildInputDigest(root);
  assert.equal(buildInputDigest(root), initial);
  writeFileSync(path.join(root, 'web/admin/src/app.ts'), 'export const value = 2;\n');
  assert.notEqual(buildInputDigest(root), initial);
  writeFileSync(path.join(root, 'web/admin/src/app.ts'), 'export const value = 1;\n');
  assert.equal(buildInputDigest(root), initial);
});

test('建置摘要排除快取、產物與 dockerignore 檔案', () => {
  const root = workspace();
  const initial = buildInputDigest(root);
  mkdirSync(path.join(root, 'web/admin/node_modules/pkg'), { recursive: true });
  mkdirSync(path.join(root, 'web/admin/dist'), { recursive: true });
  writeFileSync(path.join(root, 'web/admin/node_modules/pkg/index.js'), 'cache');
  writeFileSync(path.join(root, 'web/admin/dist/app.js'), 'build output');
  writeFileSync(path.join(root, 'web/admin/.env'), 'SECRET=1\n');
  writeFileSync(path.join(root, 'web/admin/src/clip.mp4'), 'ignored media');
  assert.equal(buildInputDigest(root), initial);
});
