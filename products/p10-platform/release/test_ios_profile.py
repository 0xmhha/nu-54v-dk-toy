"""Deterministic regression for the iOS profile evidence validator."""

from copy import deepcopy
from pathlib import Path
import tempfile
import unittest

from fleet import key_identity, sign
from ios_profile import CONTRACT, FORMAT, STEPS, check
from test_fleet import keys


class ProfileProofTest(unittest.TestCase):
    def test_integrity_scope_expiry_and_failure(self):
        with tempfile.TemporaryDirectory() as work:
            key, public = keys(Path(work),"observer")
            signer = key_identity(public)
            runtime = "ios-27-0-simulator"
            package = "com.nu54renter.fleetqa.testfixture"
            baseline = {"payload":"a"*64,"bundleId":package,"profileIdentity":"c"*64}
            upgrade = {"payload":"b"*64,"bundleId":package,"profileIdentity":"c"*64,"dimensions":{"os":runtime}}
            policy = {"iosProfileVerifiers":{signer:{"key":str(public),"packages":[package],"os":[runtime]}}}
            claim = {"format":FORMAT,"contract":CONTRACT,"source":baseline["payload"],"target":upgrade["payload"],
                     "package":package,"os":runtime,"identity":"c"*64,"observedAt":100,"expiresAt":200,
                     "steps":{step:{"passed":True,"result":str(index)*64} for index,step in enumerate(STEPS,1)}}

            def wrap(record):
                return {"claim":record,"signer":signer,"signature":sign(record,key)}

            proof = wrap(claim)
            check(proof,baseline,upgrade,policy,150)
            changed = deepcopy(claim)
            changed["steps"]["upgradeReadAndWrite"]["passed"] = False
            wrong_runtime = {**upgrade,"dimensions":{"os":"ios-26-0-simulator"}}
            revised = {**upgrade,"payload":"c"*64}
            cases = (
                (None,baseline,upgrade,policy,150,(),"missing"),
                (proof,baseline,upgrade,policy,200,(),"stale"),
                (proof,baseline,revised,policy,150,(),"different transition"),
                (proof,baseline,wrong_runtime,policy,150,(),"different transition"),
                (proof,baseline,upgrade,{},150,(),"not enrolled"),
                (proof,baseline,upgrade,policy,150,(signer,),"not enrolled"),
                (wrap(changed),baseline,upgrade,policy,150,(),"check failed"),
            )
            for *args, message in cases:
                with self.subTest(message=message), self.assertRaisesRegex(ValueError,message):
                    check(*args)


if __name__ == "__main__":
    unittest.main()
