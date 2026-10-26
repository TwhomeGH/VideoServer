import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';

const services = ['video-node', 'central', 'transcode-worker'];
const service = process.argv[2];
if (!services.includes(service)) {
  throw new Error(`請指定服務：${services.join(', ')}`);
}

mkdirSync('bin', { recursive: true });
const targetOS = process.env.GOOS || (process.platform === 'win32' ? 'windows' : process.platform);
const targetArch = process.env.GOARCH || (process.arch === 'x64' ? 'amd64' : process.arch);
const crossCompile = Boolean(process.env.GOOS || process.env.GOARCH);
const output = crossCompile ? `bin/${targetOS}-${targetArch}` : 'bin';
mkdirSync(output, { recursive: true });
const suffix = targetOS === 'windows' ? '.exe' : '';
const buildTime = new Date().toISOString();
execFileSync('go', [
  'build', '-buildvcs=true',
  '-ldflags', `-X github.com/TwhomeGH/VideoServer/pkg/buildinfo.BuildTime=${buildTime}`,
  '-o', `${output}/${service}${suffix}`, `./apps/${service}`,
], { stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0' } });
