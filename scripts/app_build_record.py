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
from xml.etree import ElementTree


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
    """The sources cannot be listed or read."""


def unread(name, error):
    return f"{name} cannot be read ({error.strerror or error}), so the app cannot be compared with its sources"


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
        try:
            if path.is_symlink():
                found[name] = link_digest(path)
            elif path.is_file():
                found[name] = file_digest(path)
        except OSError as error:
            raise Unlisted(unread(name, error)) from error
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


def build_environment(environment=None):
    """The environment of a build, without the Gradle project properties it carries.

    Gradle hands every ORG_GRADLE_PROJECT_<name> to findProperty(), where app/build.gradle reads the lab runner, the
    wallet and market profiles and a directory of native libraries. They are taken away rather than recorded: a run
    judges an APK by its digest, and what the environment holds at that later time says nothing about how the APK
    was built. So a build made here is always the one the tracked files describe.
    Not covered: properties in the Gradle user home (~/.gradle/gradle.properties) and -Dorg.gradle.project.<name> in
    GRADLE_OPTS or JAVA_OPTS.
    """
    return {name: value for name, value in (os.environ if environment is None else environment).items() if not name.startswith("ORG_GRADLE_PROJECT_")}


def build(apk, root, inputs, command, directory):
    """Build the APK and record its inputs.

    A build that fails, or whose sources move under it, writes no record. The record of the build before it is put
    back: it names that build's APK by its digest, so it holds for as long as the failed build left that APK alone.
    """
    try:
        before = fingerprint(root, inputs)
    except Unlisted as error:
        raise SystemExit(str(error)) from error
    if reason := uninstalled(root, inputs):
        raise SystemExit(reason)
    record = record_path(apk)
    try:
        last = record.read_bytes()
    except OSError:
        last = None
    record.unlink(missing_ok=True)  # the build finds its output directory as the last build left it
    try:
        try:
            subprocess.run(command, cwd=directory, check=True, env=build_environment())
        except (OSError, subprocess.CalledProcessError) as error:
            raise SystemExit(f"the debug app did not build: {error}") from error
        if not apk.is_file():
            raise SystemExit(f"the build left no debug app at {apk}")
        try:
            if fingerprint(root, inputs) != before:
                raise SystemExit("the app's sources changed while it was building: build again")
            write_record(apk, before)
        except Unlisted as error:
            raise SystemExit(str(error)) from error
        except OSError as error:  # the APK cannot be read or the record cannot be written
            raise SystemExit(f"the build cannot be recorded: {error}") from error
    except BaseException:  # an interrupt too
        if last is not None and record.parent.is_dir():
            record.write_bytes(last)
        raise


def stale(apk, root, inputs, installed=None):
    """Why the app cannot stand for the current sources, or None when it can.

    `installed` gives the digest of the APK on the emulator, or None when that cannot be read. Without it only the
    APK in the build directory is judged. With it the installed app is judged: when it is the recorded build, the
    file in the build directory may have been built again since, as the lab scripts beside gradlew do.
    """
    gone = f"there is no debug app at {apk}, so the installed one cannot be traced to these sources"
    try:
        record = json.loads(record_path(apk).read_text())
        built, files = record["apk"], record["files"]
        if record["version"] != 1 or not isinstance(files, dict):
            raise ValueError(record["version"])
    except FileNotFoundError:
        if not apk.is_file():
            return gone
        return f"nothing records which sources {apk} was built from ({record_path(apk).name} is missing), so it may be an old build"
    except (OSError, ValueError, KeyError, TypeError):
        return f"{record_path(apk)} cannot be read, so {apk.name} may be an old build"
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
        if on_device == built:
            return None
    try:
        local = file_digest(apk) if apk.is_file() else None
    except OSError as error:
        return unread(apk, error)
    file = None if local == built else gone if local is None else f"{apk} was built again without its record, so its sources are not known"
    if installed is None:
        return file
    if file is None:
        return f"the app installed on the emulator is not the build at {apk}"
    return f"the app installed on the emulator is not the build that {record_path(apk).name} records, and {file}"


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


def lacks(granted, manifest, permission):
    """Why a refused `pm grant` shows that the installed app was not built from this manifest, or None.

    It shows that only when the manifest of the sources declares the permission for every Android version. A
    permission that the sources do not declare either, or only up to some version, says nothing about the build.
    """
    if not undeclared(granted):
        return None
    name = "{http://schemas.android.com/apk/res/android}"
    try:
        declared = any(element.get(name + "name") == permission and element.get(name + "maxSdkVersion") is None
                       for element in ElementTree.parse(manifest).getroot().iter() if element.tag in ("uses-permission", "uses-permission-sdk-23"))
    except (OSError, ElementTree.ParseError) as error:
        return f"{manifest} cannot be read ({error}), so the installed app, which does not declare {permission}, cannot be compared with it"
    if not declared:
        return None
    return f"the installed app does not declare {permission.rpartition('.')[2]}, which {manifest.name} does: it was built from another manifest"


def admit(reason, allow, remedy, out=None):
    """Stop a run on a stale app, or let it through loudly. Returns the reason a let-through run must carry, else ""."""
    if not reason:
        return ""
    if not allow:
        raise SystemExit(f"{reason}\n{remedy}")
    line = "!" * 100
    print(f"\n{line}\nSTALE APP ALLOWED: {reason}\nThis run does not show that the current sources work.\n{line}\n", file=out or sys.stderr, flush=True)
    return reason
