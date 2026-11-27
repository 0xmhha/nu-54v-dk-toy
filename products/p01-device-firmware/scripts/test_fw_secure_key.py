"""The --secure-key build profile of fw.py: target, inputs, refused combinations and host evidence."""

from pathlib import Path
import shutil
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch

import fw

ENVIRONMENT = {"NU54_INSTALL_AUTHORITY_HEX": "04" + "Ab" * 64, "NU54_RECOVERY_OVERLAY": "",
               "NU54_LAB_WALLET_CHAINS": "", "NU54_COMMISSIONED_WALLET_CHAINS": ""}
GOOD_CONFIG = ("CONFIG_NU54_SECURE_KEY=y\nCONFIG_BUILD_WITH_TFM=y\nCONFIG_TRUSTED_EXECUTION_NONSECURE=y\n"
               "CONFIG_TFM_ITS_ENCRYPTED=y\nCONFIG_PSA_CRYPTO_DRIVER_CRACEN=y\nCONFIG_TFM_ITS_NUM_ASSETS=64\n"
               "CONFIG_MBEDTLS_PSA_CRYPTO_STORAGE_C=y\n")


def link_map(placed="", discarded="", dropped=""):
    """A GNU ld map: what the linker threw away comes before the memory map, /DISCARD/ inside it."""
    return ("Archive member included to satisfy reference by file (symbol)\n\n"
            "libcrypto.a(sign.o)\n                              app.o (psa_sign_hash)\n\n"
            "Discarded input sections\n\n" + discarded +
            "\nMemory Configuration\n\nName             Origin             Length             Attributes\n"
            "FLASH            0x00010000         0x00020000         xr\n\n"
            "Linker script and memory map\n\n"
            ".text           0x00010000     0x4000\n *(.text*)\n" + placed +
            "\n/DISCARD/\n *(.note.GNU-stack)\n" + dropped + "\n.bss            0x20000000      0x100\n")


def text_section(name, address=0x12340):
    return (f" .text.{name}\n                0x{address:016x}       0xd8 libdriver.a(sign.o)\n"
            f"                0x{address:016x}                {name}\n")


SIGNER_PLACED = link_map(text_section(fw.SECURE_SIGNER))
APP_WITHOUT_DRIVER = link_map(text_section("psa_sign_hash"))


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

    def write(self, config=GOOD_CONFIG, secure_map=SIGNER_PLACED, app_map=APP_WITHOUT_DRIVER):
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
                       # Without it TF-M is built with CRYPTO_STORAGE_DISABLED and keeps no persistent key.
                       GOOD_CONFIG.replace("CONFIG_MBEDTLS_PSA_CRYPTO_STORAGE_C=y\n", ""),
                       GOOD_CONFIG.replace("CONFIG_MBEDTLS_PSA_CRYPTO_STORAGE_C=y\n",
                                           "# CONFIG_MBEDTLS_PSA_CRYPTO_STORAGE_C is not set\n")):
            with self.subTest(config=config):
                self.write(config=config)
                with self.assertRaisesRegex(SystemExit, "secure side"):
                    self.evidence()

    def test_rejects_signing_code_outside_the_secure_image(self):
        self.write(secure_map=link_map(text_section("psa_sign_hash")))
        with self.assertRaisesRegex(SystemExit, "secure image"):
            self.evidence()
        self.write(app_map=link_map(text_section(fw.SECURE_SIGNER)))
        with self.assertRaisesRegex(SystemExit, "non-secure application"):
            self.evidence()

    def test_signing_code_the_linker_discarded_is_not_in_the_secure_image(self):
        gone = text_section(fw.SECURE_SIGNER, address=0)
        for secure_map in (link_map(discarded=gone), link_map(dropped=gone),
                           # Named only as the reason an archive member was pulled in.
                           link_map().replace("(psa_sign_hash)", f"({fw.SECURE_SIGNER})")):
            with self.subTest(secure_map=secure_map):
                self.write(secure_map=secure_map)
                with self.assertRaisesRegex(SystemExit, "secure image"):
                    self.evidence()

    def test_driver_code_the_linker_discarded_is_not_in_the_application(self):
        gone = text_section("cracen_init", address=0)
        self.write(app_map=link_map(text_section("psa_sign_hash"), discarded=gone, dropped=gone))
        self.assertEqual(0, self.evidence())
        self.write(app_map=link_map(" .rodata.cracen_table\n                0x0000000000013000       0x20 libdriver.a(t.o)\n"))
        with self.assertRaisesRegex(SystemExit, "non-secure application"):
            self.evidence()

    def test_a_file_without_a_memory_map_is_no_evidence(self):
        for missing in ("secure", "app"):
            with self.subTest(missing=missing):
                text = f" .text.{fw.SECURE_SIGNER}\n"
                self.write(**({"secure_map": text} if missing == "secure" else {"app_map": text}))
                with self.assertRaisesRegex(SystemExit, "memory map"):
                    self.evidence()

    def test_rejects_an_image_that_does_not_verify(self):
        with self.assertRaisesRegex(SystemExit, "signature"):
            self.evidence(verified=1)

    def flash(self):
        with patch.object(fw, "BUILD_DIR", self.root), \
                patch.object(fw, "in_toolchain", side_effect=AssertionError("must not flash")):
            return fw.main(["flash"])

    def images(self):
        """Everything `flash` would write, so only the refusal keeps it from the board."""
        (self.root / "mcuboot/zephyr").mkdir(parents=True)
        for path in ("mcuboot/zephyr/zephyr.hex", "mcuboot/zephyr/.config", "app/zephyr/zephyr.signed.hex"):
            (self.root / path).write_text("")

    def test_flash_refuses_the_build(self):
        self.images()
        with self.assertRaisesRegex(SystemExit, "not qualified"):
            self.flash()

    def test_flash_refuses_a_build_whose_evidence_failed(self):
        """The profile's own option did not take effect: the image is still a TF-M image."""
        self.images()
        failed = GOOD_CONFIG.replace("CONFIG_NU54_SECURE_KEY=y\n", "# CONFIG_NU54_SECURE_KEY is not set\n")
        self.write(config=failed)
        with self.assertRaisesRegex(SystemExit, "secure side"):
            self.evidence()
        with self.assertRaisesRegex(SystemExit, "not qualified.*CONFIG_BUILD_WITH_TFM=y"):
            self.flash()
        self.write(config=failed.replace("CONFIG_BUILD_WITH_TFM=y\n", ""))
        with self.assertRaisesRegex(SystemExit, "not qualified.*CONFIG_TRUSTED_EXECUTION_NONSECURE=y"):
            self.flash()

    def test_flash_refuses_a_trustzone_board_target_whatever_the_options_say(self):
        self.images()
        self.write(config="CONFIG_NU54_SPENDING_PROFILE=y\n")
        with self.assertRaisesRegex(SystemExit, "not qualified.*app/tfm"):
            self.flash()
        shutil.rmtree(self.root / "app/tfm")
        for cache, line in (("CMakeCache.txt", "BOARD:STRING=nu54v_dk/nrf54l15/cpuapp/ns"),
                            ("app/CMakeCache.txt", "CACHED_BOARD:STRING=nu54v_dk/nrf54l15/cpuapp/ns")):
            with self.subTest(cache=cache):
                (self.root / cache).write_text(f"CMAKE_BUILD_TYPE:STRING=\n{line}\n")
                with self.assertRaisesRegex(SystemExit, "not qualified.*/ns"):
                    self.flash()
                (self.root / cache).unlink()

    def test_flash_refuses_a_release_without_its_configuration(self):
        self.images()
        (self.root / "app/zephyr/.config").unlink()
        with self.assertRaisesRegex(SystemExit, "configuration missing"):
            self.flash()

    def test_flash_still_writes_the_installed_profile(self):
        self.images()
        self.write(config="CONFIG_NU54_SPENDING_PROFILE=y\n")
        shutil.rmtree(self.root / "app/tfm")
        (self.root / "app/CMakeCache.txt").write_text("BOARD:STRING=nu54v_dk/nrf54l15/cpuapp\n")
        with patch.object(fw, "BUILD_DIR", self.root), patch.object(fw, "in_toolchain", return_value=0) as toolchain:
            self.assertEqual(0, fw.main(["flash"]))
        self.assertEqual([str(self.root / "mcuboot/zephyr/zephyr.hex"), str(self.root / "app/zephyr/zephyr.signed.hex")],
                         [call.args[-1] for call in toolchain.call_args_list])


if __name__ == "__main__":
    unittest.main()
