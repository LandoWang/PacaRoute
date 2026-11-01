#!/usr/bin/env python3
"""Real Codex actuator. Verdict requires client dispatch logs and matching wire tool results."""
import pathlib,subprocess,os,json,socket,time,signal,threading,queue,sys,argparse,shutil
parser=argparse.ArgumentParser();parser.add_argument("--binary",required=True);parser.add_argument("--credential-root",required=True);parser.add_argument("--codex",default=shutil.which("codex"));parser.add_argument("--upstream-model",default="gpt-6-astra");options=parser.parse_args()
assert options.codex,"Set --codex or install Codex CLI"
R=pathlib.Path.cwd();Q=R/('artifacts/pacaroute-verification/live-appserver-'+time.strftime('%Y%m%d-%H%M%S')+'-'+__import__('uuid').uuid4().hex[:6]);Q.mkdir(parents=True,exist_ok=True)
exe=pathlib.Path(options.binary).resolve()
credential_root=pathlib.Path(options.credential_root).resolve()
accounts=[{'id':'qa-api','name':'QA API','kind':'api-key'}]
sources=[{'id':'qa-unavailable','name':'Unavailable QA source','kind':'openai','endpoint':'https://127.0.0.1:1','account_id':'qa-api','auth_mode':'api-key','deployments':['gpt-6-astra']}]
pools=[{'id':'qa-codex','name':'QA Codex','agent':'codex','model':'gpt-6-astra','failover':True,'members':[{'id':'bad','source_id':'qa-unavailable','model':'gpt-6-astra','weight':1000000},{'id':'good','source_id':'copilot','model':options.upstream_model,'weight':1}]}]
for name,value in [('accounts',accounts),('sources',sources),('source-pools',pools)]: (Q/(name+'.json')).write_text(json.dumps(value))
env=dict(os.environ)
for k in list(env):
 if k.startswith(('ANTHROPIC_','CLAUDE_','CODEX_','COPILOT_BRIDGE_','PACAROUTE_')) or k=='CLAUDECODE':env.pop(k,None)
env['PATH']=str(pathlib.Path(options.codex).parent)+os.pathsep+env['PATH']
sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close();assert port!=8765
bridge=None

def start_bridge(label):
 global bridge
 log=(Q/(label+'.log')).open('wb');bridge=subprocess.Popen([str(exe),'serve','--home',str(Q),'--credential-root',str(credential_root),'--port',str(port),'--allow-local','--trace-dir',str(Q/'traces')],env=env,stdout=log,stderr=subprocess.STDOUT)
 for _ in range(100):
  if bridge.poll() is not None:raise RuntimeError('QA service exited')
  try:
   with socket.create_connection(('127.0.0.1',port),timeout=.3):break
  except OSError:time.sleep(.1)
 else:raise TimeoutError('QA startup')
 print('PacaRoute ready',label,'pid',bridge.pid,'port',port,flush=True)
def stop_bridge():
 global bridge
 if bridge and bridge.poll() is None:
  assert str(exe) in subprocess.check_output(['ps','-p',str(bridge.pid),'-o','command=']).decode()
  bridge.send_signal(signal.SIGINT)
  try:bridge.wait(timeout=35)
  except subprocess.TimeoutExpired:bridge.kill();bridge.wait()
 bridge=None
try:
 start_bridge('first')
 import queue
 home=Q/'codex-home';home.mkdir(exist_ok=True);db=Q/'codex-sqlite';db.mkdir(exist_ok=True)
 work=Q/'workspace';work.mkdir(exist_ok=True);(work/'numbers.json').write_text('[9, 2, 9, 5, 2, 11]')
 (home/'config.toml').write_text(f'''model_provider = "poolqa"
web_search = "disabled"
[features]
multi_agent = false
plugins = false
remote_plugin = false
[model_providers.poolqa]
name = "PacaRoute QA"
base_url = "http://127.0.0.1:{port}/pools/qa-codex/codex"
wire_api = "responses"
env_key = "BRIDGE_DUMMY_KEY"
''')
 ce=dict(env,CODEX_HOME=str(home),CODEX_SQLITE_HOME=str(db),RUST_LOG='warn',BRIDGE_DUMMY_KEY='pacaroute-local')
 ce['PATH']=str(pathlib.Path(options.codex).parent)+os.pathsep+ce['PATH']
 err=(Q/'appserver.stderr').open('w');out=(Q/'appserver.stdout').open('w');started=time.time()
 client=subprocess.Popen([options.codex,'app-server','--stdio'],cwd=work,env=ce,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=err,text=True,bufsize=1)
 messages=queue.Queue()
 def read_messages():
  for line in client.stdout:
   out.write(line);out.flush()
   try:messages.put(json.loads(line))
   except ValueError:pass
 threading.Thread(target=read_messages,daemon=True).start()
 def send(method,params=None,id=None):
  value={'method':method}
  if params is not None:value['params']=params
  if id is not None:value['id']=id
  client.stdin.write(json.dumps(value)+'\n');client.stdin.flush()
 def until(predicate):
  limit=time.time()+180
  while time.time()<limit:
   m=messages.get(timeout=max(1,limit-time.time()))
   if 'error' in m and 'id' in m:raise RuntimeError(str(m))
   if 'id' in m and 'method' in m:raise RuntimeError('Unexpected permission request: '+m['method'])
   if predicate(m):return m
  raise TimeoutError('app-server response')
 try:
  send('initialize',{'clientInfo':{'name':'codex_app_server_daemon','title':'PacaRoute Go Verification','version':'1.0.0'}},1);until(lambda m:m.get('id')==1);send('initialized')
  send('thread/start',{'model':'gpt-6-astra','modelProvider':'poolqa','cwd':str(work),'approvalPolicy':'never','sandbox':'danger-full-access','config':{'model_reasoning_effort':'low'}},2)
  thread=until(lambda m:m.get('id')==2)['result']['thread']['id']
  def turn(id,prompt):
   send('turn/start',{'threadId':thread,'input':[{'type':'text','text':prompt}]},id);until(lambda m:m.get('id')==id)
   result=until(lambda m:m.get('method')=='turn/completed' and m.get('params',{}).get('threadId')==thread)
   assert result['params']['turn']['status']=='completed',result
   print('App-server turn completed',id,flush=True)
  turn(3,'Work only here. Read numbers.json. Write analyze.py that writes result.json with the sorted unique values and their sum. Execute it. Read result.json in a separate tool call and verify it. Finish PACAROUTE_APPSERVER_OK with the values. Do not use subagents or network.')
  rows=[json.loads(l) for l in (Q/'affinity-v2.jsonl').read_text().splitlines()];assert len(rows)==1 and rows[0]['member']=='good',rows
  pools[0]['members'][0]['weight']=1;pools[0]['members'][1]['weight']=0;(Q/'source-pools.json').write_text(json.dumps(pools));before=(Q/'affinity-v2.jsonl').read_bytes()
  stop_bridge();start_bridge('bridge-restarted')
  turn(4,'Continue. Read analyze.py, extend its output with the original input count, execute it, then read result.json in a separate tool call. Finish PACAROUTE_APPSERVER_RESUME_OK and the count.')
  assert (Q/'affinity-v2.jsonl').read_bytes()==before,'Sticky binding changed after restart'
  send('thread/delete',{'threadId':thread},1000000);until(lambda m:m.get('id')==1000000)
  client.stdin.close();client.wait(timeout=20);assert client.returncode==0
  (Q/'appserver.manifest.json').write_text(json.dumps({'start':int(started)-2,'end':int(time.time())+2,'dispatchLogPath':str(db/'logs_2.sqlite'),'stdoutPath':str(Q/'appserver.stdout'),'upstream_model':options.upstream_model,'binaryPath':str(exe),'binarySHA256':__import__('hashlib').sha256(exe.read_bytes()).hexdigest()}))
  print('Manifest:',Q/'appserver.manifest.json',flush=True)
 finally:
  if client.poll() is None:client.terminate();client.wait(timeout=10)
  out.close();err.close()

finally:
 stop_bridge()
