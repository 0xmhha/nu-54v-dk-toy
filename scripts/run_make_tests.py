#!/usr/bin/env python3
"""Run the root Makefile's tests with disposable default Go caches."""

import json
import os
import re
import shlex
import shutil
import sys

from integration_workspace import run_command, run_environment, scoped_workspace


def jobserver_descriptors(flags):
    match = re.search(r"(?:^|\s)--jobserver-(?:fds|auth)=(\d+),(\d+)(?:\s|$)", flags)
    if not match:
        return ()
    descriptors = tuple(map(int, match.groups()))
    try:
        for descriptor in descriptors:
            os.fstat(descriptor)
    except OSError:
        return ()
    return descriptors


def project_environment(work, environment):
    configured = {}
    if shutil.which("go", path=environment.get("PATH")):
        settings = run_command(["go", "env", "-changed", "-json", "GOCACHE", "GOTMPDIR"],
                               env=environment, capture_output=True, text=True, check=True)
        configured = json.loads(settings.stdout)
        env = run_environment(work, environment)
    else:
        env = environment.copy()
        temporary = work / "tmp"
        temporary.mkdir()
        cache = work / "go-cache"
        cache.mkdir()
        env.update(TMPDIR=str(temporary), TEMP=str(temporary), TMP=str(temporary),
                   GOTMPDIR=str(temporary), GOCACHE=str(cache), PYTHONDONTWRITEBYTECODE="1")
    for key in ("GOCACHE", "GOTMPDIR"):
        value = environment.get(key) or configured.get(key)
        if value:
            env[key] = value
    return env


def main(make, projects, environment):
    jobs = jobserver_descriptors(environment.get("MAKEFLAGS", ""))
    command = shlex.split(make)
    with scoped_workspace("nu54-root-test-") as work:
        env = project_environment(work, environment)
        for project in projects:
            print(f"== {project}: test", flush=True)
            child = run_command([*command, "-C", project, "test"], env=env, pass_fds=jobs)
            if child.returncode:
                return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1], sys.argv[2:], os.environ))
