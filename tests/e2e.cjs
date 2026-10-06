const { spawn, spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

const windows = process.platform === 'win32';
const root = path.resolve(__dirname, '..');
const bin = path.join(root, windows ? 'dist/wh.exe' : 'dist/wh');
const test = path.join(root, 'docs/temp/e2e-' + Date.now());
fs.mkdirSync(test, { recursive: true });
const hostHome = path.join(test, 'host');
const clientHome = path.join(test, 'client');
const env = home => ({ ...process.env, WH_HOME: home });

function run(args, home = clientHome) {
 const result = spawnSync(bin, args, { env: env(home), encoding: 'utf8', timeout: 90000 });
 console.log(args[0], result.status, result.stdout, result.stderr);
 if (result.error) throw result.error;
 return result;
}
function assert(condition, message) { if (!condition) throw Error(message); }
const host = spawn(bin, ['open'], { env: env(hostHome), stdio: ['pipe', 'pipe', 'pipe'] });
let output = '', error = '';
host.stdout.on('data', bytes => { output += bytes; process.stdout.write(bytes); });
host.stderr.on('data', bytes => { error += bytes; process.stderr.write(bytes); });

async function waitCode() {
 for (let i = 0; i < 80; i++) {
  const match = output.match(/CODE: (\S+)/);
  if (match) return match[1];
  if (host.exitCode !== null) throw Error('Host exited: ' + host.exitCode + ' ' + error);
  await new Promise(resolve => setTimeout(resolve, 500));
 }
 throw Error('Code timeout');
}

(async () => {
 try {
  const code = await waitCode();
  assert(run(['open'], hostHome).status === 0, 'Repeated open');
  assert(run(['connect', code]).status === 0, 'Connect');
  assert(run(['connect', code]).status === 0, 'Repeated connect');
  const command = windows
   ? "[Console]::Out.Write('OUT'); [Console]::Error.Write('ERR'); exit 37"
   : 'printf OUT; printf ERR >&2; exit 37';
  const result = run(['exec', 'help', '--', command]);
  assert(result.status === 37 && result.stdout === 'OUT' && result.stderr === 'ERR', 'Streams/exit status');
  const bytes = crypto.randomBytes(65536);
  const file = path.join(test, 'input.bin');
  const remote = path.join(test, 'remote Ω.bin');
  const dest = path.join(test, 'output.bin');
  fs.writeFileSync(file, bytes);
  assert(run(['put', 'help', file, remote]).status === 0, 'Upload');
  assert(run(['get', 'help', remote, dest]).status === 0, 'Download');
  assert(fs.readFileSync(dest).equals(bytes), 'File bytes');
  const config = path.join(clientHome, 'help/ssh_config');
  const native = spawnSync('ssh', ['-F', config, 'help', windows ? 'Write-Output native-ok' : 'printf native-ok'], { encoding: 'utf8', timeout: 60000 });
  assert(native.status === 0 && native.stdout.includes('native-ok'), 'Native SSH: ' + native.stderr);
  assert(run(['close', 'help']).status === 0, 'Close');
  await new Promise(resolve => setTimeout(resolve, 1000));
  assert(host.exitCode === 0, 'Host did not exit normally');
  assert(!fs.existsSync(path.join(clientHome, 'help')), 'Credentials remain');
  assert(!fs.existsSync(path.join(hostHome, 'host.json')), 'Host marker remains');
  console.log('PASS pairing, repeated open/connect, streams, exit37, SFTP, native SSH, close and cleanup');
 } finally {
  host.kill();
  fs.writeFileSync(path.join(test, 'result.log'), output + '\n' + error);
 }
})().catch(error => { console.error(error); process.exitCode = 1; });
