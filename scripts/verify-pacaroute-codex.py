#!/usr/bin/env python3
"""Real Codex actuator. Verdict requires client dispatch logs and matching wire tool results."""
import pathlib,subprocess,os,json,socket,time,signal,threading,queue,sys,argparse,shutil
parser=argparse.ArgumentParser();parser.add_argument("--binary",required=True);parser.add_argument("--credential-root",required=True);parser.add_argument("--codex",default=shutil.which("codex"));parser.add_argument("--upstream-model",default="gpt-6-astra");parser.add_argument("--source-id",default="copilot");parser.add_argument("--source-config-root");parser.add_argument("--azure-cli",action="store_true");parser.add_argument("--connection-script",action="store_true");parser.add_argument("--stream-failure",choices=["eof","error"]);parser.add_argument("--import-pool",action="store_true");parser.add_argument("--peer",action="store_true");parser.add_argument("--legacy-local",action="store_true");parser.add_argument("--billing",action="store_true");options=parser.parse_args()
assert not options.legacy_local or not (options.peer or options.import_pool or options.connection_script), "legacy-local requires a direct local Client"
assert options.codex,"Set --codex or install Codex CLI"
R=pathlib.Path.cwd();Q=R/('artifacts/pacaroute-verification/live-appserver-'+time.strftime('%Y%m%d-%H%M%S')+'-'+__import__('uuid').uuid4().hex[:6]);Q.mkdir(parents=True,exist_ok=True)
exe=pathlib.Path(options.binary).resolve()
credential_root=pathlib.Path(options.credential_root).resolve()
accounts=[{'id':'qa-api','name':'QA API','kind':'api-key'}]
sources=[{'id':'qa-unavailable','name':'Unavailable QA source','kind':'openai','endpoint':'https://127.0.0.1:1','account_id':'qa-api','auth_mode':'api-key','deployments':['gpt-6-astra']}]
pools=[{'id':'qa-codex','name':'QA Codex','agent':'codex','model':'gpt-6-astra','failover':True,'members':[{'id':'bad','source_id':'qa-unavailable','model':'gpt-6-astra','weight':1000000},{'id':'good','source_id':'copilot','model':options.upstream_model,'weight':1}]}]
clients=[{'id':'qa-codex','name':'QA Codex','enabled_agents':['codex'],'key_hash':__import__('hashlib').sha256(b'qa-client-synthetic-key').hexdigest(),'routes':[{'id':'coding','agent':'codex','match_model':'gpt-6-astra','source_id':'pool:qa-codex','model':'gpt-6-astra'}]}]
if options.legacy_local:
 clients[0].update(id='default',local=True);clients[0].pop('key_hash')
if options.source_id!='copilot':
 source_root=pathlib.Path(options.source_config_root)
 selected=next(v for v in json.loads((source_root/'sources.json').read_text()) if v['id']==options.source_id)
 source_account=next(v for v in json.loads((source_root/'accounts.json').read_text()) if v['id']==selected['account_id'])
 if options.azure_cli:
  selected=dict(selected,auth_mode='azure-cli');source_account=dict(source_account,login_mode='azure-cli')
 sources.append(selected);accounts.append(source_account);pools[0]['members'][1]['source_id']=selected['id']
if options.import_pool:
 pools[0]["multi_model"]=True
 pools[0]["members"].append({"id":"other-model","source_id":"copilot","model":"wrong-model-never-call","alias":"qa-other","weight":1000000})
fixture=None
runtime_exe=exe
if options.stream_failure:
 from verify_stream_fixture import StreamFixture
 fixture=StreamFixture(R,Q,options.stream_failure)
 runtime_exe=fixture.binary
 sources[0]['endpoint']=fixture.endpoint
for name,value in [('clients',clients),('accounts',accounts),('sources',sources),('source-pools',pools)]: (Q/(name+'.json')).write_text(json.dumps([] if name=='clients' and options.connection_script else value))
env=dict(os.environ)
for k in list(env):
 if k.startswith(('ANTHROPIC_','CLAUDE_','CODEX_','COPILOT_BRIDGE_','PACAROUTE_')) or k=='CLAUDECODE':env.pop(k,None)
env['PATH']=str(pathlib.Path(options.codex).parent)+os.pathsep+env['PATH']
sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close();assert port!=8765
bridge=None
peer=None

def start_bridge(label):
 global bridge
 log=(Q/(label+'.log')).open('wb');bridge=subprocess.Popen([str(runtime_exe),'serve','--home',str(Q),'--credential-root',str(credential_root),'--port',str(port),'--allow-local','--trace-dir',str(Q/'traces')],env=dict(env,**(dict(fixture.env,COPILOT_BRIDGE_SECRETS_STDIN='1') if fixture else {})),stdin=subprocess.PIPE if fixture else None,stdout=log,stderr=subprocess.STDOUT)
 if fixture:
  bridge.stdin.write(b'{"qa-unavailable":"synthetic-fixture-only"}\n');bridge.stdin.flush()
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
  assert str(runtime_exe) in subprocess.check_output(['ps','-p',str(bridge.pid),'-o','command=']).decode()
  bridge.send_signal(signal.SIGINT)
  try:bridge.wait(timeout=35)
  except subprocess.TimeoutExpired:bridge.kill();bridge.wait()
 bridge=None
try:
 start_bridge('first')
 client_key='copilot-bridge-provider' if options.legacy_local else 'qa-client-synthetic-key'
 if options.billing:
  from verify_quota_setup import configure_billing
  configure_billing(Q,port,options.upstream_model)
 if options.import_pool:
  from verify_config_import import imported_pool
  imported_client,pools,client_key=imported_pool(Q,port,'qa-codex')
  clients=[imported_client]
 client_port=port
 if options.peer:
  from verify_peer_hop import PeerHop
  peer=PeerHop(exe,Q,port,client_key,'codex','gpt-6-astra',env)
  client_port,client_key=peer.port,peer.key
 import queue
 home=Q/'codex-home';home.mkdir(exist_ok=True);db=Q/'codex-sqlite';db.mkdir(exist_ok=True)
 work=Q/'workspace';work.mkdir(exist_ok=True);(work/'numbers.json').write_text('[9, 2, 9, 5, 2, 11]')
 (home/'config.toml').write_text('web_search = "disabled"\n[features]\nmulti_agent = false\nplugins = false\nremote_plugin = false\n')
 if options.connection_script:
  from verify_connection_setup import configure
  setup=configure(exe,Q,port,clients[0],'codex','gpt-6-astra',dict(env,CODEX_HOME=str(home)))
 else:
  setup=subprocess.run([str(exe),'connect','--agent','codex','--url',f'http://127.0.0.1:{client_port}','--model','gpt-6-astra','--apply'],input=client_key.encode(),env=dict(env,CODEX_HOME=str(home)),capture_output=True,check=True)
 (Q/'connect-result.json').write_bytes(setup.stdout)
 if options.legacy_local:
  config=home/'config.toml';lines=[line for line in config.read_text().splitlines() if not line.startswith('http_headers =')]
  config.write_text('\n'.join(lines)+'\n[model_providers.pacaroute.auth]\ncommand = \"/usr/bin/printf\"\nargs = [\"copilot-bridge-provider\"]\ntimeout_ms = 5000\nrefresh_interval_ms = 0\n')
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
  send('thread/start',{'model':'gpt-6-astra','modelProvider':'pacaroute','cwd':str(work),'approvalPolicy':'never','sandbox':'danger-full-access','config':{'model_reasoning_effort':'low'}},2)
  thread=until(lambda m:m.get('id')==2)['result']['thread']['id']
  def turn(id,prompt):
   send('turn/start',{'threadId':thread,'input':[{'type':'text','text':prompt}]},id);until(lambda m:m.get('id')==id)
   result=until(lambda m:m.get('method')=='turn/completed' and m.get('params',{}).get('threadId')==thread)
   assert result['params']['turn']['status']=='completed',result
   print('App-server turn completed',id,flush=True)
  turn(3,'Work only here. Read numbers.json. Write analyze.py that writes result.json with the sorted unique values and their sum. Execute it. Read result.json in a separate tool call and verify it. Finish PACAROUTE_APPSERVER_OK with the values. Do not use subagents or network.')
  if fixture:assert fixture.calls==1, f'Failed stream selected {fixture.calls} times'
  rows=[json.loads(l) for l in (Q/'affinity-v2.jsonl').read_text().splitlines()]
  expected_members=['bad','good'] if fixture else ['good']
  assert [r['member'] for r in rows]==expected_members and len({r['key'] for r in rows})==1,rows
  pools[0]['members'][0]['weight']=1;pools[0]['members'][1]['weight']=0
  # Exercise the revisioned SQLite writer rather than the one-time JSON import.
  cfg=json.loads(subprocess.check_output([str(exe),'config','show','--home',str(Q)],env=env));cfg['pools']=pools
  subprocess.run([str(exe),'config','apply','--home',str(Q)],input=json.dumps(cfg).encode(),env=env,capture_output=True,check=True)
  before=(Q/'affinity-v2.jsonl').read_bytes()
  stop_bridge();start_bridge('bridge-restarted')
  if peer:peer.cycle()
  turn(4,'Continue. Read analyze.py, extend its output with the original input count, execute it, then read result.json in a separate tool call. Finish PACAROUTE_APPSERVER_RESUME_OK and the count.')
  assert (Q/'affinity-v2.jsonl').read_bytes()==before,'Sticky binding changed after restart'
  send('thread/delete',{'threadId':thread},1000000);until(lambda m:m.get('id')==1000000)
  client.stdin.close();client.wait(timeout=20);assert client.returncode==0
  (Q/'appserver.manifest.json').write_text(json.dumps({'start':int(started)-2,'end':int(time.time())+2,'dispatchLogPath':str(db/'logs_2.sqlite'),'stdoutPath':str(Q/'appserver.stdout'),'upstream_model':options.upstream_model,'source_id':options.source_id,'azure_cli':options.azure_cli,'legacy_local':options.legacy_local,'billing':options.billing,'peer':options.peer,'peer_root':str(peer.root) if peer else None,'import_pool':options.import_pool,'connection_script':options.connection_script,'stream_failure':options.stream_failure,'fixture_calls':fixture.calls if fixture else 0,'packagedBinaryPath':str(exe),'binaryPath':str(runtime_exe),'binarySHA256':__import__('hashlib').sha256(runtime_exe.read_bytes()).hexdigest()}))
  print('Manifest:',Q/'appserver.manifest.json',flush=True)
 finally:
  if client.poll() is None:client.terminate();client.wait(timeout=10)
  out.close();err.close()

finally:
 if peer:peer.close()
 stop_bridge()
 if fixture:fixture.close()
