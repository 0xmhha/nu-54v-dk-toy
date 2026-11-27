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


TFM_API = "modules/trusted-firmware-m/libtfm_api.a(tfm_crypto_api.c.obj)"
PSA_CORE = "modules/nrf/subsys/nrf_security/src/core/nrf_oberon/libpsa_core.a(psa_crypto.c.obj)"


def key_call(name, origin=TFM_API, address=0x55c88):
    """A function section as the application's map prints it: the section, its input file, its symbol."""
    return (f" .text.{name}\n                0x{address:08x}       0x58 {origin}\n"
            f"                0x{address:08x}                {name}\n")


SIGNER_PLACED = link_map(text_section(fw.SECURE_SIGNER))
KEY_CALLS_FROM_TFM = key_call("psa_sign_hash") + key_call("psa_generate_key", address=0x55e04)
APP_WITHOUT_DRIVER = link_map(KEY_CALLS_FROM_TFM)


def intel_hex(*runs):
    """Intel HEX text of (address, bytes) runs, with the record types of a signed image: 04, 00, 01."""
    def record(kind, offset, data):
        body = bytes([len(data), offset >> 8, offset & 0xff, kind]) + data
        return ":" + (body + bytes([-sum(body) & 0xff])).hex().upper()

    lines, upper = [], None
    for start, data in runs:
        for at in range(start, start + len(data), 16):
            if at >> 16 != upper:
                upper = at >> 16
                lines.append(record(4, 0, upper.to_bytes(2, "big")))
            lines.append(record(0, at & 0xffff, data[at - start:at - start + 16]))
    return "\n".join([*lines, record(1, 0, b"")]) + "\n"


def words(*values):
    return b"".join(value.to_bytes(4, "little") for value in values)


# The first words of the images of a default spending build and of a --secure-key build.
VECTORS = words(0x2001f600, 0x00029805, 0x0004b5ab, 0x000297f1)
MCUBOOT_HEADER = words(0x96f3b83d, 0, 0x800, 0x54ab8, 0, 0x300, 0, 0)
BOOT_IMAGE = intel_hex((0x0, words(0x20005208, 0x00005995, 0x0000689f, 0x00005981)))
APP_IMAGE = intel_hex((0x10000, MCUBOOT_HEADER), (0x10800, VECTORS))
SINGLE_IMAGE = intel_hex((0x0, VECTORS))
# TF-M behind the same header: its entry veneers (SG, then a branch) and the non-secure half 128 KiB in.
SECURE_HALF = [(0x10800, words(0x20000858, 0x000136b1, 0x00013751, 0x0001e6d9)),
               (0x10c80, words(0xe97fe97f, 0xbb84f006, 0xe97fe97f, 0xbb92f006))]
NONSECURE_HALF = (0x30000, words(0x2002ee00, 0x00048f9d, 0x00064eb1, 0x00048f89))
TFM_IMAGE = intel_hex((0x10000, MCUBOOT_HEADER), *SECURE_HALF, NONSECURE_HALF)


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
        self.write(app_map=link_map(KEY_CALLS_FROM_TFM, discarded=gone, dropped=gone))
        self.assertEqual(0, self.evidence())
        self.write(app_map=link_map(" .rodata.cracen_table\n                0x0000000000013000       0x20 libdriver.a(t.o)\n"))
        with self.assertRaisesRegex(SystemExit, "non-secure application"):
            self.evidence()

    def test_rejects_key_calls_that_are_not_the_tfm_api(self):
        """Without the driver the application could still sign in its own PSA core, as the default build does."""
        other_directory = "app/libtfm_api.a(tfm_crypto_api.c.obj)"
        similar_name = "modules/nrf/modules/trusted-firmware-m/libtfm_api_nrf.a(tfm_ioctl_ns_api.c.obj)"
        for name, placed in (
                ("psa_sign_hash", key_call("psa_sign_hash", PSA_CORE) + key_call("psa_generate_key")),
                ("psa_generate_key", key_call("psa_sign_hash") + key_call("psa_generate_key", PSA_CORE)),
                ("psa_sign_hash", key_call("psa_sign_hash", other_directory) + key_call("psa_generate_key")),
                ("psa_sign_hash", key_call("psa_sign_hash", similar_name) + key_call("psa_generate_key")),
                ("psa_sign_hash", key_call("psa_sign_hash", "app/libapp.a(wallet.c.obj)") + key_call("psa_generate_key")),
                # Beside the definition of the TF-M API there is a second one.
                ("psa_generate_key", KEY_CALLS_FROM_TFM + key_call("psa_generate_key", PSA_CORE, address=0x40a50)),
                # No input section before the symbol says where it comes from.
                ("psa_sign_hash", "                0x00055c88                psa_sign_hash\n" + key_call("psa_generate_key")),
                # An output section begins between the library's section and the symbol.
                ("psa_sign_hash", key_call("psa_generate_key") + ".gnu.sgstubs\n                0x00055e50       0x40\n"
                                  "                0x00055e50                psa_sign_hash\n")):
            with self.subTest(placed=placed):
                self.write(app_map=link_map(placed))
                with self.assertRaisesRegex(SystemExit, f"{name} .*TF-M API"):
                    self.evidence()

    def test_key_calls_the_linker_did_not_place_are_not_in_the_application(self):
        gone = key_call("psa_sign_hash", address=0)
        for name, app_map in (
                ("psa_sign_hash", link_map(key_call("psa_generate_key"), discarded=gone)),
                ("psa_sign_hash", link_map(key_call("psa_generate_key"), dropped=gone)),
                ("psa_sign_hash", link_map(key_call("psa_generate_key"), dropped=key_call("psa_sign_hash"))),
                # Named only as the reason an archive member was pulled in.
                ("psa_sign_hash", link_map(key_call("psa_generate_key")).replace("libcrypto.a(sign.o)", TFM_API)),
                # Other functions whose names only begin the same way.
                ("psa_sign_hash", link_map(key_call("psa_sign_hash_start") + key_call("psa_generate_key"))),
                ("psa_generate_key", link_map(key_call("psa_sign_hash") + key_call("psa_generate_key_internal"))),
                ("psa_generate_key", link_map(key_call("psa_sign_hash")))):
            with self.subTest(app_map=app_map):
                self.write(app_map=app_map)
                with self.assertRaisesRegex(SystemExit, f"{name} .*TF-M API.*not in the image"):
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

    def images(self, boot=BOOT_IMAGE, signed=APP_IMAGE, boot_config=""):
        """Everything `flash` would write, so only the refusal keeps it from the board."""
        (self.root / "mcuboot/zephyr").mkdir(parents=True)
        for path, text in (("mcuboot/zephyr/zephyr.hex", boot), ("mcuboot/zephyr/.config", boot_config),
                           ("app/zephyr/zephyr.signed.hex", signed)):
            (self.root / path).write_text(text)

    def installed_configuration(self):
        """What a build of the installed profile generates beside its images: no trace of TF-M."""
        self.write(config="CONFIG_NU54_SPENDING_PROFILE=y\n")
        shutil.rmtree(self.root / "app/tfm")
        for cache in ("CMakeCache.txt", "app/CMakeCache.txt"):
            (self.root / cache).write_text("BOARD:STRING=nu54v_dk/nrf54l15/cpuapp\nCACHED_BOARD:STRING=nu54v_dk/nrf54l15/cpuapp\n")

    def written(self):
        with patch.object(fw, "BUILD_DIR", self.root), patch.object(fw, "in_toolchain", return_value=0) as toolchain:
            self.assertEqual(0, fw.main(["flash"]))
        return [str(Path(call.args[-1]).relative_to(self.root)) for call in toolchain.call_args_list]

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

    def test_flash_refuses_a_trustzone_image_beside_the_installed_configuration(self):
        """The configuration, the board target and the missing app/tfm all say default; the image does not."""
        self.installed_configuration()
        self.images(signed=TFM_IMAGE, boot_config="# CONFIG_BASE_BOARD_IS_NON_SECURE is not set\n")
        with self.assertRaisesRegex(SystemExit, "zephyr.signed.hex.*not qualified.*secure gateway.*0x10c80"):
            self.flash()
        # The secure half alone is enough, wherever it lies in the slot.
        (self.root / "app/zephyr/zephyr.signed.hex").write_text(
            intel_hex((0x10000, MCUBOOT_HEADER), (0x10800, VECTORS), (0x40002, words(0xe97fe97f, 0xbb84f006))))
        with self.assertRaisesRegex(SystemExit, "not qualified.*secure gateway.*0x40002"):
            self.flash()

    def test_flash_refuses_a_bootloader_built_for_the_trustzone_target(self):
        self.installed_configuration()
        self.images(boot_config="CONFIG_SINGLE_APPLICATION_SLOT=y\nCONFIG_BASE_BOARD_IS_NON_SECURE=y\n")
        with self.assertRaisesRegex(SystemExit, "not qualified.*CONFIG_BASE_BOARD_IS_NON_SECURE=y"):
            self.flash()

    def test_flash_refuses_an_image_outside_the_installed_layout(self):
        self.installed_configuration()
        slot_end = 0xc2000
        for images in ({"signed": intel_hex(NONSECURE_HALF)},
                       {"signed": intel_hex((0x10000, MCUBOOT_HEADER), (slot_end - 2, bytes(4)))},
                       {"boot": intel_hex((0x0, VECTORS), (0xf7fe, bytes(4)))},
                       {"boot": APP_IMAGE}):
            with self.subTest(images=images):
                shutil.rmtree(self.root / "mcuboot", ignore_errors=True)
                self.images(**images)
                with self.assertRaisesRegex(SystemExit, "not qualified.*covers"):
                    self.flash()
        shutil.rmtree(self.root / "mcuboot")
        self.images(boot=intel_hex((0x0, VECTORS), (0xf7fc, bytes(4))),
                    signed=intel_hex((0x10000, MCUBOOT_HEADER), (slot_end - 4, bytes(4))))
        self.assertEqual(["mcuboot/zephyr/zephyr.hex", "app/zephyr/zephyr.signed.hex"], self.written())

    def test_flash_refuses_an_image_it_cannot_read(self):
        self.installed_configuration()
        records = APP_IMAGE.splitlines()
        for signed in ("", "\n", "not an image\n", ":00000001FF\n",
                       # Cut off before the end record, a wrong checksum, a record after the end.
                       "\n".join(records[:-1]) + "\n",
                       APP_IMAGE.replace(records[1], records[1][:-2] + "00"),
                       APP_IMAGE + records[1] + "\n",
                       APP_IMAGE.replace(records[1], records[1][1:]),
                       APP_IMAGE.replace(records[1], ":0100000600F9")):
            with self.subTest(signed=signed):
                shutil.rmtree(self.root / "mcuboot", ignore_errors=True)
                self.images(signed=signed)
                with self.assertRaisesRegex(SystemExit, "zephyr.signed.hex.*Intel HEX"):
                    self.flash()

    def test_flash_without_mcuboot_judges_the_single_image(self):
        self.installed_configuration()
        single, merged = self.root / "app/zephyr/zephyr.hex", self.root / "merged.hex"
        single.write_text(SINGLE_IMAGE)
        self.assertEqual(["app/zephyr/zephyr.hex"], self.written())
        # Bytes like the instruction on an odd address are data, not code.
        single.write_text(intel_hex((0x0, VECTORS + b"\x00" + words(0xe97fe97f) + b"\x00")))
        self.assertEqual(["app/zephyr/zephyr.hex"], self.written())
        merged.write_text(intel_hex(*SECURE_HALF, NONSECURE_HALF))
        with self.assertRaisesRegex(SystemExit, "merged.hex.*not qualified.*secure gateway"):
            self.flash()
        merged.unlink()
        single.write_text(intel_hex(NONSECURE_HALF))
        with self.assertRaisesRegex(SystemExit, "zephyr.hex.*not qualified.*covers 0x30000"):
            self.flash()


if __name__ == "__main__":
    unittest.main()
