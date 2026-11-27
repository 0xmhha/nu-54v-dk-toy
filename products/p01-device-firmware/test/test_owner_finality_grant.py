import sys
from pathlib import Path
import unittest
from cryptography.hazmat.primitives.asymmetric import ec
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'scripts'))
from issue_owner_finality_grant import grant,point
class FinalityGrantTest(unittest.TestCase):
 def test_scoped_independent_keys_and_exact_profile(self):
  issuer=ec.generate_private_key(ec.SECP256R1());display=ec.generate_private_key(ec.SECP256R1());finality=ec.generate_private_key(ec.SECP256R1())
  p=dict(board='01'*32,generation=7,chain=8283,signer='05'*20,genesis='06'*32,asset='07'*20,destination='08'*20,per_transfer=10,daily=30,finality_key=point(finality.public_key()).hex(),expires=87000)
  self.assertEqual(len(grant(p,issuer.public_key(),display.public_key(),86410)),233)
  for field,value in [('chain',84532),('generation',0),('daily',9),('finality_key',point(display.public_key()).hex()),('expires',86410)]:
   wrong=dict(p);wrong[field]=value
   with self.assertRaises(ValueError):grant(wrong,issuer.public_key(),display.public_key(),86410)
if __name__=='__main__':unittest.main()
