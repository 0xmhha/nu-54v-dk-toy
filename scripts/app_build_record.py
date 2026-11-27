"""Tell whether the debug app on an emulator was built from the sources as they are now.

A debug app takes its screens from the bundler at run time, so those are never old. What the build puts into the
APK can be: native code, the manifest, the native interface specs, build settings and the dependency lock.
build() runs the build and writes the contents' digests of those files beside the APK; stale() compares that
record with the working tree and with the APK installed on the emulator. Modification times play no part.
"""

from dataclasses import dataclass
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys


@dataclass(frozen=True)
class Inputs:
    """The files a build reads: everything below `take`, without what is below `leave`, except what is below `keep`.

    `written` names, as git ignore patterns from the root, the new files below `take` that are no source: what the
    build, the editor and the machine write. `installed` pairs each lockfile with the copy of it that the package
    manager keeps of its last install.
    """
    take: tuple
    leave: tuple = ()
    keep: tuple = ()
    written: tuple = ()
    installed: tuple = ()

    def has(self, name):
        def below(places):
            return any(name == place or name.startswith(place + "/") for place in places)
        return below(self.take) and not name.endswith(".md") and (below(self.keep) or not below(self.leave))


# The user app's debug APK. The whole app directory is taken, so a new native directory or build file counts without a
# change here; what is left out is named.
USER_APP = Inputs(
    take=("products/p02-user-app", "packages/protocol/platform",  # nu54-mobile.gradle adds the package's Kotlin sources to the app
          "scripts/android/legal-notices.gradle", "scripts/android/release-signers.json",  # applied by app/build.gradle
          "scripts/android/release-signing.gradle", "scripts/android/runtime-constraints.gradle",
          ".npmrc", "pnpm-lock.yaml"),  # which native dependencies node_modules holds, and where
    leave=("products/p02-user-app/src",  # screens and logic: the bundler serves them as they are
           *(f"products/p02-user-app/{name}" for name in (  # read by the bundler, the tests and the checks only
               "index.js", "app.json", "babel.config.js", "metro.config.js", "build-tools", "jest.config.js", "tsconfig.json",
               ".eslintrc.js", ".prettierrc.js", ".watchmanconfig", "Makefile", "test", "android/app/src/androidTest")),
           "packages/protocol/platform/test",
           "products/p02-user-app/ios", "packages/protocol/platform/ios", "packages/protocol/platform/Nu54ProtocolMobile.podspec"),
    keep=("products/p02-user-app/src/specs",),  # the build generates native code from these
    # The ignore rules of the repository are not used: they name build/, out/, cache/, dist/ and bin/ wherever they lie,
    # and a new package of that name is a source. The build directories are named by their place instead.
    written=(*(f"/products/p02-user-app/android/{name}/" for name in ("build", "app/build", "app/.cxx", ".gradle", ".kotlin")),
             "node_modules/", ".idea/", "*.iml", "*.hprof", ".DS_Store",
             "local.properties", "*.keystore"),  # where this machine keeps the SDK, and keys that are not the tracked debug key
    # The build takes native code from node_modules, which no digest here covers. pnpm writes the lockfile it installed
    # from to node_modules/.pnpm/lock.yaml; that file is pnpm's own and not a documented interface. An install of a part
    # of the workspace (--filter) writes a part of the lockfile and is refused here, and files edited inside
    # node_modules by hand are not seen.
    installed=(("pnpm-lock.yaml", "node_modules/.pnpm/lock.yaml"),))


class Unlisted(Exception):
    """The sources cannot be listed."""


def input_files(root, inputs):
    """Tracked and new files of the working tree that the build reads."""
    try:
        listed = subprocess.run(["git", "-C", str(root), "ls-files", "-z", "--cached", "--others", *(f"--exclude={pattern}" for pattern in inputs.written),
                                 "--", *inputs.take], capture_output=True, check=True)
    except (OSError, subprocess.CalledProcessError) as error:
        detail = getattr(error, "stderr", b"") or str(error).encode()
        raise Unlisted(f"git cannot list the app's sources in {root}: {detail.decode(errors='replace').strip()}") from error
    return sorted({name for name in listed.stdout.decode(errors="surrogateescape").split("\0") if name and inputs.has(name)})


def file_digest(path):
    digest = hashlib.sha256()
    with open(path, "rb") as file:
        while block := file.read(1 << 20):
            digest.update(block)
    return digest.hexdigest()


def tree_digest(directory):
    """One digest of all that lies below a directory. A link inside it counts by its text only, so a loop ends."""
    digest = hashlib.sha256()
    for place, directories, files in os.walk(directory):
        directories.sort()
        for name in sorted(files + [name for name in directories if os.path.islink(os.path.join(place, name))]):
            path = os.path.join(place, name)
            content = "link:" + os.readlink(path) if os.path.islink(path) else file_digest(path) if os.path.isfile(path) else "other"
            digest.update(f"{os.path.relpath(path, directory)}\0{content}\0".encode(errors="surrogateescape"))
    return digest.hexdigest()


def link_digest(path):
    """Where a link leads and what lies there: the build reads through it, also to a place outside its inputs."""
    content = file_digest(path) if path.is_file() else tree_digest(path) if path.is_dir() else "nothing"
    return f"link:{os.readlink(path)}={content}"


def fingerprint(root, inputs):
    """Each input's content digest by name. A file deleted from the working tree is left out."""
    found = {}
    for name in input_files(root, inputs):
        path = Path(root) / name
        if path.is_symlink():
            found[name] = link_digest(path)
        elif path.is_file():
            found[name] = file_digest(path)
    return found


def uninstalled(root, inputs):
    """Why node_modules does not hold what the lockfile names, or None."""
    for lock, copy in inputs.installed:
        try:
            same = (Path(root) / lock).read_bytes() == (Path(root) / copy).read_bytes()
        except OSError:
            same = False
        if not same:
            return f"node_modules does not hold what {lock} names ({copy} is missing or differs): run `pnpm install` first"
    return None


def record_path(apk):
    return apk.with_name(apk.name + ".inputs.json")


def write_record(apk, files):
    """Says which sources this very APK was built from. It lies in the build directory, so a clean removes both."""
    record = record_path(apk)
    draft = record.with_name(record.name + ".new")
    draft.write_text(json.dumps({"version": 1, "apk": file_digest(apk), "files": files}, indent=1, sort_keys=True) + "\n")
    os.replace(draft, record)


def build(apk, root, inputs, command, directory):
    """Build the APK and record its inputs. No record remains when the build fails or the sources move under it."""
    try:
        before = fingerprint(root, inputs)
    except Unlisted as error:
        raise SystemExit(str(error)) from error
    if reason := uninstalled(root, inputs):
        raise SystemExit(reason)
    record_path(apk).unlink(missing_ok=True)  # also leaves the output directory as the last build left it
    try:
        subprocess.run(command, cwd=directory, check=True)
    except (OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"the debug app did not build: {error}") from error
    if not apk.is_file():
        raise SystemExit(f"the build left no debug app at {apk}")
    if fingerprint(root, inputs) != before:
        raise SystemExit("the app's sources changed while it was building: build again")
    write_record(apk, before)


def stale(apk, root, inputs, installed=None):
    """Why the app cannot stand for the current sources, or None when it can.

    `installed` gives the digest of the APK on the emulator, or None when that cannot be read. Without it only the
    APK in the build directory is judged.
    """
    if not apk.is_file():
        return f"there is no debug app at {apk}, so the installed one cannot be traced to these sources"
    try:
        record = json.loads(record_path(apk).read_text())
        built, files = record["apk"], record["files"]
        if record["version"] != 1 or not isinstance(files, dict):
            raise ValueError(record["version"])
    except FileNotFoundError:
        return f"nothing records which sources {apk} was built from ({record_path(apk).name} is missing), so it may be an old build"
    except (OSError, ValueError, KeyError, TypeError):
        return f"{record_path(apk)} cannot be read, so {apk.name} may be an old build"
    if file_digest(apk) != built:
        return f"{apk} was built again without its record, so its sources are not known"
    try:
        now = fingerprint(root, inputs)
    except Unlisted as error:
        return str(error)
    changed = sorted(name for name in files.keys() | now.keys() if files.get(name) != now.get(name))
    if changed:
        more = f" and {len(changed) - 5} more" if len(changed) > 5 else ""
        return f"the debug app is older than its sources: {len(changed)} changed since it was built ({', '.join(changed[:5])}{more})"
    if reason := uninstalled(root, inputs):  # the bundler serves the screens' dependencies from node_modules
        return reason
    if installed is not None:
        on_device = installed()
        if on_device is None:
            return "the app installed on the emulator cannot be read, so it cannot be compared with the build"
        if on_device != built:
            return f"the app installed on the emulator is not the build at {apk}"
    return None


def installed_digest(adb, package):
    """SHA-256 of the package's APK on the emulator, or None. `adb` runs one adb command and returns its result."""
    paths = [line[len("package:"):].strip() for line in adb("shell", "pm", "path", package).stdout.decode(errors="replace").splitlines()
             if line.startswith("package:")]
    if len(paths) != 1:  # none, or split APKs, which no build of this repository installs
        return None
    summed = adb("shell", "sha256sum", paths[0])
    if not summed.returncode and (found := re.match(rb"([0-9a-f]{64})\s", summed.stdout)):
        return found[1].decode()
    copied = adb("exec-out", "cat", paths[0])  # an image without sha256sum
    return None if copied.returncode or not copied.stdout else hashlib.sha256(copied.stdout).hexdigest()


def undeclared(granted):
    """Whether `pm grant` failed because the installed app's manifest lacks the permission."""
    return bool(granted.returncode) and b"has not requested permission" in granted.stdout + granted.stderr


def admit(reason, allow, remedy, out=None):
    """Stop a run on a stale app, or let it through loudly. Returns the reason a let-through run must carry, else ""."""
    if not reason:
        return ""
    if not allow:
        raise SystemExit(f"{reason}\n{remedy}")
    line = "!" * 100
    print(f"\n{line}\nSTALE APP ALLOWED: {reason}\nThis run does not show that the current sources work.\n{line}\n", file=out or sys.stderr, flush=True)
    return reason
