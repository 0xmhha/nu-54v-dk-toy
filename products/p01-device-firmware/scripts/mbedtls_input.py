"""Prepare the pinned upstream Mbed TLS source for Zephyr without editing the SDK."""

import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
from urllib.request import urlopen

VERSION = "4.1.2"
COMMIT = "8808d31cdffb1a2c82645a531a96a0f26dd3e8c3"
SHA256 = "392215e5e4846df04e2f4c4516079303bd73edde837d8f3e68fb4d4d7ddf0126"
NAME = f"mbedtls-{VERSION}"
URL = f"https://github.com/Mbed-TLS/mbedtls/releases/download/{NAME}/{NAME}.tar.bz2"
GLUE = Path(__file__).with_name("mbedtls-ncs.patch")


def digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def prepare(directory: Path) -> Path:
    archive = directory / f"{NAME}.tar.bz2"
    supplied = os.environ.get("NU54_MBEDTLS_ARCHIVE")
    try:
        if supplied:
            shutil.copyfile(supplied, archive)
        else:
            with urlopen(URL, timeout=90) as response, archive.open("wb") as output:
                shutil.copyfileobj(response, output)
        if digest(archive) != SHA256:
            raise ValueError("Mbed TLS archive SHA-256 does not match the pinned upstream release")
        with tarfile.open(archive, "r:bz2") as source:
            source.extractall(directory, filter="data")
    except (OSError, ValueError, tarfile.TarError) as error:
        raise SystemExit(f"Unable to prepare pinned Mbed TLS: {error}") from error
    root = directory / NAME
    info = (root / "include/mbedtls/build_info.h").read_text()
    if not re.search(r'^#define MBEDTLS_VERSION_NUMBER\s+0x04010200$', info, re.MULTILINE):
        raise SystemExit("Pinned Mbed TLS archive has an unexpected build version")
    if subprocess.run(["git", "apply", str(GLUE)], cwd=root).returncode:
        raise SystemExit("Pinned Mbed TLS integration patch does not apply")
    return root


def evidence(root: Path, build: Path, nm: Path) -> dict:
    image = build / "app"
    elf = image / "zephyr/zephyr.elf"
    link_map = (image / "zephyr/zephyr.map").read_text()
    commands = json.loads((image / "compile_commands.json").read_text())
    sources = {Path(command["file"]).resolve() for command in commands}
    elf_symbols = subprocess.check_output([str(nm), "--defined-only", str(elf)], text=True)
    linked = {}
    for source, library, symbol in (
        ("ssl_tls13_server.c", "libmbedtls.a", "mbedtls_ssl_tls13_handshake_server_step"),
        ("ssl_tls.c", "libmbedtls.a", "mbedtls_ssl_handshake"),
        ("x509_crt.c", "libmbedx509.a", "mbedtls_x509_crt_verify_restartable"),
    ):
        path = root / "library" / source
        archive_member = re.escape(f"{library}({source}.obj)")
        if path.resolve() not in sources or not re.search(
            rf"{archive_member}\n[ \t]+0x[0-9a-fA-F]+[ \t]+{symbol}\s*$", link_map, re.MULTILINE
        ) or not re.search(rf"^[0-9a-fA-F]+ [Tt] {symbol}$", elf_symbols, re.MULTILINE):
            raise SystemExit(f"Unable to verify linked Mbed TLS component {source}")
        linked[source] = {"symbol": symbol, "sourceSha256": digest(path)}
    return {
        "name": "Mbed TLS",
        "version": VERSION,
        "upstreamCommit": COMMIT,
        "archiveSha256": SHA256,
        "integrationPatchSha256": digest(GLUE),
        "purl": f"pkg:github/Mbed-TLS/mbedtls@mbedtls-{VERSION}",
        "linked": linked,
    }


def write_sbom(path: Path, component: dict, psa_directory: Path) -> None:
    # This release inventory covers the two TLS-stack modules. Other firmware
    # inputs remain identified by the frozen SDK manifest and artifact hashes.
    psa = (psa_directory / "include/oberon-psa-crypto/build_info.h").read_text()
    version = re.findall(r"^#define OBERON_PSA_CRYPTO_VERSION_(?:MAJOR|MINOR|PATCH)\s+(\d+)$", psa, re.MULTILINE)
    if len(version) != 3:
        raise SystemExit("Unable to identify the selected Nordic PSA provider")
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=psa_directory, text=True).strip()
    path.write_text(json.dumps({
        "bomFormat": "CycloneDX",
        "specVersion": "1.6",
        "version": 1,
        "components": [
            {"type": "library", "name": component["name"], "version": component["version"], "purl": component["purl"],
             "properties": [{"name": "nu54:upstreamCommit", "value": component["upstreamCommit"]},
                            {"name": "nu54:archiveSha256", "value": component["archiveSha256"]},
                            {"name": "nu54:integrationPatchSha256", "value": component["integrationPatchSha256"]}]},
            {"type": "library", "name": "Oberon PSA Crypto", "version": ".".join(version),
             "properties": [{"name": "nu54:nordicCommit", "value": commit}]},
        ],
    }, sort_keys=True, indent=2) + "\n")
