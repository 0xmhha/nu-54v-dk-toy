"""Own source mirrors and working caches for Android signing acceptance."""

import fcntl
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from integration_workspace import _ignore_interruptions, run_command, scoped_workspace

ROOT = Path(__file__).resolve().parents[2]
PRODUCTS = {"p02": "p02-user-app", "p04": "p04-merchant-kiosk"}
RECEIPT_FILES = ("custom-gradle.lockfile", "release-gradle.lockfile", "apk-signature.json", "aab-signature.json")


def source_ignores(directory, names):
    location = Path(directory)
    native_project = location.name == "android" or location.parent.name == "android"
    gradle_plugin = "@react-native/gradle-plugin" in location.as_posix()
    ignored = {name for name in names if name in {
        ".git", ".gradle", ".cxx", ".kotlin", ".cache", "__pycache__", "Pods",
        ".env", "local.properties",
    } or name.startswith(".env.") or name.endswith((".jks", ".p12", ".keystore", ".apk", ".aab"))}
    if native_project or gradle_plugin:
        ignored.add("build")
    if "node_modules" not in location.parts:
        ignored.add("out")
    return ignored


def mirror_product(root, project, mirror):
    mirror.mkdir()
    root = root.resolve()

    def source_path(relative):
        path = root / relative
        if not path.resolve().is_relative_to(root):
            raise ValueError(f"Android test source symlink escapes its workspace: {relative}")
        return path

    for source in ("package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", ".npmrc"):
        path = source_path(source)
        if path.is_file():
            shutil.copy2(path, mirror / source)
    for source in ("node_modules", "packages", "scripts", "docs/content/specifications", "docs/content/commercial/legal",
                   "products/p01-device-firmware", f"products/{project}"):
        shutil.copytree(source_path(source), mirror / source, symlinks=True, ignore=source_ignores)
    mirror_root = mirror.resolve()
    for directory, folders, files in os.walk(mirror):
        for name in folders + files:
            path = Path(directory) / name
            if path.is_symlink() and not path.resolve().is_relative_to(mirror_root):
                raise ValueError(f"Android test source symlink escapes its workspace: {path.relative_to(mirror)}")
    # The debug test fixture is the only source keystore this test uses.
    debug = Path("products") / project / "android/app/debug.keystore"
    shutil.copy2(source_path(debug), mirror / debug)


def seed_wrapper(android, borrowed, owned):
    properties = (android / "gradle/wrapper/gradle-wrapper.properties").read_text()
    name = re.search(r"^distributionUrl=.*[/=](gradle-[^/]+)\.zip$", properties, re.MULTILINE)
    if not name:
        raise ValueError("Expected pinned local Gradle wrapper distribution")
    relative = Path("wrapper/dists") / name[1]
    existing = borrowed / relative
    if existing.is_dir():
        shutil.copytree(existing, owned / relative,
                        ignore=shutil.ignore_patterns("*.lck", "*.part"))


def isolated_environment(work, android, environment):
    env = {key: value for key, value in environment.items() if key.lower() != "npm_config_cache"}
    home = Path(env.get("GRADLE_USER_HOME", Path.home() / ".gradle")).resolve()
    gradle = work / "gradle"
    gradle.mkdir()
    temporary = work / "tmp"
    temporary.mkdir()
    seed_wrapper(android, home, gradle)
    # Existing dependency downloads remain inputs. The new Gradle home owns
    # transformed artifacts, daemon registries, wrapper writes and build cache.
    if (home / "caches/modules-2").is_dir():
        env.setdefault("GRADLE_RO_DEP_CACHE", str(home / "caches"))
    gradle_properties = home / "gradle.properties"
    if gradle_properties.is_file():
        shutil.copy2(gradle_properties, gradle / gradle_properties.name)
    for key in ("BUNDLETOOL_JAR", "GRADLE_RO_DEP_CACHE"):
        if env.get(key):
            env[key] = str(Path(env[key]).resolve())
    env.update(
        GRADLE_USER_HOME=str(gradle), ANDROID_USER_HOME=str(work / "android-user"),
        NU54_ANDROID_EVIDENCE_DIR=str(work / "evidence"), TMPDIR=str(temporary),
        TMP=str(temporary), TEMP=str(temporary), PYTHONDONTWRITEBYTECODE="1",
    )
    (work / "android-user").mkdir()
    env["npm_config_cache"] = str(work / "npm-cache")
    env["JAVA_TOOL_OPTIONS"] = f'{env.get("JAVA_TOOL_OPTIONS", "")} -Djava.io.tmpdir="{temporary}"'.strip()
    return env


def publish_evidence(path, contents):
    path.mkdir(parents=True, exist_ok=True)
    lock = os.open(path, os.O_RDONLY)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX)
        with _ignore_interruptions(), tempfile.TemporaryDirectory(prefix=".android-evidence-", dir=path) as storage:
            stage = Path(storage)
            backups = stage / "previous"
            backups.mkdir()
            for name, data in contents.items():
                (stage / name).write_bytes(data)
                original = path / name
                if original.is_symlink() or original.exists():
                    os.link(original, backups / name, follow_symlinks=False)
            replaced = []
            try:
                for name in contents:
                    (stage / name).replace(path / name)
                    replaced.append(name)
            except OSError:
                for name in replaced:
                    if (backups / name).exists() or (backups / name).is_symlink():
                        (backups / name).replace(path / name)
                    else:
                        (path / name).unlink()
                raise
    finally:
        os.close(lock)
    print(f"Published Android release evidence in {path}", flush=True)


def run(product, root, environment):
    project = PRODUCTS[product]
    target = environment.get("NU54_ANDROID_EVIDENCE_DIR")
    evidence = Path(target).resolve() if target else None
    with scoped_workspace(f"nu54-android-release-{product}-") as work:
        mirror = work / "source"
        mirror_product(root, project, mirror)
        android = mirror / "products" / project / "android"
        env = isolated_environment(work, android, environment)
        run_command(["bash", mirror / "scripts/android/test_release_checks.sh", product],
                    cwd=mirror, env=env, check=True)
        if evidence:
            contents = {name: (work / "evidence" / name).read_bytes() for name in RECEIPT_FILES}
    if evidence:
        publish_evidence(evidence, contents)


def main():
    if len(sys.argv) != 2 or sys.argv[1] not in PRODUCTS:
        print("Usage: bash scripts/android/test_release.sh p02|p04", file=sys.stderr)
        raise SystemExit(2)
    hangup = False

    def on_hangup(_number, _frame):
        nonlocal hangup
        hangup = True
        signal.raise_signal(signal.SIGTERM)

    previous = signal.signal(signal.SIGHUP, on_hangup)
    try:
        run(sys.argv[1], ROOT, os.environ)
    except SystemExit as error:
        if hangup and error.code == 143:
            raise SystemExit(129) from None
        raise
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode if error.returncode >= 0 else 128 - error.returncode) from None
    except (OSError, ValueError) as error:
        raise SystemExit(str(error)) from None
    finally:
        signal.signal(signal.SIGHUP, previous)


if __name__ == "__main__":
    main()
