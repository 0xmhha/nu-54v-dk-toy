#!/usr/bin/env python3
"""Build Fleet once in scratch and check actual protected O09 data over mTLS Chrome."""

import argparse
import os
from pathlib import Path
import signal
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[3]
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT / "scripts"))
from integration_workspace import OwnedProcess, cleanup_stack, run_environment, run_command


def terminate(signum, _frame):
    raise SystemExit(128 + signum)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--playwright-module", required=True, type=Path)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--zero", action="store_true")
    mode.add_argument("--held", action="store_true")
    mode.add_argument("--terminal-third-party", action="store_true")
    parser.add_argument("generation_args", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    extra = args.generation_args[1:] if args.generation_args[:1] == ["--"] else args.generation_args
    signal.signal(signal.SIGTERM, terminate)
    with tempfile.TemporaryDirectory(prefix="nu54-o09-chrome-", dir="/private/tmp") as owned:
        work = Path(owned)
        env = run_environment(work, os.environ)
        env.update(NU54_PLAYWRIGHT_MODULE=str(args.playwright_module.resolve()),
                   P05_O09_CHROME_DIST=str(work / "dist"), NODE_DISABLE_COMPILE_CACHE="1",
                   npm_config_cache=str(work / "npm"), XDG_CACHE_HOME=str(work / "xdg"))
        env.pop("P05_O09_CHROME_HOLD", None)
        if args.held:
            env["P05_O09_CHROME_HOLD"] = "1"
        web = ROOT / "products/p05-operations-backoffice/web"
        run_command(["pnpm", "exec", "tsc", "--noEmit"], cwd=web, env=env, check=True)
        run_command(["pnpm", "exec", "vite", "build", "--outDir", env["P05_O09_CHROME_DIST"]],
                    cwd=web, env=env, check=True)
        mode = "--zero-withdrawal-p05" if args.zero else "--protected-withdrawal-p05"
        if args.terminal_third_party:
            mode = "--recover-withdrawal-p05"
            env.update(P05_O09_WITHDRAWAL_RECONCILIATION="1", P05_O09_RETRY_WITHDRAWAL="third-party")
        with cleanup_stack() as stack:
            runner = OwnedProcess()
            stack.callback(runner.stop)
            child = runner.start([sys.executable, "-B", str(Path(__file__).with_name("generation-v1.py")), mode, *extra],
                                 cwd=ROOT, env=env)
            status = child.wait()
            if status:
                raise SystemExit(status if status > 0 else 128 - status)
    print("Protected O09 Chrome build, profile and credentials removed", flush=True)


if __name__ == "__main__":
    main()
