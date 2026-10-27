import { existsSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import OpenCC from 'opencc-js';

// 語言資料的來源是 zh_TW.json；zh_CN.json 由台灣繁體片語轉換器產生，不手動維護。
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const langDir = join(root, 'src', 'lang');
const baseName = 'zh_TW.json';
const generatedName = 'zh_CN.json';
const overridesName = 'zh_CN.overrides.json';
const check = process.argv.includes('--check');

const readJson = file => JSON.parse(readFileSync(file, 'utf8'));
const base = readJson(join(langDir, baseName));
const overrides = existsSync(join(langDir, overridesName)) ? readJson(join(langDir, overridesName)) : {};
const overrideKeys = Object.keys(overrides).sort((a, b) => b.length - a.length);
const converter = OpenCC.Converter({ from: 'twp', to: 'cn' });

/** convert 先做台灣繁體→簡體片語轉換，再套用轉換後的用語修正。 */
function convert(value) {
  let result = converter(value);
  for (const key of overrideKeys) result = result.split(key).join(overrides[key]);
  return result;
}

const expected = Object.fromEntries(Object.entries(base).map(([key, value]) => [key, convert(value)]));
const expectedText = `${JSON.stringify(expected, null, 2)}\n`;
const placeholders = value => [...value.matchAll(/\{([a-zA-Z0-9_]+)\}/g)].map(match => match[1]).sort().join(',');
const errors = [];

for (const [key, value] of Object.entries(base)) {
  if (typeof value !== 'string' || !value.trim()) errors.push(`${baseName}: ${key} 是空值`);
}

/** walk 遞迴列出 src 下的 TypeScript 檔，用來檢查程式使用的鍵是否都存在。 */
function walk(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) return walk(full);
    return /\.(ts|tsx)$/.test(entry.name) ? [full] : [];
  });
}
const usedKeys = new Set();
for (const file of walk(join(root, 'src'))) {
  const text = readFileSync(file, 'utf8');
  for (const match of text.matchAll(/\b(?:t|translate)\(\s*'([^']+)'/g)) usedKeys.add(match[1]);
  for (const match of text.matchAll(/'((?:page|jobPhase)\.[^']+)'/g)) usedKeys.add(match[1]);
}
for (const key of Object.keys(base)) {
  if (key.startsWith('jobPhase.')) usedKeys.add(key);
}
for (const key of usedKeys) {
  if (!(key in base)) errors.push(`程式使用未定義的鍵：${key}`);
}
const unusedKeys = Object.keys(base).filter(key => !usedKeys.has(key));
if (unusedKeys.length) console.warn(`[lang] 未使用的鍵（${unusedKeys.length}）：${unusedKeys.join(', ')}`);

const translationFiles = readdirSync(langDir)
  .filter(file => file.endsWith('.json') && file !== baseName && file !== generatedName && file !== overridesName);
const registry = readFileSync(join(root, 'src', 'i18n', 'registry.ts'), 'utf8');
for (const file of translationFiles) {
  const code = file.replace(/\.json$/, '').replace(/_/g, '-');
  if (!registry.includes(`'../lang/${file}'`) || !registry.includes(`code: '${code}'`)) {
    errors.push(`registry.ts 未註冊 ${file}（需要 '../lang/${file}' 與 code: '${code}'）`);
  }
}
for (const file of translationFiles) {
  const messages = readJson(join(langDir, file));
  for (const [key, value] of Object.entries(base)) {
    if (!(key in messages)) {
      errors.push(`${file}: 缺少 ${key}`);
      continue;
    }
    if (typeof messages[key] !== 'string' || !messages[key].trim()) errors.push(`${file}: ${key} 是空值`);
    if (placeholders(messages[key]) !== placeholders(value)) {
      errors.push(`${file}: ${key} 的插值參數不一致（應為 ${placeholders(value) || '無'}）`);
    }
  }
  for (const key of Object.keys(messages)) {
    if (!(key in base)) errors.push(`${file}: 多出 ${key}`);
  }
}

if (check) {
  const actualPath = join(langDir, generatedName);
  if (!existsSync(actualPath)) {
    errors.push(`${generatedName}: 不存在；請執行 npm run lang:build`);
  } else if (JSON.stringify(readJson(actualPath)) !== JSON.stringify(expected)) {
    errors.push(`${generatedName}: 已過期；請執行 npm run lang:build`);
  }
} else {
  writeFileSync(join(langDir, generatedName), expectedText);
  console.log(`[lang] 已產生 ${generatedName}（${Object.keys(expected).length} 個鍵）`);
}

if (errors.length) {
  console.error(`[lang] 檢查失敗：\n${errors.join('\n')}`);
  process.exit(1);
}
if (check) console.log(`[lang] ${Object.keys(base).length} 個鍵、${translationFiles.length + 1} 個語言檔通過`);
