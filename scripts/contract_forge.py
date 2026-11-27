#!/usr/bin/env python3
"""Run Foundry with the verified current contract compiler and settings."""

import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tomllib

if __name__ == "__main__":
    sys.dont_write_bytecode = True

from install_contract_compiler import BUILDS, VERSION, install, verify
from integration_workspace import run_command, scoped_workspace

ROOT = Path(__file__).resolve().parents[1]
PRODUCTS = {"p06": "p06-stablenet-contracts", "p08": "p08-market-services"}
RELEASE = VERSION.split("+", 1)[0]
COMMON_FLAGS = {"--offline", "--quiet", "-q", "-v", "-vv", "-vvv", "-vvvv", "-vvvvv", "--no-cache", "--force"}
TEST_FILTERS = {
    "--match-test", "--no-match-test", "--match-contract", "--no-match-contract", "--match-path", "--no-match-path",
}
OPTIONS = {
    # Positional arguments, flags, and options taking one value.
    "check": (0, set(), set()),
    "build": (0, {"--sizes", "--names"}, {"--skip", "--extra-output"}),
    "inspect": (2, {"--json"}, {"--skip"}),
    "test": (0, {"--gas-report", "--list"}, TEST_FILTERS),
    "snapshot": (0, {"--check"}, TEST_FILTERS),
    "script": (1, {"--broadcast", "--slow", "--unlocked"}, {
        "--rpc-url", "--sender", "--private-key", "--private-keys", "--keystores", "--password-file",
        "--priority-gas-price", "--with-gas-price",
    }),
}


def validate_arguments(args):
    command, *options = args
    if command not in OPTIONS:
        raise ValueError(f"unsupported contract Forge command: {command}")
    positional_count, flags, valued = OPTIONS[command]
    positionals = 0
    items = iter(options)
    for item in items:
        option, equals, value = item.partition("=")
        if not item.startswith("-"):
            positionals += 1
        elif option in valued:
            value = value if equals else next(items, "")
            if not value or value.startswith("-"):
                raise ValueError(f"{option} requires a value")
        elif equals or option not in flags and (command == "check" or option not in COMMON_FLAGS):
            raise ValueError(f"unsupported option for contract Forge {command}: {option}")
    if positionals != positional_count:
        raise ValueError(f"contract Forge {command} requires {positional_count} positional arguments")


def current_compiler(override, offline):
    host = platform.system(), platform.machine()
    if host not in BUILDS:
        raise ValueError(f"no pinned Solidity executable for {host}")
    distribution, digest = BUILDS[host]
    if override and override not in {RELEASE, f"solc:{RELEASE}"}:
        path = Path(override).resolve()
        if not path.is_file():
            raise ValueError(f"expected Solidity {RELEASE}, got {override!r}")
        verify(path, digest)
        return path

    binary = f"solc-{RELEASE}"
    cached = ROOT / "cache" / "solc" / binary
    home = Path.home()
    data = home / "Library/Application Support" if host[0] == "Darwin" else Path(
        os.environ.get("XDG_DATA_HOME", home / ".local/share")
    )
    for path in (cached, home / ".svm" / RELEASE / binary, data / "svm" / RELEASE / binary):
        if path.is_file():
            verify(path, digest)
            return path.resolve()
    if offline:
        raise ValueError(f"Solidity {RELEASE} required offline. Provision it with scripts/install_contract_compiler.py.")
    return install(cached, distribution, digest)


def configure(product, args, environment=None):
    validate_arguments(args)
    env = dict(os.environ if environment is None else environment)
    if "FOUNDRY_CONFIG" in env or env.get("FOUNDRY_PROFILE", "default") != "default":
        raise ValueError(f"{product}: contract Forge requires the repository default configuration")
    project = ROOT / "products" / PRODUCTS[product]
    with (project / "foundry.toml").open("rb") as handle:
        config = tomllib.load(handle)
        if config["profile"]["default"]["solc_version"] != RELEASE:
            raise ValueError(f"{product}: Foundry config must pin Solidity {RELEASE}")
    forge = env.get("FORGE") or "forge"
    effective = json.loads(subprocess.check_output([forge, "config", "--json"], cwd=project, env=env, text=True))
    settings = {
        "evm_version": "prague", "optimizer": True, "optimizer_runs": 200, "optimizer_details": None,
        "via_ir": False, "bytecode_hash": "ipfs", "cbor_metadata": True, "use_literal_content": False,
        "revert_strings": None, "libraries": [], "additional_compiler_profiles": [], "compilation_restrictions": [],
        "dynamic_test_linking": False,
        "src": "src", "test": "test", "script": "script",
        "remappings": (project / "remappings.txt").read_text().splitlines(),
        "auto_detect_remappings": product == "p06",
    }
    for setting, expected in settings.items():
        if effective.get(setting) != expected:
            raise ValueError(f"{product}: expected {setting}={expected}, got {effective.get(setting)}")
    if not effective.get("solc"):
        raise ValueError(f"{product}: missing effective compiler pin")
    compiler = current_compiler(effective["solc"], effective.get("offline") or "--offline" in args)
    env["FOUNDRY_SOLC"] = str(compiler)
    return forge, project, env, effective, config


# Forge 1.5.1 resolves these defaults independently of FOUNDRY_CACHE_PATH.
FAILURE_ARTIFACTS = (
    ("FOUNDRY_TEST_FAILURES_FILE", ("test_failures_file",), "cache/test-failures", "test-failures"),
    ("FOUNDRY_FUZZ_FAILURE_PERSIST_DIR", ("fuzz", "failure_persist_dir"), "cache/fuzz", "fuzz"),
    ("FOUNDRY_INVARIANT_FAILURE_PERSIST_DIR", ("invariant", "failure_persist_dir"), "cache/invariant", "invariant"),
)


def nested(config, path):
    value = config
    for key in path:
        if not isinstance(value, dict) or key not in value:
            return None
        value = value[key]
    return value


def run_guarded_test(product, args, work, environment=None):
    forge, project, env, effective, config = configure(product, args, environment)
    for variable, path, default, basename in FAILURE_ARTIFACTS:
        # Borrow explicit paths from caller overrides or effective configuration.
        # Consider both older top-level options and the default profile TOML.
        if (variable not in env and nested(config, path) is None
                and nested(config["profile"]["default"], path) is None
                and nested(effective, path) == default):
            env[variable] = str(work / basename)
    return run_command([forge, *args], cwd=project, env=env, check=True)


def main():
    if len(sys.argv) < 3 or sys.argv[1] not in PRODUCTS:
        raise SystemExit("usage: contract_forge.py p06|p08 <forge command> [arguments]")
    product, *args = sys.argv[1:]
    try:
        if args and args[0] in {"test", "snapshot"}:
            with scoped_workspace(f"nu54-{product}-forge-failures-") as work:
                run_guarded_test(product, args, work)
            return
        forge, project, env, _, _ = configure(product, args)
        if args == ["check"]:
            return
        os.chdir(project)
        os.execvpe(forge, [forge, *args], env)
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode if error.returncode >= 0 else 128 - error.returncode) from None
    except (OSError, ValueError) as error:
        raise SystemExit(str(error)) from None


if __name__ == "__main__":
    main()
