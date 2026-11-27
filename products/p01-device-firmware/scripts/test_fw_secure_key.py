"""The --secure-key build profile of fw.py: target, inputs, refused combinations and host evidence."""

import importlib
import os
from pathlib import Path
import re
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
# MCUboot: its vectors, and the entry of its flash map that names the application slot it starts
# (id and device, start, size), as the real images hold it.
BOOT_VECTORS = words(0x20005208, 0x00005995, 0x0000689f, 0x00005981)
INSTALLED_SLOT, SECURE_KEY_SLOT = words(0x10000, 0xb2000), words(0x10000, 0x157000)
BOOT_CODE = BOOT_VECTORS + words(0x00000001) + INSTALLED_SLOT
BOOT_IMAGE = intel_hex((0x0, BOOT_CODE))
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

    def test_rejects_a_software_signer_in_the_application(self):
        """Both PSA calls come from the TF-M API and no driver is linked, and the application still signs by itself."""
        def placed(name, origin="app/libapp.a(wallet.c.obj)", address=0x41000):
            return key_call(name, origin, address)

        for name in ("secp256k1_ecdsa_sign", "secp256k1_ecdsa_sign_recoverable", "ocrypto_ecdsa_p256k1_det_sign_hash",
                     "ocrypto_ecdsa_p256_sign", "uECC_sign", "mbedtls_ecdsa_sign_det_ext", "mbedtls_ecdsa_write_signature"):
            with self.subTest(name=name):
                self.write(app_map=link_map(KEY_CALLS_FROM_TFM + placed(name)))
                with self.assertRaisesRegex(SystemExit, f"signs with a key of its own: {name}$"):
                    self.evidence()
        # A section without a symbol line, and a section whose name the compiler extended.
        for section in (" .text.secp256k1_ecdsa_sign\n                0x00041000       0x58 app/libapp.a(w.c.obj)\n",
                        " .text.secp256k1_ecdsa_sign.constprop.0\n                0x00041000       0x58 app/libapp.a(w.c.obj)\n"):
            with self.subTest(section=section):
                self.write(app_map=link_map(KEY_CALLS_FROM_TFM + section))
                with self.assertRaisesRegex(SystemExit, "signs with a key of its own: secp256k1_ecdsa_sign$"):
                    self.evidence()
        # What the real application holds: parsing, normalising and verifying signatures, and the PSA calls.
        kept = "".join(placed(name, address=0x41000 + 0x100 * index) for index, name in enumerate(
            ("secp256k1_ecdsa_signature_normalize", "secp256k1_ecdsa_signature_parse_compact", "secp256k1_ecdsa_recover",
             "secp256k1_ec_pubkey_create", "mbedtls_pk_sign", "mbedtls_ecdsa_raw_to_der", "device_spending_sign")))
        self.write(app_map=link_map(KEY_CALLS_FROM_TFM + kept))
        self.assertEqual(0, self.evidence())
        # A signer the linker threw away is not in the image. Its strings may stay, as in the real build.
        gone = placed("secp256k1_ecdsa_sign", address=0)
        strings = " .rodata.secp256k1_ecdsa_sign.str1.1\n                0x0007934e       0x5d libnu54_secp256k1.a(secp256k1.c.obj)\n"
        self.write(app_map=link_map(KEY_CALLS_FROM_TFM + strings, discarded=gone, dropped=gone))
        self.assertEqual(0, self.evidence())

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
                       {"boot": intel_hex((0x0, BOOT_CODE), (0xf7fe, bytes(4)))},
                       {"boot": APP_IMAGE}):
            with self.subTest(images=images):
                shutil.rmtree(self.root / "mcuboot", ignore_errors=True)
                self.images(**images)
                with self.assertRaisesRegex(SystemExit, "not qualified.*covers"):
                    self.flash()
        shutil.rmtree(self.root / "mcuboot")
        self.images(boot=intel_hex((0x0, BOOT_CODE), (0xf7fc, bytes(4))),
                    signed=intel_hex((0x10000, MCUBOOT_HEADER), (slot_end - 4, bytes(4))))
        self.assertEqual(["mcuboot/zephyr/zephyr.hex", "app/zephyr/zephyr.signed.hex"], self.written())

    def test_flash_refuses_a_bootloader_of_another_layout_beside_the_installed_configuration(self):
        """MCUboot of the --secure-key build has no gateway and fits the boot area. Its flash map tells."""
        self.installed_configuration()
        for boot in (intel_hex((0x0, BOOT_VECTORS + words(1) + SECURE_KEY_SLOT)),
                     intel_hex((0x0, BOOT_VECTORS)),
                     # The words of the installed slot, but not as an entry of a table of words.
                     intel_hex((0x0, BOOT_VECTORS + b"\x00\x00" + INSTALLED_SLOT + b"\x00\x00")),
                     intel_hex((0x0, BOOT_VECTORS + words(0xb2000, 0x10000))),
                     # The start in one run and the size in the next.
                     intel_hex((0x0, BOOT_VECTORS + INSTALLED_SLOT[:4]), (0x100, INSTALLED_SLOT[4:]))):
            with self.subTest(boot=boot):
                shutil.rmtree(self.root / "mcuboot", ignore_errors=True)
                self.images(boot=boot)
                with self.assertRaisesRegex(SystemExit, "mcuboot/zephyr/zephyr.hex.*not qualified.*application slot 0x10000-0xc2000"):
                    self.flash()
        shutil.rmtree(self.root / "mcuboot")
        self.images(boot=intel_hex((0x0, BOOT_VECTORS), (0x7a30, words(1, 1) + INSTALLED_SLOT)))
        self.assertEqual(["mcuboot/zephyr/zephyr.hex", "app/zephyr/zephyr.signed.hex"], self.written())
        # The application is not asked for a flash map.
        self.assertIsNone(fw.unqualified_image(self.root / "app/zephyr/zephyr.signed.hex", fw.APP_SLOT))

    def test_flash_refuses_records_that_go_back_in_the_address_space(self):
        """The same TF-M bytes in records that are no neighbours: no run holds a whole gateway."""
        self.installed_configuration()

        def record(kind, offset, data):
            body = bytes([len(data), offset >> 8, offset & 0xff, kind]) + data
            return ":" + (body + bytes([-sum(body) & 0xff])).hex().upper()

        gateway = words(0xe97fe97f, 0xbb84f006)
        ascending = [record(4, 0, b"\x00\x01"), record(0, 0x0000, MCUBOOT_HEADER[:16]), record(0, 0x0010, MCUBOOT_HEADER[16:]),
                     record(0, 0x0800, VECTORS), record(0, 0x0c80, gateway[:2]), record(0, 0x0c82, gateway[2:]),
                     record(1, 0, b"")]
        self.images(signed="\n".join(ascending) + "\n")
        with self.assertRaisesRegex(SystemExit, "not qualified.*secure gateway.*0x10c80"):
            self.flash()
        # The second half of the instruction first, and its first half after the vectors.
        split = [ascending[0], ascending[1], ascending[2], ascending[5], ascending[3], ascending[4], ascending[6]]
        # One address written twice: the flasher would keep the later bytes.
        twice = [*ascending[:4], record(0, 0x0800, VECTORS), ascending[6]]
        # Back to a lower address after a record that sets the upper address bits again.
        wrapped = [*ascending[:4], record(4, 0, b"\x00\x00"), record(0, 0xfff0, bytes(16)), ascending[6]]
        for name, lines in (("split", split), ("twice", twice), ("wrapped", wrapped)):
            with self.subTest(name=name):
                shutil.rmtree(self.root / "mcuboot")
                self.images(signed="\n".join(lines) + "\n")
                with self.assertRaisesRegex(SystemExit, "zephyr.signed.hex.*Intel HEX image with ascending addresses"):
                    self.flash()

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


class BuildDirectoryTest(unittest.TestCase):
    """The directory this script judges is the directory the toolchain builds and flashes."""

    def test_a_relative_build_directory_names_one_place(self):
        with TemporaryDirectory() as root:
            root = Path(root).resolve()
            self.assertEqual(fw.PRODUCT / "build", fw.build_directory(None))
            self.assertEqual(Path("/somewhere/else"), fw.build_directory("/somewhere/else"))
            before = os.getcwd()
            os.chdir(root)
            try:
                self.assertEqual(root / "out/board", fw.build_directory("out/board"))
                self.assertEqual(root.parent / "board", fw.build_directory("../board"))
            finally:
                os.chdir(before)

    def test_the_flasher_and_west_receive_absolute_paths(self):
        """The toolchain is started in the SDK directory, where a relative path names another place."""
        with TemporaryDirectory() as root:
            root = Path(root).resolve()
            build = root / "work/out/board"
            for path, text in (("app/zephyr/.config", "CONFIG_NU54_SPENDING_PROFILE=y\n"),
                               ("app/CMakeCache.txt", "BOARD:STRING=nu54v_dk/nrf54l15/cpuapp\n"),
                               ("mcuboot/zephyr/.config", ""), ("mcuboot/zephyr/zephyr.hex", BOOT_IMAGE),
                               ("app/zephyr/zephyr.signed.hex", APP_IMAGE)):
                (build / path).parent.mkdir(parents=True, exist_ok=True)
                (build / path).write_text(text)
            # Under the SDK directory the same relative path holds a TrustZone image.
            sdk = root / "sdk"
            (sdk / "out/board/app/zephyr").mkdir(parents=True)
            (sdk / "out/board/app/zephyr/zephyr.signed.hex").write_text(TFM_IMAGE)
            before = os.getcwd()
            os.chdir(root / "work")
            self.addCleanup(importlib.reload, fw)
            try:
                with patch.dict("os.environ", {"P01_BUILD_DIR": "out/board", "NCS_DIR": str(sdk)}):
                    importlib.reload(fw)
                self.assertEqual(build, fw.BUILD_DIR)
                with patch.object(fw, "nrfutil", return_value="nrfutil"), \
                        patch.object(fw.subprocess, "run") as run:
                    run.return_value.returncode = 0
                    self.assertEqual(0, fw.main(["flash"]))
                    flashed = [(call.args[0][-1], call.kwargs["cwd"]) for call in run.call_args_list]
                    self.assertEqual(0, fw.main(["build"]))
                    west = run.call_args.args[0]
            finally:
                os.chdir(before)
            self.assertEqual([(str(build / "mcuboot/zephyr/zephyr.hex"), sdk), (str(build / "app/zephyr/zephyr.signed.hex"), sdk)],
                             flashed)
            self.assertEqual(str(build), west[west.index("-d") + 1])

    def test_flash_refuses_a_relative_build_directory(self):
        """Whatever set the directory: the flasher is never handed a path that depends on where it runs."""
        with TemporaryDirectory() as root:
            build = Path(root).resolve() / "out"
            for path, text in (("app/zephyr/.config", ""), ("app/zephyr/zephyr.hex", SINGLE_IMAGE)):
                (build / path).parent.mkdir(parents=True, exist_ok=True)
                (build / path).write_text(text)
            before = os.getcwd()
            os.chdir(build.parent)
            try:
                with patch.object(fw, "BUILD_DIR", Path("out")), \
                        patch.object(fw, "in_toolchain", side_effect=AssertionError("must not flash")), \
                        self.assertRaisesRegex(SystemExit, "absolute"):
                    fw.main(["flash"])
            finally:
                os.chdir(before)


class InstalledLayoutTest(unittest.TestCase):
    def test_the_flash_areas_are_the_partitions_of_the_sdk(self):
        """`flash` judges images against these three areas; the board takes them from the SoC's partition file."""
        partitions = fw.NCS_DIR / "zephyr/dts/vendor/nordic/nrf54l15_cpuapp_partition.dtsi"
        if not partitions.is_file():
            self.skipTest(f"no SDK at {fw.NCS_DIR}")
        board = (fw.PRODUCT / "boards/nucode/nu54v_dk/nu54v_dk_nrf54l15_cpuapp.dts").read_text()
        self.assertIn("#include <vendor/nordic/nrf54l15_cpuapp_partition.dtsi>", board)
        self.assertNotIn("WITH_FLPR_PARTITIONS", board)
        # The part of the file that applies without the FLPR core's partitions.
        text = partitions.read_text()
        common, _, rest = text.partition("#ifdef WITH_FLPR_PARTITIONS")
        _, _, default = rest.partition("#else")
        default, _, _ = default.partition("#endif")
        found = {label: (int(start, 16), int(start, 16) + int(kib) * 1024) for label, start, kib in re.findall(
            r'label = "([\w-]+)";(?:\s*/\*.*?\*/)?\s*reg = <(0x[0-9a-fA-F]+) DT_SIZE_K\((\d+)\)>', common + default, re.DOTALL)}
        self.assertEqual({"mcuboot", "image-0", "image-1", "storage"}, set(found))
        self.assertEqual(fw.BOOT_AREA, found["mcuboot"])
        self.assertEqual(fw.APP_SLOT, found["image-0"])
        self.assertEqual(fw.SINGLE_IMAGE_AREA, (found["mcuboot"][0], found["storage"][0]))


if __name__ == "__main__":
    unittest.main()
