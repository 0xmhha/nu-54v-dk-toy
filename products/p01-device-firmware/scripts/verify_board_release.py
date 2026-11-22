#!/usr/bin/env python3
"""Check a P10-signed custom board release before the separate USB install.

This is preflight, not evidence of a device having booted. Trust policy and boot
fingerprint must be obtained independently of the release bundle.
"""
import argparse
import json
import re
import sys
import hashlib
import tempfile
import struct
import subprocess
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'p10-platform' / 'release'))
import evidence

MODEL = 'nu54v_dk/nrf54l15/cpuapp'


def image_measurement(wire, expected_counter):
    if len(wire) < 32:
        raise ValueError('invalid MCUboot image')
    magic, _, header, protected, size = struct.unpack_from('<IIHHI', wire)
    start, end = header + size, header + size + protected
    if magic != 0x96F3B83D or header < 32 or size < 1 or protected < 4 or end >= len(wire):
        raise ValueError('invalid counted MCUboot image')
    info_magic, total = struct.unpack_from('<HH',wire,start)
    if info_magic != 0x6908 or total != protected:
        raise ValueError('invalid protected MCUboot TLV region')
    cursor=start+4; counters=[]
    while cursor < end:
        if cursor+4 > end: raise ValueError('truncated protected MCUboot TLV')
        tag, pad, length = struct.unpack_from('<BBH',wire,cursor); cursor += 4
        if pad != 0 or cursor+length > end: raise ValueError('invalid protected MCUboot TLV')
        if tag == 0x50:
            if length != 4: raise ValueError('invalid security counter TLV')
            counters.append(struct.unpack_from('<I',wire,cursor)[0])
        cursor += length
    if cursor != end or counters != [expected_counter]:
        raise ValueError('protected security counter differs from signed provenance')
    return hashlib.sha256(wire[:end]).hexdigest()


def check(manifest, signature, public, trust, channel, boot_fingerprint, minimum_counter):
    if channel == 'official':
        policy_path = Path(__file__).with_name('release-policy.json')
        approved_boot = evidence.read(policy_path).get('officialPublicKeySha256')
        if not approved_boot or approved_boot != boot_fingerprint:
            raise ValueError('official board boot trust is not provisioned')
    if channel not in ('custom', 'official') or not re.fullmatch(r'[0-9a-f]{64}', boot_fingerprint):
        raise ValueError('invalid channel or independently verified boot fingerprint')
    if not isinstance(minimum_counter, int) or minimum_counter < 1:
        raise ValueError('minimum counter must be positive')
    # Verify exact manifest bytes, artifact hashes, all signed evidence and key/channel scope.
    evidence.verify(manifest, signature, public, trust, channel)
    release = evidence.read(manifest)
    artifacts = {item['id']: item for item in release['artifacts']}
    if not {'p01-image', 'p01-provenance'} <= artifacts.keys():
        raise ValueError('board image or signed provenance is missing')
    image, provenance = artifacts['p01-image'], artifacts['p01-provenance']
    if image['target'] != MODEL or provenance['target'] != MODEL or image['version'] != provenance['version']:
        raise ValueError('board model or version mismatch')
    build = evidence.read(evidence.resolve(manifest.parent, provenance['payload']))
    counter = build.get('serialRecovery', {}).get('hardwareSecurityCounter')
    if (build.get('board') != MODEL or build.get('channel') != channel or
        build.get('mcubootPublicKeySha256') != boot_fingerprint or
        type(counter) is not int or counter < minimum_counter or
        build.get('artifacts', {}).get('app/zephyr/zephyr.signed.bin') != image['payload']['sha256']):
        raise ValueError('board provenance, boot key, image or counter mismatch')
    wire = evidence.resolve(manifest.parent,image['payload']).read_bytes()
    if hashlib.sha256(wire).hexdigest() != image['payload']['sha256']:
        raise ValueError('image changed during verification')
    measurement = image_measurement(wire,counter)
    return {'model':MODEL, 'channel':channel, 'version':image['version'], 'counter':counter,
            'measurement':measurement, 'measurementAlgorithm':'mcuboot-header-image-protected-tlv-sha256',
            'imageSha256':image['payload']['sha256'], 'bootKeySha256':boot_fingerprint,
            'releaseSha256':evidence.digest(manifest), 'status':'preflight-only'}


def verify_boot_signature(image, boot_public, preflight):
    """Reuse the P05/P10 MCUboot verifier on private immutable input copies."""
    root = Path(__file__).resolve().parents[3]
    try:
        image_bytes, key_bytes = image.read_bytes(), boot_public.read_bytes()
        if hashlib.sha256(image_bytes).hexdigest() != preflight['imageSha256']:
            raise ValueError('image changed during verification')
        with tempfile.TemporaryDirectory(prefix='nu54-boot-verification-') as directory:
            saved_image, saved_key = Path(directory)/'image.bin', Path(directory)/'boot.pem'
            saved_image.write_bytes(image_bytes); saved_key.write_bytes(key_bytes)
            if evidence.key_sha256(saved_key) != preflight['bootKeySha256']:
                raise ValueError('boot public key fingerprint differs from independent pin')
            command = ['go','run','./products/p05-operations-backoffice/cmd/verify-firmware',
                       '--key',str(saved_key),'--image',str(saved_image),
                       '--version',preflight['version'],'--board','nu54v_dk','--counter',str(preflight['counter'])]
            process = subprocess.run(command,cwd=root,capture_output=True,text=True,timeout=120)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ValueError('MCUboot signature verifier unavailable') from exc
    if process.returncode != 0 or process.stdout.strip() != preflight['measurement']:
        raise ValueError('MCUboot signature, version, counter or measurement differs')


def claim_from_observation(report, preflight, now):
    """Validate fields only; caller must authenticate the installation report first."""
    keys = {'format','observed','model','channel','candidate','device','bootKey','measurement',
            'counter','generation','challenge','issuedAt','expiresAt'}
    if (not isinstance(report, dict) or set(report) != keys or
        report['format'] != 'nu54-board-observation-v1' or report['observed'] is not True or
        report['model'] != MODEL or report['channel'] != preflight['channel'] or
        report['candidate'] != preflight['releaseSha256'] or report['bootKey'] != preflight['bootKeySha256'] or
        report['measurement'] != preflight['measurement'] or
        type(report['counter']) is not int or report['counter'] < preflight['counter'] or
        type(report['generation']) is not int or report['generation'] < 0 or
        type(report['issuedAt']) is not int or type(report['expiresAt']) is not int or
        not report['issuedAt'] <= now < report['expiresAt'] or report['expiresAt']-report['issuedAt'] > 300 or
        any(not isinstance(report[k],str) or not re.fullmatch(r'[0-9a-f]{64}',report[k]) or report[k] == '0'*64
            for k in ('candidate','device','bootKey','measurement','challenge'))):
        raise ValueError('invalid observed board installation')
    return {'format':'nu54-board-install-v1','model':report['model'],'channel':report['channel'],
            'candidate':report['candidate'],'device':report['device'],'bootKey':report['bootKey'],
            'measurement':report['measurement'],'counter':report['counter'],'generation':report['generation'],
            'challenge':report['challenge'],'issuedAt':report['issuedAt'],'expiresAt':report['expiresAt'],'evidence':'observed'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('manifest','signature','public-key','trust'):
        parser.add_argument('--'+name, type=Path, required=True)
    parser.add_argument('--channel', choices=('official','custom'), required=True)
    parser.add_argument('--boot-key-sha256', required=True, help='independently verified MCUboot key fingerprint')
    parser.add_argument('--minimum-counter', type=int, required=True)
    parser.add_argument('--boot-public', type=Path, help='independently pinned MCUboot Ed25519 public key')
    parser.add_argument('--installer-key', type=Path, help='trusted independent installer public key for setup')
    parser.add_argument('--setup-out', type=Path, help='new public setup file for the app')
    parser.add_argument('--installation', nargs=3, type=Path, metavar=('RECORD','SIGNATURE','KEY'),
                        help='P10 independently signed observed installation and public key')
    parser.add_argument('--attester-private', type=Path, help='same independent installer key, never stored in the app')
    parser.add_argument('--receipt-out', type=Path, help='new file for signed mobile handoff')
    args = parser.parse_args()
    try:
        result=check(args.manifest,args.signature,args.public_key,evidence.read(args.trust),args.channel,
                     args.boot_key_sha256,args.minimum_counter)
        if args.setup_out or args.installation or args.receipt_out:
            if not args.boot_public:
                raise ValueError('independent MCUboot public key is required to issue setup or receipt')
            release=evidence.read(args.manifest)
            image=next(item for item in release['artifacts'] if item['id']=='p01-image')
            verify_boot_signature(evidence.resolve(args.manifest.parent,image['payload']),args.boot_public,result)
        if bool(args.installer_key) != bool(args.setup_out):
            raise ValueError('installer key and setup output must be supplied together')
        if args.installer_key:
            policy=evidence.read(args.trust)
            fingerprint=evidence.key_sha256(args.installer_key)
            other='custom' if args.channel=='official' else 'official'
            if (fingerprint not in policy['installers'][args.channel] or
                'p01-image' not in policy['installers'][args.channel][fingerprint] or
                fingerprint in policy['installers'][other] or
                fingerprint in policy['channels'][args.channel] or fingerprint in policy['channels'][other]):
                raise ValueError('independent installer is not trusted for this channel')
            der=evidence.openssl(['pkey','-pubin','-in',args.installer_key,'-outform','DER'])
            prefix=bytes.fromhex('302a300506032b6570032100')
            if len(der)!=len(prefix)+32 or not der.startswith(prefix):
                raise ValueError('installer key must be Ed25519')
            setup={'channel':args.channel,'candidate':result['releaseSha256'],'bootKey':result['bootKeySha256'],
                   'measurement':result['measurement'],'minimumCounter':result['counter'],'publicKey':der[len(prefix):].hex()}
            with args.setup_out.open('x',encoding='utf-8') as destination:
                json.dump(setup,destination,separators=(',',':'))
            result['installerSpkiSha256']=fingerprint
        if any((args.installation, args.attester_private, args.receipt_out)):
            if not all((args.installation, args.attester_private, args.receipt_out)):
                raise ValueError('installation, attester key and output must be supplied together')
            record, signature, installer_public = args.installation
            states = evidence.verify(args.manifest,args.signature,args.public_key,evidence.read(args.trust),args.channel,
                                     (record,signature,installer_public))
            claim = evidence.read(record)
            key = ('p01-image',result['version'],MODEL)
            if claim['artifactId'] != 'p01-image' or states.get(key) != 'installation-observed':
                raise ValueError('observed installation for this image is required')
            private_der = evidence.openssl(['pkey','-in',args.attester_private,'-pubout','-outform','DER'])
            if hashlib.sha256(private_der).hexdigest() != evidence.key_sha256(installer_public):
                raise ValueError('attester private key does not match trusted installer')
            report = evidence.read(evidence.resolve(record.parent,claim['report']))
            handoff = claim_from_observation(report,result,int(time.time()))
            payload = json.dumps(handoff,ensure_ascii=False,separators=(',',':'))
            with tempfile.TemporaryDirectory(prefix='nu54-board-receipt-') as directory:
                src, sig = Path(directory)/'payload', Path(directory)/'signature'
                src.write_text(payload,encoding='utf-8')
                evidence.openssl(['pkeyutl','-sign','-rawin','-inkey',args.attester_private,'-in',src,'-out',sig])
                envelope = {'payload':payload,'signature':sig.read_bytes().hex()}
            with args.receipt_out.open('x',encoding='utf-8') as destination:
                json.dump(envelope,destination,separators=(',',':'))
            result['handoff'] = 'signed-observed-receipt'
        print(json.dumps(result,sort_keys=True))
    except (ValueError, KeyError, OSError, StopIteration) as exc:
        print(f'Board release rejected: {exc}',file=sys.stderr)
        return 1
    return 0

if __name__ == '__main__':
    raise SystemExit(main())
