#!/usr/bin/env python3
"""Issue a scoped 8283 finality-key delegation from independently verified setup data."""
import argparse
import json
import time
from pathlib import Path
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec
from enroll_board import exact_hex,point,public_file,signed

FIELDS={'board','generation','chain','signer','genesis','asset','destination','per_transfer','daily','finality_key','expires'}
def grant(profile,issuer_public,display_public,now):
    if not isinstance(profile,dict) or set(profile)!=FIELDS:
        raise ValueError('Expected the exact independently reviewed profile')
    for name in ('generation','chain','per_transfer','daily','expires'):
        if type(profile[name]) is not int or not 0<=profile[name]<2**64:
            raise ValueError('Invalid integer')
    if (not profile['generation'] or profile['chain']!=8283 or not profile['per_transfer'] or
        profile['per_transfer']>profile['daily'] or not now<profile['expires']<=now+600):
        raise ValueError('Unsupported scope or expiry')
    key=exact_hex(profile['finality_key'],65)
    ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256R1(),key)
    if key in (point(issuer_public),point(display_public)):
        raise ValueError('Finality key must be independent of issuer and display')
    board,signer,genesis,asset,destination=(exact_hex(profile[name],size) for name,size in
        (('board',32),('signer',20),('genesis',32),('asset',20),('destination',20)))
    if not all(any(x) for x in (board,signer,genesis,asset,destination)):
        raise ValueError('Empty scope')
    number=lambda name:profile[name].to_bytes(8,'big')
    body=(b'N54D'+board+number('generation')+number('chain')+signer+genesis+asset+destination+
          number('per_transfer')+number('daily')+key+number('expires'))
    if len(body)!=233:raise ValueError('Invalid grant')
    return body

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--profile',type=Path,required=True);parser.add_argument('--issuer',type=Path,required=True)
    parser.add_argument('--display-public',type=Path,required=True);parser.add_argument('--out',type=Path,required=True)
    args=parser.parse_args();issuer=serialization.load_pem_private_key(args.issuer.read_bytes(),None)
    body=grant(json.loads(args.profile.read_text()),issuer.public_key(),public_file(args.display_public),int(time.time()))
    args.out.write_text(json.dumps({'type':'nu54-finality-enrollment-v1','grant':body.hex(),'signature':signed(args.issuer,body).hex()},indent=2)+'\n')
if __name__=='__main__':main()
