const {spawn, spawnSync} = require('child_process');
const path = require('path');
const fs = require('fs');
const windows = process.platform === 'win32';
const root = path.resolve(__dirname, '..');
const bin = path.join(root, windows ? 'dist/wh.exe' : 'dist/wh');
const test = path.join(root, 'docs/temp/lifecycle-' + Date.now());
fs.mkdirSync(test, {recursive:true});
const hostEnv = {...process.env, WH_HOME:path.join(test,'host')};
const clientEnv = {...process.env, WH_HOME:path.join(test,'client')};
const host = spawn(bin,['open'],{env:hostEnv});
let output = '';
host.stdout.on('data',b=>output+=b);
host.stderr.on('data',b=>process.stderr.write(b));
function call(args) {
 const r=spawnSync(bin,args,{env:clientEnv,encoding:'utf8',timeout:90000});
 if(r.error) throw r.error;
 return r;
}
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function waitFor(test,description) {
 for(let i=0;i<80;i++) {if(test())return;await delay(500);}
 throw Error('Timed out: '+description);
}
(async()=>{
 let command;
 try {
  await waitFor(()=>output.includes('CODE:'),'pairing');
  const code=output.match(/CODE: (\S+)/)[1];
  const paired=call(['connect',code]);
  if(paired.status!==0)throw Error(paired.stderr);
  const alive=call(['exec','help','--',windows ? "Start-Sleep -Seconds 27; Write-Output alive" : 'sleep 27; printf alive']);
  if(alive.status!==0||!alive.stdout.includes('alive'))throw Error('Keepalive broke a healthy command: '+alive.stderr);
  console.log('PASS healthy command survives keepalive');
  command=spawn(bin,['exec','help','--',windows ? "[Console]::Out.Write('READY'); Start-Sleep -Seconds 60" : 'printf READY; sleep 60'],{env:clientEnv});
  let stdout='',stderr='';
  command.stdout.on('data',b=>stdout+=b);
  command.stderr.on('data',b=>stderr+=b);
  await waitFor(()=>stdout.includes('READY'),'running command');
  host.kill('SIGKILL');
  const start=Date.now();
  await waitFor(()=>command.exitCode!==null,'command detects host loss');
  if(command.exitCode===0||Date.now()-start>28000)throw Error('Host loss did not fail promptly: '+stderr);
  if(call(['forget','help']).status!==0)throw Error('Could not erase invalid credentials');
  console.log('PASS forced host loss stops the client within 28 seconds; credentials erased');
 } finally {
  host.kill('SIGKILL');
  if(command)command.kill('SIGKILL');
 }
})().catch(e=>{console.error(e);process.exitCode=1});
