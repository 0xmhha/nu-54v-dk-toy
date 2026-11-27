#!/usr/bin/env python3
"""Run every implementation's check against the shared EIP-712 vectors (WBS2-P10-02, [N21]).

Each check is a command that exits 0 only if that implementation reproduces the vectors.
A check is SKIP, not FAIL, when a tool it needs is not installed or when the implementation
it tests does not exist yet. The kiosk and firmware checks run once their teams add the
entry points described in README.md next to this file.

Usage:
    python3 products/p10-platform/harness/run_conformance.py            SKIP does not fail
    python3 products/p10-platform/harness/run_conformance.py --strict   SKIP fails too (gate use)
"""

from __future__ import annotations

import json
import os
import shlex
import shutil
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable

ROOT = Path(__file__).resolve().parents[3]
KIOSK = ROOT / "products/p04-merchant-kiosk"
FIRMWARE = ROOT / "products/p01-device-firmware"
FIRMWARE_TEST = FIRMWARE / "test/test_eip712.c"
FIRMWARE_BUILD = "{owned-firmware-build}"

# Import shared ownership helpers without creating bytecode in the checkout.
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT / "scripts"))
from contract_forge import current_compiler
from host_test_workspace import python_test_env
from integration_workspace import WorkspaceCleanupError, run_command, run_environment, scoped_workspace


def kiosk_has_conformance_script() -> str | None:
    """None when ready, otherwise the reason to skip."""
    scripts = json.loads((KIOSK / "package.json").read_text()).get("scripts", {})
    return None if "test:conformance" in scripts else "kiosk has no test:conformance script yet"


def firmware_has_eip712_test() -> str | None:
    return None if FIRMWARE_TEST.exists() else "firmware has no test/test_eip712.c yet"


@dataclass
class Check:
    name: str
    cmd: list[str]
    cwd: str
    tools: list[str] = field(default_factory=list)
    ready: Callable[[], str | None] = lambda: None
    # Commands run before cmd in the same directory (for example a build step).
    before: list[list[str]] = field(default_factory=list)


CHECKS = [
    Check("vectors reproducible (cast)", ["python3", "docs/content/specifications/protocol/build_eip712_vectors.py", "--check"], ".", ["cast"]),
    # The generator formats Go output with gofmt; without it the Go file would look stale.
    Check("generated bindings match schema", ["python3", "schema-gen/generate.py", "--check"], "packages/protocol", ["gofmt"]),
    Check("protocol Go encodeType and vector signers by role", ["go", "test", "./..."], "packages/protocol/go", ["go"]),
    Check("protocol TS encodeType", ["pnpm", "test"], "packages/protocol/ts", ["pnpm"]),
    Check("protocol Python encodeType", ["uv", "run", "--quiet", "--locked", "--all-packages", "--extra", "quote-v2", "python", "-m", "pytest", "-q", "python/tests"], "packages/protocol", ["uv"]),
    Check("operations tool Go digests and setup session (vectors SV-13..15)", ["go", "test", "./internal/core/..."], "products/p05-operations-backoffice", ["go"]),
    Check("Go CBOR, fragments and session bodies", ["go", "test", "./protocol/..."], "packages/protocol/go", ["go"]),
    Check("CBOR message vectors round-trip", ["python3", "cbor-gen/generate.py", "--check"], "packages/protocol"),
    Check("BLE fragment vectors reassemble", ["python3", "frame-gen/generate.py", "--check"], "packages/protocol"),
    Check("session vectors reproduced by the device simulator", ["node", "--experimental-strip-types", "--no-warnings", "scripts/gen-session-vectors.ts", "--check"], "packages/device-sim", ["node"]),
    Check("contract Solidity typehashes", ["python3", "scripts/contract_forge.py", "p06", "test", "--offline", "--match-contract", "PaymentTypesTest"], ".", ["forge"]),
    Check("contract Solidity EIP-712 digest and signer", ["python3", "scripts/contract_forge.py", "p06", "test", "--offline", "--match-contract", "Eip712VectorsTest"], ".", ["forge"]),
    Check("kiosk TypeScript EIP-712, CBOR and fragments", ["pnpm", "run", "test:conformance"], "products/p04-merchant-kiosk", ["pnpm"], kiosk_has_conformance_script),
    Check(
        "firmware C EIP-712, signatures, CBOR, fragments and sessions (host)",
        ["ctest", "--test-dir", FIRMWARE_BUILD, "-R", "eip712_vectors|frame_vectors|cbor_vectors|session_vectors", "--output-on-failure", "--no-tests=error"],
        "products/p01-device-firmware",
        ["cmake", "ctest"],
        firmware_has_eip712_test,
        [["cmake", "-S", "test", "-B", FIRMWARE_BUILD], ["cmake", "--build", FIRMWARE_BUILD]],
    ),
]


def command_environment(work: Path) -> dict[str, str]:
    tmp = work / "tmp"
    tmp.mkdir()
    env = python_test_env(os.environ, dict(os.environ))
    env.update(TMPDIR=str(tmp), TEMP=str(tmp), TMP=str(tmp))
    for variable, file in (
        ("PYTHONPYCACHEPREFIX", "pycache"), ("UV_CACHE_DIR", "uv-cache"), ("UV_PROJECT_ENVIRONMENT", "venv"),
        ("UV_PYTHON_INSTALL_DIR", "uv-python"), ("FOUNDRY_OUT", "forge-out"), ("FOUNDRY_CACHE_PATH", "forge-cache"),
        ("FOUNDRY_TEST_FAILURES_FILE", "forge-test-failures"), ("FOUNDRY_FUZZ_FAILURE_PERSIST_DIR", "forge-fuzz"),
        ("FOUNDRY_INVARIANT_FAILURE_PERSIST_DIR", "forge-invariant"),
    ):
        env.setdefault(variable, str(work / file))
    return env


def go_environment(work: Path, environment: dict[str, str]) -> dict[str, str]:
    work.mkdir(exist_ok=True)
    env = run_environment(work, environment)
    for key in ("GOCACHE", "GOTMPDIR"):
        if key in environment:
            env[key] = environment[key]
    if "GOMODCACHE" in environment:
        return env
    paths = json.loads(run_command(["go", "env", "-json", "GOMODCACHE", "GOPROXY", "GOFLAGS"], env=env,
                                   capture_output=True, text=True, check=True).stdout)
    # Read already downloaded module archives as a local proxy, and direct
    # extraction, build outputs and any new downloads to the owned workspace.
    archive = Path(paths["GOMODCACHE"]) / "cache" / "download"
    if archive.is_dir():
        env["GOPROXY"] = archive.resolve().as_uri() + "|" + paths["GOPROXY"]
    env["GOMODCACHE"] = str(work / "modules")
    # By default Go makes downloaded module directories read-only.
    env["GOFLAGS"] = (paths.get("GOFLAGS", "") + " -modcacherw").strip()
    return env


def python_conformance(command: list[str], work: Path, cwd: Path,
                       environment: dict[str, str]) -> tuple[list[str], dict[str, str]]:
    env = environment
    if environment.get("UV_OFFLINE", "").lower() in ("true", "1", "yes", "on"):
        configured = environment["UV_PROJECT_ENVIRONMENT"]
        borrowed = ROOT / ".venv" if configured == str(work / "venv") else ROOT / configured
        if (borrowed / "bin/python").exists():
            candidate = {**environment, "UV_PROJECT_ENVIRONMENT": str(borrowed)}
            check = run_command(["uv", "sync", "--check", "--locked", "--offline", "--quiet", "--all-packages", "--extra", "quote-v2"],
                                cwd=cwd, env=candidate, capture_output=True, text=True)
            if check.returncode == 0:
                # --no-sync borrows the pre-provisioned environment without
                # touching its installed files; --check validated the lock.
                command = ["--no-sync" if arg == "--locked" else arg for arg in command]
                env = candidate
            else:
                env = {**environment, "UV_PROJECT_ENVIRONMENT": str(work / "venv")}
    defaults = shlex.join(["-o", f"cache_dir={work / 'pytest-cache'}", "--basetemp", str(work / "pytest-tmp")])
    return command, {**env, "PYTEST_ADDOPTS": f"{defaults} {env.get('PYTEST_ADDOPTS', '')}"}


def forge_environment(work: Path, environment: dict[str, str]) -> dict[str, str]:
    if environment.get("FOUNDRY_SOLC"):
        return environment
    try:
        compiler = current_compiler(None, offline=True)
    except ValueError as error:
        if "required offline" not in str(error) or environment.get("FOUNDRY_OFFLINE", "").lower() in ("true", "1"):
            raise
        compiler = work / "solc"
        if not compiler.exists():
            run_command([sys.executable, ROOT / "scripts/install_contract_compiler.py", compiler],
                        cwd=ROOT, env=environment, capture_output=True, text=True, check=True)
    return {**environment, "FOUNDRY_SOLC": str(compiler)}


def run(check: Check, work: Path, environment: dict[str, str], go_env) -> tuple[str, str]:
    """Returns (status, detail) with status PASS, FAIL or SKIP."""
    missing = [t for t in check.tools if shutil.which(t) is None]
    if missing:
        return "SKIP", f"tool not installed: {', '.join(missing)}"
    reason = check.ready()
    if reason:
        return "SKIP", reason
    try:
        env = go_env() if check.cmd[0] == "go" else environment
        if "forge" in check.tools:
            env = forge_environment(work, env)
        for original in [*check.before, check.cmd]:
            cmd = [str(work / "firmware") if arg == FIRMWARE_BUILD else arg for arg in original]
            if cmd[0] == "uv":
                cmd, env = python_conformance(cmd, work, ROOT / check.cwd, env)
            proc = run_command(cmd, cwd=ROOT / check.cwd, env=env, capture_output=True, text=True)
            if proc.returncode != 0:
                return "FAIL", (proc.stdout + proc.stderr)[-2000:]
    except subprocess.CalledProcessError as error:
        return "FAIL", ((error.stdout or "") + (error.stderr or ""))[-2000:]
    except WorkspaceCleanupError:
        raise
    except (OSError, ValueError, RuntimeError) as error:
        return "FAIL", str(error)
    return "PASS", ""


def main(argv: list[str]) -> int:
    strict = "--strict" in argv
    counts = {"PASS": 0, "FAIL": 0, "SKIP": 0}
    with scoped_workspace("nu54-conformance-") as work:
        env = command_environment(work)
        cached_go = None

        def go_env():
            nonlocal cached_go
            if cached_go is None:
                cached_go = go_environment(work / "go", env)
            return cached_go

        for check in CHECKS:
            status, detail = run(check, work, env, go_env)
            counts[status] += 1
            print(f"{status}  {check.name}" + (f"  ({detail})" if status == "SKIP" else ""))
            if status == "FAIL":
                print(detail)
    print(f"{counts['PASS']} passed, {counts['FAIL']} failed, {counts['SKIP']} skipped of {len(CHECKS)}")
    if counts["FAIL"] or (strict and counts["SKIP"]):
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
