#!/usr/bin/env python3
"""Emit a deterministic per-artifact source SBOM from the pinned v1 revision."""

import json
import posixpath
import re
import subprocess

from verify import HERE, git, load, require

FIRMWARE = "products/p01-device-firmware"
CONTRACTS = "products/p06-stablenet-contracts"
NPM_SECTIONS = {"dependencies": "runtime", "devDependencies": "development", "optionalDependencies": "runtime"}


def component(name, version, ecosystem, relation, scope, **evidence):
    return dict(name=name, version=version, ecosystem=ecosystem, relation=relation, scope=scope, **evidence)


def npm_graph(lock, importer, source):
    components = {}
    visited = set()

    def walk(name, version, scope, relation, parent_importer):
        if version.startswith("link:"):
            linked_from = posixpath.normpath(posixpath.join(parent_importer, version.removeprefix("link:")))
            package = json.loads(source(f"{linked_from}/package.json"))
            key = (name, linked_from, scope)
            if key not in components or relation == "direct":
                components[key] = component(name, package.get("version"), "workspace", relation, scope, path=linked_from)
            for section in ("dependencies", "optionalDependencies"):
                for dep, data in lock["importers"][linked_from].get(section, {}).items():
                    walk(dep, data["version"], scope, "transitive", linked_from)
            return

        split = version.split("(", 1)[0]
        # A resolved alias can carry the actual package name in the snapshot key.
        snapshot = version if "@" in split else f"{name}@{version}"
        package_key = snapshot.split("(", 1)[0]
        package_name, package_version = package_key.rsplit("@", 1)
        package = lock["packages"][package_key]
        item = component(package_name, package_version, "npm", relation, scope, snapshot=snapshot)
        integrity = package.get("resolution", {}).get("integrity")
        if integrity:
            item["checksum"] = integrity
        key = (package_name, snapshot, scope)
        if key not in components or relation == "direct":
            components[key] = item
        if key in visited:
            return
        visited.add(key)
        for section in ("dependencies", "optionalDependencies"):
            for dep, resolved in lock["snapshots"][snapshot].get(section, {}).items():
                walk(dep, resolved, scope, "transitive", parent_importer)

    for section, scope in NPM_SECTIONS.items():
        for name, data in lock["importers"][importer].get(section, {}).items():
            walk(name, data["version"], scope, "direct", importer)
    return [components[key] for key in sorted(components)]


def go_graph(source, directory):
    sums = {tuple(line.split()[:2]): line.split()[2] for line in source(f"{directory}/go.sum").splitlines()}
    entries = re.findall(
        r"^(?:require )?\s*([\w.\-/]+)\s+(v\d+\S*)(?:\s+(// indirect))?$",
        source(f"{directory}/go.mod"),
        flags=re.MULTILINE,
    )
    require(entries, f"No Go module requirements: {directory}")
    return [
        component(name, version, "golang", "transitive" if indirect else "direct", "module-requirement",
                  checksum=sums.get((name, version)))
        for name, version, indirect in sorted(entries)
    ]


def artifact(identifier, kind, sources, components, *gaps):
    return dict(id=identifier, kind=kind, sourceInputs=sources, artifactSha256=None,
                coverage="source inputs", components=components, pendingEvidence=gaps)


def generate():
    manifest = load(HERE / "v1.json")
    commit = manifest["source"]["commit"]

    def source(path):
        return git("show", f"{commit}:{path}").decode()

    # Psych is part of Ruby on the supported hosts. It reads the pinned lock with no package install.
    lock = json.loads(subprocess.check_output(
        ["ruby", "-rjson", "-ryaml", "-e", "puts JSON.generate(YAML.safe_load(STDIN.read))"],
        input=source("pnpm-lock.yaml"), text=True,
    ))
    apps = manifest["apps"]
    renter = npm_graph(lock, apps["P02"]["path"], source)
    kiosk = npm_graph(lock, apps["P04"]["path"], source)
    web = "products/p05-operations-backoffice/web"
    backoffice = "products/p05-operations-backoffice"
    indexer = "products/p07-indexer"
    pnpm = manifest["toolchain"]["sourceBuildInputs"]["pnpm"]
    node_tools = [component("pnpm", pnpm, "npm", "direct", "build-tool")]

    def android(app, modules):
        return [*modules, *node_tools, component("Gradle", app["androidGradle"], "gradle", "direct", "build-tool")]

    vendor_record = source(f"{FIRMWARE}/third_party/README.md")

    def vendor(name):
        row = next(line for line in vendor_record.splitlines() if line.startswith(f"| `{name}/`"))
        version = re.search(r"commit `([0-9a-f]{40})`", row).group(1)
        return component(name, version, "git", "vendor", "runtime", path=f"{FIRMWARE}/third_party/{name}")

    board = manifest["board"]["definitionPath"]
    board_source = source(f"{board}/VENDORED.md")
    board_commit = re.search(r"`([0-9a-f]{40})`", board_source).group(1)
    tools = manifest["toolchain"]["archivedBringup"]
    firmware = [
        vendor("secp256k1"), vendor("keccak"),
        component("NU-54V-DK board definition", board_commit, "git", "vendor", "build-input", path=board),
        component("nRF Connect SDK", tools["ncs"], "git", "direct", "sdk"),
        component("Zephyr", tools["zephyr"], "git", "transitive", "runtime"),
        component("MCUboot", None, "git", "transitive", "bootloader"),
        component("Trusted Firmware-M", None, "git", "transitive", "runtime"),
        component("CRACEN PSA driver", None, "git", "transitive", "runtime"),
        component("Zephyr SDK", tools["sdk"], "sdk", "direct", "build-tool"),
    ]
    solc = component("solc", manifest["toolchain"]["sourceBuildInputs"]["solc"], "toolchain", "direct", "build-tool")
    foundry = component("Foundry", manifest["toolchain"]["sourceBuildInputs"]["foundry"], "toolchain", "direct", "build-tool")
    forge_std = json.loads(source(f"{CONTRACTS}/lib/forge-std/package.json"))
    contracts_vendor = component("forge-std", forge_std["version"], "git", "vendor", "development", path=f"{CONTRACTS}/lib/forge-std")
    pg = component("PostgreSQL", None, "service", "direct", "runtime-service")

    def go_artifact(identifier, directory, role, image):
        inputs = ["go.work", "go.work.sum", f"{directory}/go.mod", f"{directory}/go.sum"]
        modules = go_graph(source, directory)
        pending = ["go.work selected and binary-reachable modules by target", "Go toolchain digest and binary SHA-256"]
        if image:
            inputs.append(f"{directory}/Dockerfile")
            modules += [
                component("golang", "1.25", "oci", "direct", "builder-image"),
                component("gcr.io/distroless/static-debian12", None, "oci", "direct", "base-image"),
            ]
            pending.append("OCI manifest/base digests and OS-package inventory")
        if identifier == "p07-indexer":
            modules.append(pg)
        return artifact(identifier, role, inputs, modules, *pending)

    artifacts = [
        artifact("p01-firmware", "release firmware and boot chain",
                 [f"{FIRMWARE}/app", f"{FIRMWARE}/third_party/README.md", f"{board}/VENDORED.md"], firmware,
                 "frozen west manifest and selected MCUboot, TF-M and Nordic crypto commits",
                 "linked firmware and bootloader inputs, hashes and toolchain bundle digest"),
        artifact("p02-android", "Android APK/AAB", ["pnpm-lock.yaml", f"{apps['P02']['path']}/android"],
                 android(apps["P02"], renter), "resolved Gradle/Maven, Hermes/native contents, signing and APK/AAB SHA-256"),
        artifact("p04-android", "Android APK/AAB", ["pnpm-lock.yaml", f"{apps['P04']['path']}/android"],
                 android(apps["P04"], kiosk), "resolved Gradle/Maven, Hermes/native contents, signing and APK/AAB SHA-256"),
        artifact("p04-ios", "iOS app archive", ["pnpm-lock.yaml", f"{apps['P04']['path']}/Gemfile", f"{apps['P04']['path']}/ios/Podfile"],
                 [*kiosk, *node_tools], "resolved Ruby/CocoaPods, Xcode, Hermes/native contents, signing and archive SHA-256"),
        go_artifact("p05-opsctl", backoffice, "operations CLI", False),
        go_artifact("p05-opsd", backoffice, "operations service image", True),
        artifact("p05-web", "operations web bundle", ["pnpm-lock.yaml", f"{web}/package.json"],
                 [*npm_graph(lock, web, source), *node_tools], "production bundle file manifest and SHA-256"),
        artifact("p06-contract-deployment", "deployed settlement, registry and test token",
                 [f"{CONTRACTS}/src", f"{CONTRACTS}/foundry.toml", manifest["deployment"]["record"]], [solc, foundry],
                 "matching compiler and build tool digests; see baseline deployment record for archived runtime fingerprints"),
        artifact("p06-contract-tests", "Foundry verification build",
                 [f"{CONTRACTS}/test", f"{CONTRACTS}/lib/forge-std/package.json", f"{CONTRACTS}/foundry.toml"],
                 [solc, foundry, contracts_vendor], "Foundry binary digest"),
        artifact("p06-abi", "ABI and generated bindings",
                 ["packages/contracts-abi/abi", "packages/contracts-abi/ts/package.json", "packages/contracts-abi/go/go.mod"],
                 [solc, *npm_graph(lock, "packages/contracts-abi/ts", source), *go_graph(source, "packages/contracts-abi/go")],
                 "published package SHA-256 and binary/module selections"),
        go_artifact("p07-indexer", indexer, "indexer service image", True),
    ]
    return dict(format="nu54-source-sbom-v1", baselineCommit=commit, artifacts=artifacts)


def patch_ledger():
    return load(HERE / "patch-ledger.json")


def check_ledger(ledger, artifacts):
    covered = {artifact for item in ledger["entries"] for artifact in item["artifacts"]}
    require(covered == artifacts, "patch ledger and SBOM artifact coverage differ")
    require(len({item["id"] for item in ledger["entries"]}) == len(ledger["entries"]), "duplicate patch IDs")
    for item in ledger["entries"]:
        if item["status"] == "closed":
            for key in ("resolvedVersion", "buildHash", "sbom", "rescan", "regression", "installedVersion"):
                require(item.get(key), f"{item['id']}: closed without {key}")
        else:
            require(item["status"] == "open", f"{item['id']}: unknown status")


if __name__ == "__main__":
    inventory = generate()
    check_ledger(patch_ledger(), {item["id"] for item in inventory["artifacts"]})
    print(json.dumps(inventory, indent=2))
