#!/usr/bin/env python3
"""Create and verify an independent local mobile SDK candidate without rebuilding v0.2.0."""
import argparse
import base64
import hashlib
import json
import os
import re
import shutil
import subprocess
import tarfile
import tempfile
from pathlib import Path

from evidence import authenticate, deterministic_tar, digest, key_sha256, openssl, require, resolve

ROOT = Path(__file__).resolve().parents[3]
SOURCE = ROOT / "packages/protocol/platform"
LEGACY_FORMAT = "nu54-mobile-sdk-local-candidate-v1"
PEER_FORMAT = "nu54-mobile-sdk-local-candidate-v2"
CONSUMER_LOCK = Path(__file__).with_name("mobile-consumer-lock.json")
NPM_STRICT = ["--strict-peer-deps", "--legacy-peer-deps=false", "--force=false",
              "--ignore-scripts", "--no-audit", "--no-fund"]


def run(args, cwd=ROOT, capture=False):
    return subprocess.run(args, cwd=cwd, text=True, check=True,
                          stdout=subprocess.PIPE if capture else None).stdout


def write(path, value):
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")


def package_members():
    allowed = {"package.json", "LICENSE", "README.md", "Nu54ProtocolMobile.podspec"}
    paths = [SOURCE / item for item in allowed]
    paths += sorted((SOURCE / "android").rglob("*.kt"))
    paths += [SOURCE / "android/nu54-mobile.gradle"]
    paths += sorted((SOURCE / "ios").glob("*.h"))
    return {path.relative_to(SOURCE).as_posix(): path.read_bytes() for path in paths}


def external_android(consumer, sdk):
    env_home = os.environ.get("ANDROID_HOME")
    require(env_home and Path(env_home).is_dir(), "external Android build requires ANDROID_HOME")
    (consumer / "settings.gradle").write_text("""
pluginManagement { repositories { google(); mavenCentral(); gradlePluginPortal() } }
dependencyResolutionManagement { repositories { google(); mavenCentral() } }
rootProject.name = 'nu54-external'
""")
    (consumer / "build.gradle").write_text("""
buildscript {
    repositories { google(); mavenCentral() }
    dependencies {
        classpath 'com.android.tools.build:gradle:9.2.1'
        classpath 'org.jetbrains.kotlin:kotlin-gradle-plugin:2.2.0'
    }
}
apply plugin: 'com.android.library'
apply plugin: 'org.jetbrains.kotlin.android'
android {
    namespace 'org.nu54.external'
    compileSdk 37
    defaultConfig { minSdk 24 }
    compileOptions {
        sourceCompatibility JavaVersion.VERSION_17
        targetCompatibility JavaVersion.VERSION_17
    }
}
kotlin { jvmToolchain(17) }
def installed = providers.exec {
    commandLine 'node', '--print', "require.resolve('@nu54/protocol-mobile/package.json')"
}.standardOutput.asText.get().trim()
apply from: new File(new File(installed).parentFile, 'android/nu54-mobile.gradle')
""")
    (consumer / "gradle.properties").write_text("android.builtInKotlin=false\nandroid.newDsl=false\n")
    src = consumer / "src/main/java/org/nu54/external"
    src.mkdir(parents=True)
    (src / "Consumer.kt").write_text("""
package org.nu54.external
import android.content.Context
import com.nu54.protocol.NearbyBluetooth
import com.nu54.protocol.CommerceHttp
fun init(c: Context) = NearbyBluetooth(c, false) {} to CommerceHttp(c, setOf("customer"))
""")
    (consumer / "src/main/AndroidManifest.xml").write_text("<manifest />\n")
    gradle = ROOT / "products/p02-user-app/android/gradlew"
    log = run([str(gradle), "--offline", ":assembleDebug"], consumer, capture=True)
    aar = consumer / "build/outputs/aar/nu54-external-debug.aar"
    require(aar.is_file(), "external Android consumer AAR absent")
    import io
    import zipfile
    with zipfile.ZipFile(aar) as archive, zipfile.ZipFile(io.BytesIO(archive.read("classes.jar"))) as classes:
        required = ("NearbyBluetooth", "NearbyTls", "CommerceHttp")
        require(all(f"com/nu54/protocol/{name}.class" in classes.namelist() for name in required),
                "external Android AAR did not link installed SDK")
    return {"installed": str(sdk.relative_to(consumer)), "artifactSha256": digest(aar), "build": log}


def external_apple(consumer, sdk):
    headers = sdk / "ios"
    mobile = ROOT / "packages/protocol/platform/test"
    # Compile the shipped header for the target SDK from a separate consumer directory.
    source = consumer / "external-apple.mm"
    source.write_text("#define NU54_MOBILE_LAB 1\n#include <NearbyBle.h>\n")
    binary = consumer / "external-apple.o"
    run(["xcrun", "--sdk", "iphonesimulator", "clang++", "-target", "arm64-apple-ios15.1-simulator",
         "-fobjc-arc", "-std=c++17", "-I", str(headers), "-c", str(source), "-o", str(binary)], consumer)
    observation = run([os.sys.executable, str(mobile / "test_nearby_apple.py"), "--headers", str(headers)],
                      consumer, capture=True)
    return {"installed": str(sdk.relative_to(consumer)), "simulatorObjectSha256": digest(binary),
            "nativeRuntime": "macOS Network/Security TLS with installed iOS adapter sources",
            "checks": observation.strip()}


def runtime_peers(package):
    peers = package.get("peerDependencies", {})
    require(set(peers) == {"react", "react-native"} and not package.get("peerDependenciesMeta")
            and all(re.fullmatch(r"\d+\.\d+\.\d+", version) for version in peers.values()),
            "mobile runtime must declare exact required React and React Native peers")
    return peers


def peer_ref(name, version):
    return f"pkg:npm/{name}@{version}"


def verify_peer_inventory(package, sbom, observed):
    peers = runtime_peers(package)
    require(observed == peers, "installed mobile runtime differs from declared peers")
    component = sbom["metadata"]["component"]
    root = component["bom-ref"]
    require(component["name"] == package["name"] and component["version"] == package["version"]
            and root == f"pkg:npm/%40nu54/protocol-mobile@{package['version']}",
            "mobile SBOM metadata does not identify the archived SDK")
    edges = sbom["dependencies"]
    direct = [edge["dependsOn"] for edge in edges if edge["ref"] == root]
    require(len(direct) == 1, "missing or duplicate mobile root dependency edge")
    for name, version in peers.items():
        items = [item for item in sbom["components"] if item["type"] == "library" and item["name"] == name]
        ref = peer_ref(name, version)
        require(len(items) == 1 and items[0]["version"] == version and items[0]["bom-ref"] == ref
                and items[0]["purl"] == ref and direct[0].count(ref) == 1, f"missing or incorrect mobile SBOM peer {name}")


def install_external_consumer(consumer, archive, package, peers):
    sdk_name = package["name"]
    lock = json.loads(CONSUMER_LOCK.read_bytes())
    dependencies = {sdk_name: "file:candidate.tgz", **peers}
    require(lock["packages"][""]["dependencies"] == dependencies, "mobile consumer lock peer pins are stale")
    entry = lock["packages"][f"node_modules/{sdk_name}"]
    require(entry["version"] == package["version"] and entry["peerDependencies"] == peers and
            entry["resolved"] == dependencies[sdk_name], "mobile consumer lock SDK metadata is stale")
    shutil.copyfile(archive, consumer / "candidate.tgz")
    entry["integrity"] = "sha512-" + base64.b64encode(hashlib.sha512(archive.read_bytes()).digest()).decode()
    write(consumer / "package-lock.json", lock)
    write(consumer / "package.json", {"name": lock["name"], "version": lock["version"], "private": True,
                                     "dependencies": dependencies})
    run(["npm", "ci", *NPM_STRICT], consumer)
    run(["npm", "ls", "--all", "--json", "--strict-peer-deps"], consumer, capture=True)
    # Resolve from the app, installed SDK and installed React Native context to
    # detect duplicate React installations hidden by a superficially valid tree.
    resolver = """
const {createRequire} = require('node:module');
const {realpathSync} = require('node:fs');
const app = createRequire(process.cwd() + '/package.json');
const sdk = createRequire(app.resolve(process.argv[1] + '/package.json'));
const rn = createRequire(app.resolve('react-native/package.json'));
const observed = {};
for (const name of JSON.parse(process.argv[2])) {
  const file = app.resolve(name + '/package.json');
  if ([sdk, rn].some(context => realpathSync(context.resolve(name + '/package.json')) !== realpathSync(file))) {
    throw Error('duplicated mobile runtime peer ' + name);
  }
  observed[name] = app(file).version;
}
process.stdout.write(JSON.stringify(observed));
"""
    observed = json.loads(run(["node", "-e", resolver, sdk_name, json.dumps(sorted(peers))],
                              consumer, capture=True))
    require(observed == peers, "external app, SDK and React Native must share exact mobile peers")
    return observed


def reject_incompatible_peer(consumer, archive, package, peers):
    consumer.mkdir()
    test_dependencies = {package["name"]: archive.as_uri()}
    for name, version in peers.items():
        candidate = consumer / f"fixture-{name}.tgz"
        metadata = {"name": name, "version": "0.0.0" if name == "react" else version}
        deterministic_tar(candidate, {"package/package.json": json.dumps(metadata).encode()})
        test_dependencies[name] = candidate.as_uri()
    write(consumer / "package.json", {"name": "nu54-incompatible-peer", "version": "1.0.0", "private": True,
                                     "dependencies": test_dependencies})
    result = subprocess.run(["npm", "install", *NPM_STRICT, "--offline"], cwd=consumer,
                            text=True, capture_output=True)
    require(result.returncode != 0 and "ERESOLVE" in result.stderr and
            f'react@"{peers["react"]}"' in result.stderr,
            "incompatible React must be rejected by npm's strict peer resolver")


def verify(out, trusted_fingerprint):
    manifest = out / "mobile-candidate.json"
    evidence = json.loads(manifest.read_bytes())
    require(evidence["format"] in {LEGACY_FORMAT, PEER_FORMAT} and
            evidence["channel"] == "local-custom-test-key", "unexpected mobile review format")
    issuer = out / "mobile-candidate.pub.pem"
    signer = authenticate(manifest.read_bytes(), out / "mobile-candidate.manifest.sig", issuer.read_bytes())
    require(signer == trusted_fingerprint and evidence["signer"] == signer, "mobile candidate signer untrusted")
    artifact = resolve(out, evidence["archive"])
    sbom = json.loads(resolve(out, evidence["sbom"]).read_bytes())
    require(authenticate(artifact.read_bytes(), out / "mobile-candidate.sig", issuer.read_bytes()) == signer,
            "mobile payload signer differs from evidence signer")
    if evidence["format"] == PEER_FORMAT:
        with tarfile.open(artifact, "r:gz") as bundled:
            package = json.load(bundled.extractfile("package/package.json"))
        verify_peer_inventory(package, sbom, evidence["evidence"]["runtimePeers"])
    return evidence


def sign_local(out, archive, report):
    with tempfile.TemporaryDirectory(prefix="nu54-local-mobile-sign-") as temp:
        private = Path(temp) / "signature.key"
        openssl(["genpkey", "-algorithm", "Ed25519", "-out", private])
        public = out / "mobile-candidate.pub.pem"
        public.write_bytes(openssl(["pkey", "-in", private, "-pubout"]))
        seal = out / "mobile-candidate.sig"
        seal.write_bytes(openssl(["pkeyutl", "-sign", "-rawin", "-inkey", private, "-in", archive]))
        try:
            authenticate(archive.read_bytes() + b"x", seal, public.read_bytes())
        except ValueError:
            report["evidence"]["tamperedArchiveRejected"] = True
        else:
            raise ValueError("altered archive passed detached verification")
        report["signer"] = key_sha256(public)
        manifest = out / "mobile-candidate.json"
        write(manifest, report)
        signature = out / "mobile-candidate.manifest.sig"
        signature.write_bytes(openssl(["pkeyutl", "-sign", "-rawin", "-inkey", private, "-in", manifest]))
        try:
            authenticate(manifest.read_bytes() + b" ", signature, public.read_bytes())
        except ValueError:
            pass
        else:
            raise ValueError("altered candidate evidence passed verification")
        verify(out, report["signer"])


def build(out, compile_android, compile_apple):
    require(not out.is_relative_to(ROOT), "use a new directory outside the source checkout")
    out.mkdir(parents=True)
    contents = package_members()
    source_commit = run(["git", "rev-parse", "HEAD"], capture=True).strip()
    source_clean = not run(["git", "status", "--porcelain"], capture=True).strip()
    package = json.loads(contents["package.json"])
    version = package["version"]
    peers = runtime_peers(package)
    require(re.fullmatch(r"\d+\.\d+\.\d+", version) and version != "0.2.0", "mobile SDK version is independent")
    dependency_line = contents["android/nu54-mobile.gradle"].decode()
    conscrypt = re.search(r"org\.conscrypt:conscrypt-android:(\d+\.\d+\.\d+)", dependency_line)
    require(conscrypt is not None, "Conscrypt pin missing")
    archive = out / f"nu54-protocol-mobile-{version}.tgz"
    payload = {f"package/{name}": content for name, content in contents.items()}
    deterministic_tar(archive, payload)
    with tempfile.TemporaryDirectory(prefix="nu54-external-mobile-") as tmp:
        consumer = Path(tmp)
        duplicate = consumer / "again.tgz"
        deterministic_tar(duplicate, payload)
        require(digest(duplicate) == digest(archive), "archive is not reproducible")
        installed_peers = install_external_consumer(consumer, archive, package, peers)
        sdk = consumer / "node_modules/@nu54/protocol-mobile"
        require(not sdk.is_symlink(), "external consumer resolved a workspace source link")
        require(all((sdk / name).read_bytes() == expected for name, expected in contents.items()),
                "installed mobile source differs from archive")
        checks = {"archiveReproduced": True, "installedSourceMatches": True, "runtimePeers": installed_peers,
                  "consumerLockSha256": digest(CONSUMER_LOCK)}
        reject_incompatible_peer(consumer / "incompatible-peer", archive, package, peers)
        checks["incompatiblePeerRejected"] = True
        if compile_android:
            checks["android"] = external_android(consumer, sdk)
        if compile_apple:
            checks["apple"] = external_apple(consumer, sdk)
    sdk_ref = f"pkg:npm/%40nu54/protocol-mobile@{version}"
    metadata = {
        "bomFormat": "CycloneDX", "specVersion": "1.6", "version": 1,
        "metadata": {"component": {"type": "library", "name": package["name"], "version": version, "bom-ref": sdk_ref}},
        "components": [
            {"type": "library", "group": "org.conscrypt", "name": "conscrypt-android", "version": conscrypt[1]},
            *[{"type": "library", "name": name, "version": version,
               "bom-ref": peer_ref(name, version), "purl": peer_ref(name, version)}
              for name, version in sorted(installed_peers.items())],
            *[{"type": "framework", "name": name, "scope": "required"}
              for name in ("CoreBluetooth", "Network", "Security")],
            *[{"type": "file", "name": name, "hashes": [{"alg": "SHA-256", "content": hashlib.sha256(contents[name]).hexdigest()}]}
              for name in sorted(contents)],
        ],
        "dependencies": [{"ref": sdk_ref,
                          "dependsOn": [peer_ref(name, version) for name, version in sorted(installed_peers.items())]}],
    }
    verify_peer_inventory(package, metadata, installed_peers)
    sbom = out / "mobile-sbom.json"
    write(sbom, metadata)
    report = {
        "format": PEER_FORMAT, "version": version,
        "sourceCommit": source_commit,
        "sourceClean": source_clean,
        "source": {name: hashlib.sha256(contents[name]).hexdigest() for name in sorted(contents)},
        "recipeSha256": digest(Path(__file__)), "archive": {"file": archive.name, "sha256": digest(archive)},
        "sbom": {"file": sbom.name, "sha256": digest(sbom)},
        "evidence": checks, "channel": "local-custom-test-key",
    }
    sign_local(out, archive, report)
    print(out / "mobile-candidate.json")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--android", action="store_true", help="build the installed SDK in a fresh consumer")
    parser.add_argument("--apple", action="store_true", help="compile and exercise the installed Apple TLS adapter")
    parser.add_argument("--verify", help="externally pinned local signer fingerprint for verification")
    args = parser.parse_args()
    if args.verify:
        print(verify(args.out.resolve(), args.verify)["archive"]["sha256"])
    else:
        build(args.out.resolve(), args.android, args.apple)
