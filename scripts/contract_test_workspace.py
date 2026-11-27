"""Run a product's normal contract tests with owned default build paths."""

import argparse
import os
from pathlib import Path
import subprocess
import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

from check_contract_compatibility import main as check_compatibility
from contract_forge import run_guarded_test
from integration_workspace import scoped_workspace, run_command, run_environment

ROOT = Path(__file__).resolve().parents[1]
PROJECTS = {
    "p06": ROOT / "products/p06-stablenet-contracts",
    "p08": ROOT / "products/p08-market-services",
}


def test_product(product, snapshot_filters, environment):
    compiler = [sys.executable, "-B", ROOT / "scripts/contract_forge.py", product]
    with scoped_workspace(f"nu54-{product}-test-") as work:
        if product == "p08":
            env = run_environment(work, environment)
            for key in ("GOCACHE", "GOTMPDIR"):
                if key in environment:
                    env[key] = environment[key]
            commands = [[*compiler, "build", "--quiet"], [*compiler, "test", "--offline"], ["go", "test", "./..."]]
        else:
            env = environment.copy()
            commands = [[*compiler, "test", "--offline"], [*compiler, "snapshot", "--offline", "--check", *snapshot_filters]]
        # Explicit paths are borrowed and stay outside the owned workspace.
        for key, default in (("FOUNDRY_OUT", "out"), ("FOUNDRY_CACHE_PATH", "foundry-cache")):
            # Forge resolves relative paths from the product root, but Go
            # runs test binaries from their package directories.
            value = env.get(key)
            env[key] = str(PROJECTS[product] / value if value else work / default)
        check_compatibility([product], env)
        for command in commands:
            if command[:4] == compiler and command[4] in {"test", "snapshot"}:
                run_guarded_test(product, command[4:], work, env)
            else:
                run_command(command, cwd=PROJECTS[product], env=env, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("product", choices=PROJECTS)
    parser.add_argument("snapshot_filters", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    filters = args.snapshot_filters
    if filters[:1] == ["--"]:
        filters = filters[1:]
    try:
        test_product(args.product, filters, os.environ)
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode if error.returncode >= 0 else 128 - error.returncode) from None
    except (OSError, ValueError) as error:
        raise SystemExit(str(error)) from None


if __name__ == "__main__":
    main()
