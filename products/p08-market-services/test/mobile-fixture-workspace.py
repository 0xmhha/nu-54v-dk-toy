"""Run the P08 mobile fixture with an owned build and process lifetime."""

import os
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parents[2] / "scripts"))

from integration_workspace import OwnedProcess, cleanup_stack, run_environment, scoped_workspace


def main(arguments, environment):
    with scoped_workspace("nu54-p08-mobile-") as work, cleanup_stack() as cleanup:
        env = run_environment(work, environment)
        for option in ("GOCACHE", "GOTMPDIR"):
            if option in environment:
                env[option] = environment[option]
        out = env.setdefault("FOUNDRY_OUT", str(work / "forge-out"))
        env.setdefault("FOUNDRY_CACHE_PATH", str(work / "forge-cache"))
        if not env.get("P08_ARTIFACT"):
            env["P08_ARTIFACT"] = str(Path(out) / "Market.sol" / "Market.json")
        env["P08_MOBILE_FIXTURE_MARKETD"] = str(work / "marketd")
        fixture = OwnedProcess()
        cleanup.callback(fixture.stop)
        rc = fixture.start(["bash", HERE / "mobile-fixture-worker.sh", *arguments], env=env).wait()
        return rc if rc >= 0 else 128 - rc


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:], os.environ))
