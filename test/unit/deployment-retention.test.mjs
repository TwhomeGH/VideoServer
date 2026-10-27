import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, readdirSync, readFileSync, renameSync, symlinkSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { cleanupDeploymentTests } from '../../scripts/cleanup-deployment-tests.mjs';
import { inspectTestFolder, ownedResources, retentionPlan, testResources } from '../../scripts/deployment-retention.mjs';
import { runDeploymentTest } from '../../scripts/run-deployment-test.mjs';

function fixture(count = 7) {
  const root = mkdtempSync(path.join(os.tmpdir(), 'dvn-retention-'));
  mkdirSync(path.join(root, 'test'));
  writeFileSync(path.join(root, 'test/deployment-retention.json'), JSON.stringify({ keep: 5, allowDockerRemoval: false }));
  const names = [];
  for (let index = 0; index < count; index++) {
    const project = `dvn-test-${1700000000000 + index}`;
    const directory = path.join(root, 'data/deployment-tests', project);
    mkdirSync(path.join(directory, 'media'), { recursive: true });
    writeFileSync(path.join(directory, 'result.json'), JSON.stringify({ project }));
    writeFileSync(path.join(directory, 'video-node.json'), JSON.stringify({ dataDirectory: '/data/acceptance' }));
    writeFileSync(path.join(directory, 'media/sample.mp4'), 'sample');
    writeFileSync(path.join(directory, 'test.env'), `DVN_MEDIA_PATH=${path.join(directory, 'media')}\nDVN_NODE_CONFIG=${path.join(directory, 'video-node.json')}\n`);
    names.push(project);
  }
  return { root, names, base: path.join(root, 'data/deployment-tests') };
}

const empty = () => ({ containers: [], volumes: [], networks: [] });
const noDocker = args => {
  if (args.includes('rm')) assert.fail('unexpected Docker removal');
  return '';
};

function resourceFixture(root, project) {
  const directory = path.join(root, 'data/deployment-tests', project);
  return {
    containers: [{ Id: 'a'.repeat(64), Name: '/' + project + '-video-node-1', State: { Status: 'exited' }, Mounts: [], Config: { Labels: {
      'com.docker.compose.project': project, 'com.docker.compose.service': 'video-node',
      'com.docker.compose.project.working_dir': root, 'com.docker.compose.project.config_files': path.join(root, 'compose.yaml'),
      'com.docker.compose.project.environment_file': path.join(directory, 'test.env'),
    } } }],
    volumes: [{ Name: project + '_node-data', Driver: 'local', Options: null, Labels: { 'com.docker.compose.project': project, 'com.docker.compose.volume': 'node-data' } }],
    networks: [{ Id: 'network-id', Name: project + '_default', Containers: {}, Labels: { 'com.docker.compose.project': project, 'com.docker.compose.network': 'default' } }],
  };
}

test('部署產物保留最近五組，不以資料夾修改時間排序', () => {
  const { root, names, base } = fixture();
  mkdirSync(path.join(base, 'dvn-testing'));
  const plan = retentionPlan(root, 5, empty(), noDocker);
  assert.deepEqual(plan.retained.map(item => item.project), names.slice(2).reverse());
  assert.deepEqual(plan.remove.map(item => item.project), names.slice(0, 2).reverse());
  assert.equal(plan.skipped.length, 0);
  assert.throws(() => retentionPlan(root, 0, empty(), noDocker));
  assert.throws(() => inspectTestFolder(root, '../dvn-testing'));
});

test('略過活動測試、連結與包含未知檔案的目錄', () => {
  const { root, names, base } = fixture();
  const first = path.join(base, names[0]);
  writeFileSync(path.join(first, 'run.json'), JSON.stringify({ kind: 'dvn-deployment-test', workspace: root, project: names[0], status: 'running', host: os.hostname(), pid: process.pid }));
  writeFileSync(path.join(base, names[1], 'do-not-delete.txt'), 'user content');
  const outside = path.join(root, 'outside');
  mkdirSync(outside);
  symlinkSync(outside, path.join(base, 'dvn-test-1700000000999'), process.platform === 'win32' ? 'junction' : 'dir');
  const plan = retentionPlan(root, 5, empty(), noDocker);
  assert.equal(plan.remove.length, 0);
  assert.equal(plan.skipped.length, 3);
});

test('拒絕執行中容器、不同工作區、共用資料卷與外部網路', () => {
  const { root, names } = fixture(1);
  const candidate = inspectTestFolder(root, names[0]);
  const original = resourceFixture(root, names[0]);
  assert.equal(ownedResources(root, candidate, original, () => 'a'.repeat(12)).containers.length, 1);
  for (const change of [
    value => { value.containers[0].State.Status = 'running'; },
    value => { value.containers[0].Config.Labels['com.docker.compose.project.working_dir'] = path.dirname(root); },
    value => { value.containers[0].Mounts = [{ Type: 'volume', Name: 'user-data' }]; },
    value => { value.volumes[0].Labels['com.docker.compose.project'] = 'dvn-testing'; },
    value => { value.networks[0].Containers = { outsider: {} }; },
  ]) {
    const changed = structuredClone(original); change(changed);
    assert.throws(() => ownedResources(root, candidate, changed, () => 'a'.repeat(12)));
  }
  assert.throws(() => ownedResources(root, candidate, original, () => 'foreign-container'));
  assert.throws(() => testResources(() => { throw new Error('Docker unavailable'); }), /Docker unavailable/);
});

test('唯讀預覽不回收，執行時精確處理超額目錄並保留五組', async () => {
  const { root, base } = fixture();
  let recycled = 0;
  await cleanupDeploymentTests({ root, docker: noDocker, recycle: () => assert.fail('preview recycled data') });
  assert.equal(readdirSync(base).length, 7);
  const held = path.join(root, 'mock-system-trash'); mkdirSync(held);
  const report = await cleanupDeploymentTests({ root, apply: true, docker: noDocker, recycle: async directory => {
    const target = path.join(held, path.basename(directory));
    assert.equal(path.dirname(directory), base);
    assert.equal(path.dirname(target), held);
    renameSync(directory, target); recycled++;
  } });
  assert.equal(recycled, 2);
  assert.equal(readdirSync(base).length, 5);
  assert.ok(report.items.every(item => item.status === 'recycled'));
  assert.equal(JSON.parse(readFileSync(report.record)).items.length, 2);
});

test('回收失敗不降級永久刪除，也不繼續處理下一組', async () => {
  const { root, base } = fixture();
  let attempts = 0;
  const report = await cleanupDeploymentTests({ root, apply: true, docker: noDocker, removeHost: () => assert.fail('permanent fallback'), recycle: async () => { attempts++; throw new Error('recycle canceled'); } });
  assert.equal(attempts, 1);
  assert.equal(report.items[0].status, 'failed');
  assert.equal(report.items[1].status, 'planned');
  assert.equal(readdirSync(base).length, 7);
});

test('未授權永久移除時保留 Docker 與本機資料', async () => {
  const { root, names, base } = fixture(6);
  const resources = resourceFixture(root, names[0]);
  const docker = args => {
    assert.ok(!args.includes('rm'));
    if (args[0] === 'ps') return args.includes('--filter') ? 'a'.repeat(12) : `${'a'.repeat(12)} ${names[0]}-video-node-1 ${names[0]}`;
    if (args[1] === 'ls') return args[0] === 'volume' ? names[0] + '_node-data' : names[0] + '_default';
    return JSON.stringify(resources[args[0] === 'container' ? 'containers' : args[0] === 'volume' ? 'volumes' : 'networks']);
  };
  const report = await cleanupDeploymentTests({ root, apply: true, docker, recycle: () => assert.fail('blocked Docker project recycled') });
  assert.equal(report.items[0].status, 'blocked');
  assert.equal(readdirSync(base).length, 6);
});

test('明確啟用後只移除核對的 Docker 身分，且本機仍使用指定模式', async () => {
  const { root, names, base } = fixture(6);
  const state = resourceFixture(root, names[0]);
  const removed = [];
  const docker = args => {
    const kind = args[0] === 'container' ? 'containers' : args[0] === 'volume' ? 'volumes' : 'networks';
    if (args[1] === 'rm') {
      assert.equal(args.length, 3);
      removed.push(args);
      state[kind] = [];
      return '';
    }
    if (args[0] === 'ps') {
      if (args.includes('--filter')) return state.containers.length ? 'a'.repeat(12) : '';
      return state.containers.length ? `${'a'.repeat(12)} ${names[0]}-video-node-1 ${names[0]}` : '';
    }
    if (args[1] === 'ls') return state[kind].map(item => item.Name).join('\n');
    return JSON.stringify(state[kind]);
  };
  const held = path.join(root, 'mock-permanent-target'); mkdirSync(held);
  const report = await cleanupDeploymentTests({ root, apply: true, allowDockerRemoval: true, hostDataMode: 'permanent', docker,
    recycle: () => assert.fail('wrong host mode'), removeHost: directory => {
      assert.equal(path.dirname(directory), base);
      const destination = path.join(held, path.basename(directory));
      assert.equal(path.dirname(destination), held);
      renameSync(directory, destination);
    } });
  assert.deepEqual(removed, [['container', 'rm', 'a'.repeat(64)], ['network', 'rm', 'network-id'], ['volume', 'rm', names[0] + '_node-data']]);
  assert.equal(report.items[0].status, 'removed');
  assert.equal(readdirSync(base).length, 5);
});

test('部署入口轉交參數、失敗後仍執行保留階段且不隱藏退出碼', async () => {
  for (const status of [0, 7]) {
    let cleaned = false;
    const result = await runDeploymentTest(['--skip-build'], { run: (executable, args) => {
      assert.equal(executable, process.execPath);
      assert.equal(args.at(-1), '--skip-build');
      assert.equal(cleaned, false);
      return { status };
    }, cleanup: async options => { cleaned = true; assert.equal(options.apply, true); return { items: [] }; } });
    assert.equal(cleaned, true);
    assert.equal(result, status);
  }
  assert.equal(await runDeploymentTest([], { run: () => ({ status: 0 }), cleanup: async () => ({ items: [{ status: 'failed' }] }) }), 1);
  assert.equal(await runDeploymentTest([], { run: () => ({ status: 0 }), cleanup: async () => { throw new Error('Docker unavailable'); } }), 1);
});
