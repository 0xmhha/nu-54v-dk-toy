from datetime import datetime, timedelta, timezone
from pathlib import Path
from tempfile import TemporaryDirectory
import unittest

from release import identity, verify_owner_registration_isolation, verify_profile, workspace


class ReleaseProfileTest(unittest.TestCase):
    def setUp(self):
        self.leaf = b"test certificate"
        self.bundle = "com.nu54kiosk"
        self.team = "ABCDEFGHIJ"
        self.entitlements = {
            "application-identifier": f"{self.team}.{self.bundle}",
            "com.apple.developer.team-identifier": self.team,
            "get-task-allow": False,
            "keychain-access-groups": [f"{self.team}.{self.bundle}"],
        }
        self.profile = {
            "TeamIdentifier": [self.team],
            "ApplicationIdentifierPrefix": [self.team],
            "Entitlements": {**self.entitlements, "keychain-access-groups": [f"{self.team}.*"]},
            "DeveloperCertificates": [self.leaf],
            "Platform": ["iOS"],
            "ExpirationDate": datetime.now(timezone.utc) + timedelta(days=1),
        }

    def verify(self, channel="official"):
        return verify_profile(channel, self.profile, self.entitlements, self.bundle, self.leaf)

    def test_valid_device_profile(self):
        self.assertEqual(self.team, self.verify())

    def test_reject_other_team_or_debugger(self):
        self.entitlements["com.apple.developer.team-identifier"] = "ZYXWVUTSRQ"
        with self.assertRaisesRegex(ValueError, "Team ID"):
            self.verify()
        self.entitlements["com.apple.developer.team-identifier"] = self.team
        self.entitlements["get-task-allow"] = True
        with self.assertRaisesRegex(ValueError, "Debug entitlements"):
            self.verify()

    def test_reject_shared_keychain_even_if_profile_allows_it(self):
        self.entitlements["keychain-access-groups"] = [f"{self.team}.other"]
        with self.assertRaisesRegex(ValueError, "keychain"):
            self.verify()

    def test_reject_substituted_signer_and_official_ad_hoc_profile(self):
        with self.assertRaisesRegex(ValueError, "absent from the profile"):
            verify_profile("official", self.profile, self.entitlements, self.bundle, b"other")
        self.profile["ProvisionedDevices"] = ["test device"]
        with self.assertRaisesRegex(ValueError, "App Store"):
            self.verify()
        self.assertEqual(self.team, self.verify("custom"))

    def test_custom_policy_uses_distinct_bundle(self):
        policy = identity("custom")
        self.assertNotEqual(policy["bundle"], policy["customBundle"])

    def test_renter_uses_its_own_workspace_and_release_identity(self):
        policy = identity("custom", "p02")
        self.assertEqual(policy["bundle"], "com.nu54renter")
        self.assertEqual(policy["customBundle"], "com.nu54renter.custom")
        self.assertEqual(workspace("p02")[1], "NU54Renter")
        self.assertNotEqual(policy["bundle"], identity("custom")["bundle"])


    def test_renter_release_excludes_registration_adapter(self):
        with TemporaryDirectory() as temp:
            executable = Path(temp) / "NU54Renter"
            executable.write_bytes(b"com.nu54renter.owner.p256.v2")
            verify_owner_registration_isolation(executable)
            for marker in (b"nu54.owner.registration.w04qa.p256", b"com.nu54renter.w04qa",
                           b"owner-registration-host", b"owner-registration-other",
                           b"w04-host-port", b"w04-other-host-port"):
                with self.subTest(marker=marker):
                    executable.write_bytes(b"release\0" + marker + b"\0")
                    with self.assertRaisesRegex(ValueError, "simulator registration adapter"):
                        verify_owner_registration_isolation(executable)


if __name__ == "__main__":
    unittest.main()
