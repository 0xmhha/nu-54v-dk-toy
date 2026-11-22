"""Own default host build and Python test caches for their Make recipes."""

import os
from pathlib import Path
import shlex
import subprocess
import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

from integration_workspace import run_command, run_environment, scoped_workspace

ROOT = Path(__file__).resolve().parents[1]


def python_test_env(environment, env):
    if "PYTHONPYCACHEPREFIX" not in environment:
        env["PYTHONDONTWRITEBYTECODE"] = "1"
    elif "PYTHONDONTWRITEBYTECODE" in environment:
        env["PYTHONDONTWRITEBYTECODE"] = environment["PYTHONDONTWRITEBYTECODE"]
    else:
        env.pop("PYTHONDONTWRITEBYTECODE", None)
    return env


def run_firmware(make, build, environment):
    with scoped_workspace("nu54-p01-host-test-") as work:
        env = python_test_env(environment, run_environment(work, environment))
        for key in ("GOCACHE", "GOTMPDIR"):
            if key in environment:
                env[key] = environment[key]
        output = build if build is not None else work / "host"
        run_command([make, "_host-test", f"HOST_BUILD={output}"],
                    cwd=ROOT / "products/p01-device-firmware", env=env, check=True)


def run_protocol_python(environment):
    with scoped_workspace("nu54-protocol-python-test-") as work:
        env = python_test_env(environment, environment.copy())
        env.update(TMPDIR=str(work), TEMP=str(work), TMP=str(work))
        # Pytest reads caller options after these defaults, including explicit
        # cache_dir and basetemp overrides.
        defaults = shlex.join(["-o", f"cache_dir={work / 'pytest-cache'}", "--basetemp", str(work / "pytest-temp")])
        env["PYTEST_ADDOPTS"] = f"{defaults} {env.get('PYTEST_ADDOPTS', '')}"
        run_command(["uv", "run", "--locked", "--all-packages", "--extra", "quote-v2", "pytest", "-q", "python/tests"],
                    cwd=ROOT / "packages/protocol", env=env, check=True)


def main():
    args = sys.argv[1:]
    try:
        if args == ["protocol-python"]:
            run_protocol_python(os.environ)
        elif len(args) == 4 and args[0] == "firmware":
            _, make, origin, build = args
            run_firmware(make, None if origin == "file" else build, os.environ)
        else:
            raise SystemExit("expected protocol-python or firmware MAKE HOST_BUILD_ORIGIN HOST_BUILD")
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode if error.returncode >= 0 else 128 - error.returncode) from None


if __name__ == "__main__":
    main()
