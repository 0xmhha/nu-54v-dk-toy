from pathlib import Path
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch

import fw
import mbedtls_input

from fw import release_debug_configuration


class CapabilityCacheTest(unittest.TestCase):
    def test_test_build_uses_owned_cache_and_keeps_caller_output(self):
        for exit_code in (0, 7):
            with self.subTest(exit_code=exit_code), TemporaryDirectory() as root:
                output = Path(root) / "output"
                caches = []

                def configure(*args, runner):
                    self.assertTrue(callable(runner))
                    self.assertIn(str(output), args)
                    self.assertIn(f"-DEXTRA_ZEPHYR_MODULES={fw.HERE / 'capability-cache'}", args)
                    cache_arg = next(value for value in args if value.startswith("-DUSER_CACHE_DIR:PATH="))
                    cache = Path(cache_arg.split("=", 1)[1])
                    caches.append(cache)
                    (cache / "probe").touch()
                    output.mkdir()
                    (output / "artifact").touch()
                    return exit_code

                with patch.dict("os.environ", {"P01_BUILD_DIR": str(output), "TMPDIR": root}), \
                        patch.object(fw, "BUILD_DIR", output), patch.object(fw, "in_toolchain", configure):
                    self.assertEqual(exit_code, fw.main(["build", "--test-capability-cache"]))
                    with self.assertRaisesRegex(SystemExit, "fresh, explicit"):
                        fw.main(["build", "--test-capability-cache"])
                self.assertEqual(1, len(caches))
                self.assertFalse(caches[0].exists())
                self.assertTrue((output / "artifact").is_file())

    def test_product_build_keeps_default_cache_selection(self):
        with patch.object(fw, "in_toolchain", return_value=0) as toolchain:
            self.assertEqual(0, fw.main(["build"]))
        self.assertFalse(any("USER_CACHE_DIR" in arg or "capability-cache" in arg
                             for arg in toolchain.call_args.args))


class SpendingBuildTest(unittest.TestCase):
    environment = {"NU54_INSTALL_AUTHORITY_HEX": "04" + "Ab" * 64, "NU54_RECOVERY_OVERLAY": "",
                   "NU54_LAB_WALLET_CHAINS": "", "NU54_COMMISSIONED_WALLET_CHAINS": ""}

    def configured(self, *flags):
        """The app configuration files of a release build, without running the toolchain."""
        with TemporaryDirectory() as root, patch.dict("os.environ", self.environment), \
                patch.object(fw, "BUILD_DIR", Path(root)), patch.object(fw, "signing_key", return_value=Path("key.pem")), \
                patch.object(fw, "release_signer", return_value=(Path("public.pem"), "0")), \
                patch.object(fw.mbedtls_input, "prepare", return_value=Path("mbedtls")), \
                patch.object(fw.mcuboot_input, "prepare", return_value=Path("mcuboot")), \
                patch.object(fw, "in_toolchain", return_value=1) as toolchain:
            self.assertEqual(1, fw.main(["build", "--release", *flags]))
        conf = next(arg for arg in toolchain.call_args.args if arg.startswith("-Dapp_EXTRA_CONF_FILE="))
        return [Path(path).name for path in conf.split("=", 1)[1].split(";")]

    def test_profile_replaces_hd_recovery(self):
        self.assertEqual(["release.conf", "pilot-tls.conf", "spending.conf", "enrollment-pin.conf", "counted-update.conf"],
                         self.configured("--spending", "--security-counter=1"))
        self.assertEqual(["release.conf", "pilot-tls.conf", "hd-recovery.conf", "enrollment-pin.conf"], self.configured())

    def test_refused_combinations(self):
        for flags, changed in (([], {}), (["--release", "--rental"], {}), (["--release", "--approtect", "--official"], {}),
                               (["--release"], {"NU54_INSTALL_AUTHORITY_HEX": ""}),
                               (["--release"], {"NU54_RECOVERY_OVERLAY": "accessory.overlay"}),
                               (["--release"], {"NU54_COMMISSIONED_WALLET_CHAINS": "1"})):
            with self.subTest(flags=flags, changed=changed), \
                    patch.dict("os.environ", {**self.environment, **changed}), \
                    patch.object(fw, "in_toolchain", side_effect=AssertionError("must not build")), \
                    self.assertRaises(SystemExit):
                fw.main(["build", "--spending", *flags])


class ReleaseDebugConfigurationTest(unittest.TestCase):
    def setUp(self):
        temporary = TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.configs = {
            name: Path(temporary.name) / name for name in ("app", "mcuboot")
        }
        self.diagnostic = "CONFIG_NRF_APPROTECT_DISABLE=y\nCONFIG_NRF_SECURE_APPROTECT_DISABLE=y\n"
        self.locked = "CONFIG_NRF_APPROTECT_LOCK=y\nCONFIG_NRF_SECURE_APPROTECT_LOCK=y\n"

    def write(self, app, mcuboot):
        self.configs["app"].write_text(app)
        self.configs["mcuboot"].write_text(mcuboot)

    def test_diagnostic_and_locked(self):
        self.write(self.diagnostic, self.diagnostic)
        self.assertEqual({"app": "diagnostic", "mcuboot": "diagnostic"}, release_debug_configuration(self.configs, False))
        self.write(self.locked, self.locked)
        self.assertEqual({"app": "locked", "mcuboot": "locked"}, release_debug_configuration(self.configs, True))

    def test_reject_disagreement_between_images(self):
        self.write(self.locked, self.diagnostic)
        with self.assertRaisesRegex(SystemExit, "mcuboot.*diagnostic.*locked"):
            release_debug_configuration(self.configs, True)

    def test_reject_missing_or_conflicting_generated_options(self):
        for altered in ("", self.locked + "CONFIG_NRF_APPROTECT_DISABLE=y\n"):
            with self.subTest(altered=altered):
                self.write(altered, self.locked)
                with self.assertRaisesRegex(SystemExit, "app.*invalid"):
                    release_debug_configuration(self.configs, True)


class ReleaseMbedTLSInputTest(unittest.TestCase):
    def test_reject_archive_before_extraction(self):
        with TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "changed.tar.bz2"
            source.write_bytes(b"obsolete or altered vendor input")
            with patch.dict("os.environ", {"NU54_MBEDTLS_ARCHIVE": str(source)}):
                with self.assertRaisesRegex(SystemExit, "SHA-256 does not match"):
                    mbedtls_input.prepare(root)
            self.assertFalse((root / mbedtls_input.NAME).exists())


if __name__ == "__main__":
    unittest.main()
