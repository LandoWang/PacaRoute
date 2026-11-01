#!/usr/bin/env python3
"""Real Claude actuator; inspect its complete answer and tool_use/tool_result pairs."""
import pathlib,subprocess,os,json,socket,time,signal,sys,argparse,shutil
parser=argparse.ArgumentParser();parser.add_argument("--binary",required=True);parser.add_argument("--credential-root",required=True);parser.add_argument("--claude",default=shutil.which("claude"));parser.add_argument("--mode",choices=["native","cross"],default="native");options=parser.parse_args()
assert options.claude,"Set --claude or install Claude Code"
R=pathlib.Path.cwd();kind=options.mode;Q=R/'artifacts/pacaroute-verification'/('live-claude-'+kind+'-'+time.strftime('%Y%m%d-%H%M%S')+'-'+__import__('uuid').uuid4().hex[:6]);Q.mkdir(parents=True,exist_ok=True)
exe=pathlib.Path(options.binary).resolve();credential_root=pathlib.Path(options.credential_root).resolve()
model='claude-opus-5.5' if kind=='native' else 'gpt-6-astra'
# Claude requests a stable alias; Client routing selects the actual provider/model.
clients=[{'id':'qa-claude','name':'QA Claude','key_hash':__import__('hashlib').sha256(b'qa-client-synthetic-key').hexdigest(),'routes':[{'id':'coding','agent':'claude-code','match_model':'claude-opus-5.5','source_id':'copilot','model':model}]}]
(Q/'clients.json').write_text(json.dumps(clients))
env=dict(os.environ)
for k in list(env):
 if k.startswith(('ANTHROPIC_','CLAUDE_','CODEX_','COPILOT_BRIDGE_','PACAROUTE_')) or k=='CLAUDECODE':env.pop(k,None)
env['PATH']=str(pathlib.Path(options.claude).parent)+os.pathsep+env['PATH']
sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close();assert port!=8765
bridge=None
try:
 log=(Q/'bridge.log').open('wb');bridge=subprocess.Popen([str(exe),'serve','--home',str(Q),'--credential-root',str(credential_root),'--port',str(port),'--trace-dir',str(Q/'traces')],env=env,stdout=log,stderr=subprocess.STDOUT)
 for _ in range(100):
  if bridge.poll() is not None:raise RuntimeError('QA service exited')
  try:
   with socket.create_connection(('127.0.0.1',port),timeout=.3):break
  except OSError:time.sleep(.1)
 else:raise TimeoutError('QA startup')
 print('PacaRoute Claude',kind,'pid',bridge.pid,'port',port,flush=True)
 work=Q/'workspace';work.mkdir(exist_ok=True);(work/'numbers.json').write_text('[9, 2, 9, 5, 2, 11]');home=Q/'claude-home';home.mkdir(exist_ok=True)
 ce=dict(env,CLAUDE_CONFIG_DIR=str(home),ANTHROPIC_BASE_URL=f'http://127.0.0.1:{port}/cc',ANTHROPIC_API_KEY='qa-client-synthetic-key',ANTHROPIC_MODEL='claude-opus-5.5',CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC='1')
 prompt='Work only here. Read numbers.json with Read. Write analyze.py with Write; it must compute a frequency map and the sum of unique numbers into result.json. Run it using Bash, then read result.json in a separate Read tool call. Verify the result and finish PACAROUTE_CLAUDE_OK with the computed values. Do not use subagents or network.'
 started=time.time()
 with (Q/'claude.stdout').open('wb') as out,(Q/'claude.stderr').open('wb') as err:
  result=subprocess.run([options.claude,'--bare','--print','--output-format','stream-json','--verbose','--dangerously-skip-permissions','--model','claude-opus-5.5','--tools','Read,Write,Edit,Bash','--',prompt],cwd=work,env=ce,stdout=out,stderr=err,timeout=300)
 (Q/'manifest.json').write_text(json.dumps({'start':started,'end':time.time(),'exit_code':result.returncode,'upstream_model':model,'stdoutPath':str(Q/'claude.stdout'),'binaryPath':str(exe),'binarySHA256':__import__('hashlib').sha256(exe.read_bytes()).hexdigest()}))
 print('Claude',kind,'exit',result.returncode,'duration',round(time.time()-started),flush=True)
 assert result.returncode==0,'Claude failed; inspect transcript'
 print('Manifest:',Q/'manifest.json',flush=True)
finally:
 if bridge and bridge.poll() is None:
  assert str(exe) in subprocess.check_output(['ps','-p',str(bridge.pid),'-o','command=']).decode()
  bridge.send_signal(signal.SIGINT)
  try:bridge.wait(timeout=35)
  except subprocess.TimeoutExpired:bridge.kill();bridge.wait()
