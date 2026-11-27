"""The --secure-key build profile of fw.py: target, inputs, refused combinations and host evidence."""

from pathlib import Path
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch

import fw

ENVIRONMENT = {"NU54_INSTALL_AUTHORITY_HEX": "04" + "Ab" * 64, "NU54_RECOVERY_OVERLAY": "",
               "NU54_LAB_WALLET_CHAINS": "", "NU54_COMMISSIONED_WALLET_CHAINS": ""}
GOOD_CONFIG = ("CONFIG_NU54_SECURE_KEY=y\nCONFIG_BUILD_WITH_TFM=y\nCONFIG_TRUSTED_EXECUTION_NONSECURE=y\n"
               "CONFIG_TFM_ITS_ENCRYPTED=y\nCONFIG_PSA_CRYPTO_DRIVER_CRACEN=y\nCONFIG_TFM_ITS_NUM_ASSETS=64\n")


def west_arguments(*flags):
    """The west command of a release build, without running the toolchain."""
    with TemporaryDirectory() as root, patch.dict("os.environ", ENVIRONMENT), \
            patch.object(fw, "BUILD_DIR", Path(root)), patch.object(fw, "signing_key", return_value=Path("key.pem")), \
            patch.object(fw, "release_signer", return_value=(Path("public.pem"), "0")), \
            patch.object(fw.mbedtls_input, "prepare", return_value=Path("mbedtls")), \
            patch.object(fw.mcuboot_input, "prepare", return_value=Path("mcuboot")), \
            patch.object(fw, "in_toolchain", return_value=1) as toolchain:
        assert fw.main(["build", "--release", *flags]) == 1
    return list(toolchain.call_args.args)


class SecureKeyBuildTest(unittest.TestCase):
    def test_trustzone_target_configuration_and_layout(self):
        args = west_arguments("--spending", "--secure-key")
        self.assertEqual("nu54v_dk/nrf54l15/cpuapp/ns", args[args.index("-b") + 1])
        conf = next(arg for arg in args if arg.startswith("-Dapp_EXTRA_CONF_FILE=")).split("=", 1)[1].split(";")
        self.assertEqual(["release.conf", "pilot-tls.conf", "spending.conf", "enrollment-pin.conf", "secure-key.conf"],
                         [Path(path).name for path in conf])
        self.assertIn(f"-Dapp_EXTRA_DTC_OVERLAY_FILE={fw.APP / 'secure-key.overlay'}", args)
        self.assertIn(f"-Dmcuboot_EXTRA_DTC_OVERLAY_FILE={fw.APP / 'secure-key-mcuboot.overlay'}", args)
        for name in ("secure-key.conf", "secure-key.overlay", "secure-key-mcuboot.overlay"):
            self.assertTrue((fw.APP / name).is_file(), name)

    def test_other_builds_keep_their_target_and_inputs(self):
        for flags in (["--spending"], []):
            with self.subTest(flags=flags):
                args = west_arguments(*flags)
                self.assertEqual("nu54v_dk/nrf54l15/cpuapp", args[args.index("-b") + 1])
                self.assertFalse(any("secure-key" in arg for arg in args))

    def test_refused_combinations(self):
        for flags in (["--secure-key"], ["--release", "--secure-key"], ["--rental", "--secure-key"],
                      ["--release", "--spending", "--secure-key", "--approtect"],
                      ["--release", "--spending", "--secure-key", "--security-counter=2"],
                      ["--release", "--spending", "--secure-key", "--approtect", "--official", "--security-counter=2"]):
            with self.subTest(flags=flags), patch.dict("os.environ", ENVIRONMENT), \
                    patch.object(fw, "in_toolchain", side_effect=AssertionError("must not build")), \
                    self.assertRaises(SystemExit):
                fw.main(["build", *flags])


class SecureKeyEvidenceTest(unittest.TestCase):
    def setUp(self):
        temporary = TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        (self.root / "app/zephyr").mkdir(parents=True)
        (self.root / "app/tfm/bin").mkdir(parents=True)
        self.write()

    def write(self, config=GOOD_CONFIG, secure_map=f" .text.{fw.SECURE_SIGNER}\n", app_map=" .text.psa_sign_hash\n"):
        (self.root / "app/zephyr/.config").write_text(config)
        (self.root / "app/tfm/bin/tfm_s.map").write_text(secure_map)
        (self.root / "app/zephyr/zephyr.map").write_text(app_map)

    def evidence(self, verified=0):
        with patch.object(fw, "BUILD_DIR", self.root), patch.object(fw, "in_toolchain", return_value=verified):
            return fw.secure_key_evidence(Path("public.pem"))

    def test_accepts_the_key_in_the_secure_side_without_a_release_manifest(self):
        self.assertEqual(0, self.evidence())
        self.assertFalse((self.root / "release-manifest.json").exists())

    def test_rejects_a_configuration_that_keeps_the_key_elsewhere(self):
        for config in (GOOD_CONFIG.replace("CONFIG_BUILD_WITH_TFM=y\n", ""),
                       GOOD_CONFIG.replace("CONFIG_TFM_ITS_ENCRYPTED=y\n", ""),
                       GOOD_CONFIG.replace("CONFIG_NU54_SECURE_KEY=y\n", ""),
                       GOOD_CONFIG.replace("CONFIG_TFM_ITS_NUM_ASSETS=64\n", "CONFIG_TFM_ITS_NUM_ASSETS=10\n"),
                       GOOD_CONFIG + "CONFIG_SECURE_STORAGE=y\n",
                       GOOD_CONFIG + "CONFIG_MBEDTLS_PSA_CRYPTO_STORAGE_C=y\n"):
            with self.subTest(config=config):
                self.write(config=config)
                with self.assertRaisesRegex(SystemExit, "secure side"):
                    self.evidence()

    def test_rejects_signing_code_outside_the_secure_image(self):
        self.write(secure_map=" .text.psa_sign_hash\n")
        with self.assertRaisesRegex(SystemExit, "secure image"):
            self.evidence()
        self.write(app_map=f" .text.{fw.SECURE_SIGNER}\n")
        with self.assertRaisesRegex(SystemExit, "non-secure application"):
            self.evidence()

    def test_rejects_an_image_that_does_not_verify(self):
        with self.assertRaisesRegex(SystemExit, "signature"):
            self.evidence(verified=1)

    def test_flash_refuses_the_build(self):
        with patch.object(fw, "BUILD_DIR", self.root), \
                patch.object(fw, "in_toolchain", side_effect=AssertionError("must not flash")), \
                self.assertRaisesRegex(SystemExit, "not qualified"):
            fw.main(["flash"])


if __name__ == "__main__":
    unittest.main()
