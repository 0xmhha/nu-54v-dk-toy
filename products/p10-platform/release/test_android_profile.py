"""Verify the Android profile manifest and independent evidence contract."""

from copy import deepcopy
from pathlib import Path
import tempfile
import unittest
import xml.etree.ElementTree as ET

from android_profile import (CONTRACT, FORMAT, PROTECTED_CONTRACT, PROTECTED_FORMAT, PROTECTED_STEPS, STEPS,
                             check, check_protected)
from fleet import key_identity, sign
from test_fleet import keys


class AndroidProfileTests(unittest.TestCase):
    def test_application_manifest_retains_rollback_data(self):
        root = Path(__file__).resolve().parents[3]
        manifest = root/"products/p02-user-app/android/app/src/main/AndroidManifest.xml"
        app = ET.parse(manifest).find("application")
        self.assertEqual(app.attrib["{http://schemas.android.com/apk/res/android}rollbackDataPolicy"],"retain")
        self.assertEqual(app.attrib["{http://schemas.android.com/apk/res/android}allowBackup"],"false")

    def test_signed_scope_and_failed_rollback(self):
        with tempfile.TemporaryDirectory() as directory:
            private,public=keys(Path(directory),"observer")
            fingerprint=key_identity(public)
            package,os="com.nu54renter.fleetqa.r0123456789abcdef","android-api-34"
            source={"package":package,"payload":"a"*64,"certificate":"b"*64,"versionCode":1}
            target={**source,"payload":"c"*64,"versionCode":2,"dimensions":{"os":os},"rollbackDataPolicy":"retain"}
            claim={"format":FORMAT,"contract":CONTRACT,"source":source["payload"],"target":target["payload"],
                   "package":package,"os":os,"certificate":source["certificate"],"rollbackDataPolicy":"retain",
                   "observedAt":100,"expiresAt":400,
                   "steps":{step:{"result":"d"*64,"passed":True} for step in STEPS}}
            policy={"androidProfileVerifiers":{fingerprint:{"key":str(public),"packages":[package],"os":[os]}}}

            def proof(payload):
                return {"claim":payload,"signer":fingerprint,"signature":sign(payload,private)}

            check(proof(claim),source,target,policy,200,set())
            failed=deepcopy(claim)
            failed["steps"]["rollbackRead"]["passed"]=False
            cases=(
                (claim,{**target,"rollbackDataPolicy":"unverified"},200,set(),"incompatible APK"),
                (claim,{**target,"certificate":"e"*64},200,set(),"incompatible APK"),
                (claim,{**target,"payload":"e"*64},200,set(),"different transition"),
                (claim,target,400,set(),"stale"),
                (claim,target,200,{fingerprint},"not enrolled"),
                (failed,target,200,set(),"check failed"),
            )
            for payload,row,now,excluded,message in cases:
                with self.subTest(message=message),self.assertRaisesRegex(ValueError,message):
                    check(proof(payload),source,row,policy,now,excluded)


    def test_protected_scope_requires_new_expired_unattempted_observation(self):
        with tempfile.TemporaryDirectory() as directory:
            private,public=keys(Path(directory),"observer")
            verifier=key_identity(public)
            package,os="com.nu54renter.fleetqa.r0123456789abcdef","android-api-34"
            source={"package":package,"payload":"a"*64,"certificate":"b"*64,"versionCode":101}
            target={**source,"payload":"c"*64,"versionCode":102,"dimensions":{"os":os},"rollbackDataPolicy":"retain"}
            claim={"format":PROTECTED_FORMAT,"contract":PROTECTED_CONTRACT,"source":source["payload"],
                   "target":target["payload"],"package":package,"os":os,"certificate":source["certificate"],
                   "rollbackDataPolicy":"retain","scenario":"expired-unattempted-c1","authority":"synthetic-local",
                   "observedAt":100,"expiresAt":400,"assurance":"owner-hardware-unavailable",
                   "ownerScope":"e"*64,"rawHash":"f"*64,
                   "steps":{step:{"result":"d"*64,"passed":True} for step in PROTECTED_STEPS}}
            grant={"key":str(public),"packages":[package],"os":[os],"contracts":[PROTECTED_CONTRACT]}
            policy={"androidProfileVerifiers":{verifier:grant}}

            def proof(payload):
                return {"claim":payload,"signer":verifier,"signature":sign(payload,private)}

            check_protected(proof(claim),source,target,policy,200,set())
            with self.assertRaisesRegex(ValueError,"not independently enrolled"):
                check_protected(proof(claim),source,target,{"androidProfileVerifiers":{verifier:{**grant,"contracts":[]}}},200,set())
            with self.assertRaisesRegex(ValueError,"different APK transition"):
                check_protected(proof({**claim,"scenario":"submitted-only"}),source,target,policy,200,set())
            with self.assertRaisesRegex(ValueError,"evidence missing"):
                check_protected(None,source,target,policy,200,set())
            failed=deepcopy(claim)
            failed["steps"]["rollbackPending"]["passed"]=False
            with self.assertRaisesRegex(ValueError,"security-state continuity not verified"):
                check_protected(proof(failed),source,target,policy,200,set())


if __name__=="__main__":
    unittest.main()
