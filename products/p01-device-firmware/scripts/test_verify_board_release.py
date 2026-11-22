import hashlib
import importlib.util
import json
import tempfile
import subprocess
import sys
import time
import struct
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location('board_release', Path(__file__).with_name('verify_board_release.py'))
board = importlib.util.module_from_spec(spec)
spec.loader.exec_module(board)
evidence = board.evidence

class VerifyBoardRelease(unittest.TestCase):
 def test_signed_custom_provenance_and_rejections(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d); key=root/'key.pem'; pub=root/'pub.pem'
   evidence.openssl(['genpkey','-algorithm','ED25519','-out',key]); evidence.openssl(['pkey','-in',key,'-pubout','-out',pub])
   def file(name,obj):
    p=root/name; p.write_text(json.dumps(obj)); return {'file':name,'sha256':evidence.digest(p)}
   image_path,boot_public,measurement=synthetic_boot_image(root)
   image={'file':image_path.name,'sha256':evidence.digest(image_path)}; boot={'sha256':evidence.key_sha256(boot_public)}
   provenance=file('p01.json',{'channel':'custom','board':board.MODEL,'mcubootPublicKeySha256':boot['sha256'],
     'serialRecovery':{'hardwareSecurityCounter':7},'artifacts':{'app/zephyr/zephyr.signed.bin':image['sha256']}})
   commit='a'*40
   def artifact(identifier,ref):
    roles={}
    for role in ('provenance','sbom','rescan','regression','artifactSigning'):
     roles[role]=file(identifier+'-'+role+'.json',{'format':'nu54-'+role+'-v1','artifactSha256':ref['sha256'],
       'evidenceKind':'synthetic','passed':True,'sourceCommit':commit,'sourceClean':True})
    return {'id':identifier,'version':'1.0.0','target':board.MODEL,'payload':ref,'evidence':roles}
   manifest=root/'release.json'; manifest.write_text(json.dumps({'format':'nu54-release-evidence-v1','channel':'custom',
     'sourceCommit':commit,'sourceClean':True,'artifacts':[artifact('p01-image',image),artifact('p01-provenance',provenance)]}))
   sig=root/'release.sig'; evidence.openssl(['pkeyutl','-sign','-rawin','-inkey',key,'-in',manifest,'-out',sig])
   trust={'format':'nu54-release-trust-v1','channels':{'custom':{evidence.key_sha256(pub):['p01-image','p01-provenance']},'official':{}},'installers':{'custom':{},'official':{}}}
   result=board.check(manifest,sig,pub,trust,'custom',boot['sha256'],7)
   self.assertEqual(result['counter'],7)
   for channel, fingerprint, minimum in [('official',boot['sha256'],7),('custom','0'*64,7),('custom',boot['sha256'],8)]:
    with self.assertRaises(ValueError): board.check(manifest,sig,pub,trust,channel,fingerprint,minimum)
   wrong=root/'wrong.pem'; wrongpub=root/'wrong.pub.pem'
   evidence.openssl(['genpkey','-algorithm','ED25519','-out',wrong]); evidence.openssl(['pkey','-in',wrong,'-pubout','-out',wrongpub])
   with self.assertRaises(ValueError): board.check(manifest,sig,wrongpub,trust,'custom',boot['sha256'],7)
   release=json.loads(manifest.read_text()); release['artifacts'][0]['target']='different-model'; manifest.write_text(json.dumps(release))
   evidence.openssl(['pkeyutl','-sign','-rawin','-inkey',key,'-in',manifest,'-out',sig])
   with self.assertRaises(ValueError): board.check(manifest,sig,pub,trust,'custom',boot['sha256'],7)
   release['artifacts'][0]['target']=board.MODEL; manifest.write_text(json.dumps(release))
   evidence.openssl(['pkeyutl','-sign','-rawin','-inkey',key,'-in',manifest,'-out',sig])
   installer=root/'installer.pem'; installer_pub=root/'installer.pub.pem'
   evidence.openssl(['genpkey','-algorithm','ED25519','-out',installer]); evidence.openssl(['pkey','-in',installer,'-pubout','-out',installer_pub])
   trust['installers']['custom']={evidence.key_sha256(installer_pub):['p01-image']}
   trust_file=root/'trust.json'; trust_file.write_text(json.dumps(trust))
   now=int(time.time())
   report=file('observation.json',{'format':'nu54-board-observation-v1','observed':True,'model':board.MODEL,'channel':'custom',
     'candidate':evidence.digest(manifest),'device':'b'*64,'bootKey':boot['sha256'],'measurement':measurement,
     'counter':7,'generation':2,'challenge':'e'*64,'issuedAt':now,'expiresAt':now+120})
   record=root/'installation.json'; record.write_text(json.dumps({'format':'nu54-installation-v1','channel':'custom',
     'releaseSha256':evidence.digest(manifest),'artifactId':'p01-image','version':'1.0.0','target':board.MODEL,
     'artifactSha256':image['sha256'],'evidenceKind':'observed','report':report}))
   install_sig=root/'install.sig'; evidence.openssl(['pkeyutl','-sign','-rawin','-inkey',installer,'-in',record,'-out',install_sig])
   output=root/'handoff.json'
   command=[sys.executable,str(Path(board.__file__)), '--manifest',str(manifest),'--signature',str(sig),
     '--public-key',str(pub),'--trust',str(trust_file),'--channel','custom','--boot-key-sha256',boot['sha256'],
     '--minimum-counter','7','--boot-public',str(boot_public),'--installation',str(record),str(install_sig),str(installer_pub),
     '--attester-private',str(installer),'--receipt-out',str(output),'--installer-key',str(installer_pub),'--setup-out',str(root/'setup.json')]
   no_key=command[:]; pos=no_key.index('--boot-public'); del no_key[pos:pos+2]
   self.assertNotEqual(subprocess.run(no_key,capture_output=True).returncode,0)
   self.assertFalse(output.exists()); self.assertFalse((root/'setup.json').exists())
   wrong_key=command[:]; wrong_key[wrong_key.index('--boot-public')+1]=str(wrongpub)
   self.assertNotEqual(subprocess.run(wrong_key,capture_output=True).returncode,0)
   self.assertFalse(output.exists()); self.assertFalse((root/'setup.json').exists())
   self.assertEqual(subprocess.run(command,capture_output=True).returncode,0)
   setup=json.loads((root/'setup.json').read_text()); self.assertEqual(setup['measurement'],measurement)
   self.assertEqual(hashlib.sha256(bytes.fromhex('302a300506032b6570032100'+setup['publicKey'])).hexdigest(),evidence.key_sha256(installer_pub))
   result=json.loads(output.read_text()); payload=root/'receipt-payload'; payload.write_text(result['payload'])
   receipt_sig=root/'receipt.sig'; receipt_sig.write_bytes(bytes.fromhex(result['signature']))
   evidence.authenticate(payload.read_bytes(),receipt_sig,installer_pub.read_bytes())
   self.assertEqual(json.loads(result['payload'])['challenge'],'e'*64)
   # Reusing the destination is rejected, rather than overwriting an earlier handoff.
   self.assertNotEqual(subprocess.run(command,capture_output=True).returncode,0)
   image_path.write_text('tampered')
   with self.assertRaises(ValueError): board.check(manifest,sig,pub,trust,'custom',boot['sha256'],7)

class ReceiptAttestation(unittest.TestCase):
 def test_report_validation_binds_challenge_and_physical_evidence(self):
  report={'format':'nu54-board-observation-v1','observed':True,'model':board.MODEL,'channel':'custom',
    'candidate':'a'*64,'device':'b'*64,'bootKey':'c'*64,'measurement':'d'*64,
    'counter':7,'generation':2,'challenge':'e'*64,'issuedAt':100,'expiresAt':200}
  result=board.claim_from_observation(report, {'channel':'custom','releaseSha256':'a'*64,
    'bootKeySha256':'c'*64,'counter':7,'measurement':'d'*64}, now=150)
  self.assertEqual(result['evidence'],'observed')
  for update in ({'observed':False},{'device':'0'*64},{'counter':6},{'candidate':'f'*64},
                 {'model':'other'},{'measurement':'f'*64},{'expiresAt':150},{'challenge':'x'*64}):
   with self.assertRaises(ValueError): board.claim_from_observation({**report,**update},
    {'channel':'custom','releaseSha256':'a'*64,'bootKeySha256':'c'*64,'counter':7,'measurement':'d'*64},now=150)

class ProtectedCounter(unittest.TestCase):
 def test_protected_counter_must_be_unique_and_match_release(self):
  wire=bytearray(88); struct.pack_into('<IIHHI',wire,0,0x96F3B83D,0,32,12,32)
  struct.pack_into('<HHBBHI',wire,64,0x6908,12,0x50,0,4,7)
  self.assertEqual(board.image_measurement(bytes(wire),7),hashlib.sha256(wire[:76]).hexdigest())
  with self.assertRaises(ValueError): board.image_measurement(bytes(wire),8)
  wire[64]=0
  with self.assertRaises(ValueError): board.image_measurement(bytes(wire),7)
  duplicate=bytearray(96); struct.pack_into('<IIHHI',duplicate,0,0x96F3B83D,0,32,20,32)
  struct.pack_into('<HHBBHIBBHI',duplicate,64,0x6908,20,0x50,0,4,7,0x50,0,4,7)
  with self.assertRaises(ValueError): board.image_measurement(bytes(duplicate),7)

def synthetic_boot_image(root):
 """Non-executable test image using the real MCUboot Ed25519 trailer format."""
 private=root/'boot-private.pem'; public=root/'boot-public.pem'
 evidence.openssl(['genpkey','-algorithm','ED25519','-out',private]); evidence.openssl(['pkey','-in',private,'-pubout','-out',public])
 wire=bytearray(76); struct.pack_into('<IIHHI',wire,0,0x96F3B83D,0,32,12,32); wire[20]=1
 struct.pack_into('<HHBBHI',wire,64,0x6908,12,0x50,0,4,7)
 digest=hashlib.sha512(wire).digest(); pubder=evidence.openssl(['pkey','-pubin','-in',public,'-outform','DER'])
 digestfile=root/'digest'; digestfile.write_bytes(digest)
 signature=evidence.openssl(['pkeyutl','-sign','-rawin','-inkey',private,'-in',digestfile])
 def tlv(kind,value): return struct.pack('<BBH',kind,0,len(value))+value
 trailer=tlv(0x12,digest)+tlv(0x01,hashlib.sha512(pubder).digest())+tlv(0x24,signature)
 wire.extend(struct.pack('<HH',0x6907,len(trailer)+4)+trailer)
 path=root/'signed.bin'; path.write_bytes(wire)
 return path,public,hashlib.sha256(wire[:76]).hexdigest()

class DirectBootSignature(unittest.TestCase):
 def test_verifies_real_mcuboot_signature_not_only_signed_metadata(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d); image,key,measurement=synthetic_boot_image(root)
   result={'counter':7,'version':'1.0.0','measurement':measurement,'bootKeySha256':evidence.key_sha256(key),'imageSha256':evidence.digest(image)}
   board.verify_boot_signature(image,key,result)
   wrong=root/'wrong.pem'; wrongpub=root/'wrong.pub.pem'
   evidence.openssl(['genpkey','-algorithm','ED25519','-out',wrong]); evidence.openssl(['pkey','-in',wrong,'-pubout','-out',wrongpub])
   with self.assertRaises(ValueError): board.verify_boot_signature(image,wrongpub,result)
   raw=bytearray(image.read_bytes()); raw[-1]^=1; image.write_bytes(raw)
   result['imageSha256']=evidence.digest(image)
   with self.assertRaises(ValueError): board.verify_boot_signature(image,key,result)
