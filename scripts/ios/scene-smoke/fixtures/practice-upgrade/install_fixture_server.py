#!/usr/bin/env python3
"""Loopback-only synthetic P10 board installation fixture; never a real attester."""
import json, os, signal, sys, tempfile, time
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlsplit

sys.dont_write_bytecode = True
repo=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(repo/'scripts'))
from integration_workspace import run_command, run_environment
sys.path.insert(0,str(repo/'products/p10-platform/release'))
import evidence
sys.path.insert(0,str(repo/'products/p01-device-firmware/scripts'))
from test_verify_board_release import synthetic_boot_image
cli=repo/'products/p01-device-firmware/scripts/verify_board_release.py'
model='nu54v_dk/nrf54l15/cpuapp'
def serve(root):
 env=run_environment(root,os.environ)
 def key(name):
  private=root/(name+'.pem'); public=root/(name+'.pub.pem')
  evidence.openssl(['genpkey','-algorithm','ED25519','-out',private]); evidence.openssl(['pkey','-in',private,'-pubout','-out',public]); return private,public
 def save(name,data):
  p=root/name; p.write_text(json.dumps(data)); return {'file':name,'sha256':evidence.digest(p)}
 def sign(path,key):
  sig=path.with_suffix('.sig'); evidence.openssl(['pkeyutl','-sign','-rawin','-inkey',key,'-in',path,'-out',sig]); return sig
 release_key,release_pub=key('release'); installer,installer_pub=key('installer')
 image_path,boot_public,measurement=synthetic_boot_image(root)
 image={'file':image_path.name,'sha256':evidence.digest(image_path)}; boot=evidence.key_sha256(boot_public)
 provenance=save('p01.json',{'channel':'custom','board':model,'mcubootPublicKeySha256':boot,'serialRecovery':{'hardwareSecurityCounter':7},'artifacts':{'app/zephyr/zephyr.signed.bin':image['sha256']}})
 def artifact(name,ref):
  roles={}
  for role in ('provenance','sbom','rescan','regression','artifactSigning'):
   roles[role]=save(name+'-'+role+'.json',{'format':'nu54-'+role+'-v1','artifactSha256':ref['sha256'],'evidenceKind':'synthetic','passed':True,'sourceCommit':'a'*40,'sourceClean':True})
  return {'id':name,'version':'1.0.0','target':model,'payload':ref,'evidence':roles}
 manifest=root/'release.json'; manifest.write_text(json.dumps({'format':'nu54-release-evidence-v1','channel':'custom','sourceCommit':'a'*40,'sourceClean':True,'artifacts':[artifact('p01-image',image),artifact('p01-provenance',provenance)]}))
 sig=sign(manifest,release_key)
 trust={'format':'nu54-release-trust-v1','channels':{'custom':{evidence.key_sha256(release_pub):['p01-image','p01-provenance']},'official':{}},'installers':{'custom':{evidence.key_sha256(installer_pub):['p01-image']},'official':{}}}
 trustfile=root/'trust.json'; trustfile.write_text(json.dumps(trust))
 base=[sys.executable,str(cli),'--manifest',str(manifest),'--signature',str(sig),'--public-key',str(release_pub),'--trust',str(trustfile),'--channel','custom','--boot-key-sha256',boot,'--minimum-counter','7','--boot-public',str(boot_public)]
 setupfile=root/'setup.json'; run_command(base+['--installer-key',str(installer_pub),'--setup-out',str(setupfile)],check=True,capture_output=True,env=env)
 setup=json.loads(setupfile.read_text()); device='b'*64
 folder=Path(os.environ['W03_FIXTURE_DIR']) if 'W03_FIXTURE_DIR' in os.environ else None
 if folder:
  folder.mkdir(parents=True,exist_ok=True)
  (folder/'setup.json').write_text(json.dumps(setup))
  (folder/'trust.json').write_text(json.dumps({'signer':evidence.key_sha256(installer_pub),'candidate':setup['candidate'],'device':device}))
  (folder/'not-json.txt').write_text('fixture')
  (folder/'oversize.json').write_text('x'*8193)
 class Handler(BaseHTTPRequestHandler):
  def do_GET(self):
   try:
    u=urlsplit(self.path)
    if u.path=='/setup': result={'setup':setup,'signer':evidence.key_sha256(installer_pub),'candidate':setup['candidate'],'device':device}
    elif u.path=='/receipt':
     nonce=parse_qs(u.query)['challenge'][0]; now=int(time.time())
     report=save('observation.json',{'format':'nu54-board-observation-v1','observed':True,'model':model,'channel':'custom','candidate':setup['candidate'],'device':('f'*64 if parse_qs(u.query).get('wrong') == ['1'] else device),'bootKey':boot,'measurement':setup['measurement'],'counter':7,'generation':0,'challenge':nonce,'issuedAt':now,'expiresAt':now+180})
     record=root/'installation.json'; record.write_text(json.dumps({'format':'nu54-installation-v1','channel':'custom','releaseSha256':setup['candidate'],'artifactId':'p01-image','version':'1.0.0','target':model,'artifactSha256':image['sha256'],'evidenceKind':'observed','report':report}))
     installsig=sign(record,installer); output=root/('handoff-'+nonce+('-wrong' if parse_qs(u.query).get('wrong') == ['1'] else '')+'.json')
     run_command(base+['--installation',str(record),str(installsig),str(installer_pub),'--attester-private',str(installer),'--receipt-out',str(output)],check=True,capture_output=True,env=env)
     result=json.loads(output.read_text())
     if folder: (folder/('wrong.json' if parse_qs(u.query).get('wrong') == ['1'] else 'receipt.json')).write_text(json.dumps(result))
    else: self.send_error(404); return
    data=json.dumps(result,separators=(',',':')).encode(); self.send_response(200); self.send_header('Content-Type','application/json'); self.send_header('Content-Length',str(len(data))); self.end_headers(); self.wfile.write(data)
   except Exception as e: self.send_error(500,str(e))
  def log_message(self,*args): pass
 with HTTPServer(('127.0.0.1',18941),Handler) as server:
  print('SYNTHETIC FIXTURE ONLY on 127.0.0.1:18941',flush=True)
  server.serve_forever()


def terminate(signum, _frame):
 raise SystemExit(128 + signum)


if __name__ == '__main__':
 signal.signal(signal.SIGTERM, terminate)
 try:
  with tempfile.TemporaryDirectory(prefix='nu54-synthetic-install-') as scratch:
   serve(Path(scratch))
 except KeyboardInterrupt:
  sys.exit(130)
