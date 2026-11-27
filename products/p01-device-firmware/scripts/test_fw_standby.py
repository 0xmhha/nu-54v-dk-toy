"""The --standby option of fw.py: its input, the refused combinations and the read-back of the configuration."""

from pathlib import Path
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch

import fw

ENVIRONMENT = {"NU54_INSTALL_AUTHORITY_HEX": "04" + "Ab" * 64, "NU54_RECOVERY_OVERLAY": "",
               "NU54_LAB_WALLET_CHAINS": "", "NU54_COMMISSIONED_WALLET_CHAINS": ""}


def configured(*flags):
    """The application configuration files of a release build, without running the toolchain."""
    with TemporaryDirectory() as root, patch.dict("os.environ", ENVIRONMENT), \
            patch.object(fw, "BUILD_DIR", Path(root)), patch.object(fw, "signing_key", return_value=Path("key.pem")), \
            patch.object(fw, "release_signer", return_value=(Path("public.pem"), "0")), \
            patch.object(fw.mbedtls_input, "prepare", return_value=Path("mbedtls")), \
            patch.object(fw.mcuboot_input, "prepare", return_value=Path("mcuboot")), \
            patch.object(fw, "in_toolchain", return_value=1) as toolchain:
        assert fw.main(["build", "--release", *flags]) == 1
    conf = next(arg for arg in toolchain.call_args.args if arg.startswith("-Dapp_EXTRA_CONF_FILE="))
    return [Path(path).name for path in conf.split("=", 1)[1].split(";")]


class StandbyBuildTest(unittest.TestCase):
    def test_adds_its_configuration_to_the_spending_wallet_only_when_asked(self):
        self.assertEqual(["release.conf", "pilot-tls.conf", "spending.conf", "enrollment-pin.conf", "standby.conf"],
                         configured("--spending", "--standby"))
        self.assertEqual(["release.conf", "pilot-tls.conf", "spending.conf", "enrollment-pin.conf", "standby.conf",
                          "secure-key.conf"], configured("--spending", "--standby", "--secure-key"))
        self.assertNotIn("standby.conf", configured("--spending"))
        self.assertEqual("CONFIG_SERIAL=n", [line for line in (fw.APP / "standby.conf").read_text().splitlines()
                                              if line and not line.startswith("#")][0])

    def test_refused_combinations(self):
        for flags in (["--standby"], ["--release", "--standby"], ["--rental", "--standby"],
                      ["--release", "--spending", "--standby", "--security-counter=2"]):
            with self.subTest(flags=flags), patch.dict("os.environ", ENVIRONMENT), \
                    patch.object(fw, "in_toolchain", side_effect=AssertionError("must not build")), \
                    self.assertRaisesRegex(SystemExit, "--standby"):
                fw.main(["build", *flags])

    def test_reads_the_generated_configuration_back(self):
        with TemporaryDirectory() as root, patch.object(fw, "BUILD_DIR", Path(root)):
            config = Path(root) / "app/zephyr/.config"
            config.parent.mkdir(parents=True)
            config.write_text("# CONFIG_SERIAL is not set\n# CONFIG_LOG is not set\nCONFIG_NU54_SPENDING_PROFILE=y\n")
            fw.standby_evidence()
            for kept in ("CONFIG_SERIAL=y", "CONFIG_UART_NRFX_UARTE=y", "CONFIG_UART_CONSOLE=y", "CONFIG_LOG=y"):
                with self.subTest(kept=kept):
                    config.write_text(f"CONFIG_NU54_SPENDING_PROFILE=y\n{kept}\n")
                    with self.assertRaisesRegex(SystemExit, kept):
                        fw.standby_evidence()


if __name__ == "__main__":
    unittest.main()
