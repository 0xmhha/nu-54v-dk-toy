#!/usr/bin/env python3
"""Check pinned Git objects and archived v1 metadata without using live services."""

import argparse
import hashlib
import json
import subprocess
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
SOURCE_PATHS = {
    "products/p01-device-firmware",
    "products/p01-device-firmware/boards/nucode/nu54v_dk",
    "products/p02-user-app",
    "products/p04-merchant-kiosk",
    "products/p05-operations-backoffice",
    "products/p06-stablenet-contracts",
    "products/p07-indexer",
    "packages/contracts-abi",
    "packages/protocol",
    "packages/device-sim",
    "pnpm-lock.yaml",
    "package.json",
    "pnpm-workspace.yaml",
    "go.work",
    "go.work.sum",
    "uv.lock",
    "docs/content/acceptance/finality-8283.md",
    "docs/content/acceptance/gate-w7.md",
    "docs/content/nu54v-basic-peripheral-bringup-log.md",
}


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)


def load(path):
    return json.loads(path.read_bytes())


def require(condition, message):
    if not condition:
        raise ValueError(message)


def check_receipt(payment, root=ROOT):
    path = root / payment["receiptPath"]
    body = path.read_bytes()
    require(hashlib.sha256(body).hexdigest() == payment["receiptSha256"], "receipt SHA-256 mismatch")
    receipt = json.loads(body)
    require(
        int(receipt["blockNumber"], 16) == payment["block"] and receipt["blockHash"] == payment["blockHash"],
        "receipt block mismatch",
    )
    require(receipt["status"] == "0x1", "archived transaction did not succeed")


def check(manifest):
    require(manifest["version"] == "v1", "baseline version mismatch")
    source = manifest["source"]
    require(set(source["objects"]) == SOURCE_PATHS, "source coverage mismatch")
    require(set(manifest["apps"]) == {"P02", "P04"}, "app coverage mismatch")
    commit = source["commit"]

    def pinned(path):
        return git("show", f"{commit}:{path}")

    for path, oid in source["objects"].items():
        require(git("rev-parse", f"{commit}:{path}").decode().strip() == oid, f"Git object mismatch: {path}")

    board = manifest["board"]
    require(board["definitionPath"] in source["objects"], "board definition is not pinned")
    board_record = pinned(board["record"]).decode()
    for value in [board["target"], *board["revision"].split(" / "), *manifest["toolchain"]["archivedBringup"].values()]:
        require(value in board_record, f"board bring-up record does not contain {value}")

    for app in manifest["apps"].values():
        path = app["path"]
        require(path in source["objects"], f"app source is not pinned: {path}")
        package = json.loads(pinned(f"{path}/package.json"))
        require(package["version"] == app["packageVersion"], f"package version mismatch: {path}")
        require(package["dependencies"]["react-native"] == app["reactNative"], f"RN version mismatch: {path}")
        gradle = pinned(f"{path}/android/gradle/wrapper/gradle-wrapper.properties").decode()
        require(f"gradle-{app['androidGradle']}-bin.zip" in gradle, f"Gradle version mismatch: {path}")
        ios = git("ls-tree", "-r", "--name-only", commit, f"{path}/ios")
        require(bool(ios) == app["iOSSourceAvailable"], f"iOS source availability mismatch: {path}")

    deployment = manifest["deployment"]
    record = json.loads(pinned(deployment["record"]))
    require(record["chainId"] == deployment["chainId"], "deployment chain mismatch")
    require(record["sourceCommit"] == deployment["deployedSourceCommit"], "deployment source mismatch")
    require(record["sourceCommitOnMain"] == deployment["equivalentMainCommit"], "deployment main source mismatch")
    require(set(deployment["contracts"]) == set(record["contracts"]), "contract coverage mismatch")
    for name, contract in deployment["contracts"].items():
        require(set(contract) == {"address", "block", "runtimeCodeKeccak"}, f"contract field coverage mismatch: {name}")
        require(all(record["contracts"][name][key] == value for key, value in contract.items()), f"deployment mismatch: {name}")
    tools = manifest["toolchain"]["sourceBuildInputs"]
    require(f"go {tools['goModMinimum']}" in pinned("go.work").decode().splitlines(), "Go workspace version mismatch")
    directory = Path(deployment["record"]).parent.parent
    for path in ("src", "lib", "foundry.toml"):
        deployed_oid = git("rev-parse", f"{deployment['deployedSourceCommit']}:{directory / path}")
        main_oid = git("rev-parse", f"{deployment['equivalentMainCommit']}:{directory / path}")
        require(deployed_oid == main_oid, f"deployed and main revisions differ: {path}")
    require(all(tools[key] == value for key, value in record["compiler"].items()), "deployment compiler mismatch")
    require(json.loads(pinned("package.json"))["packageManager"] == f"pnpm@{tools['pnpm']}", "pnpm version mismatch")

    abi_dir = "packages/contracts-abi/abi"
    abi_manifest = json.loads(pinned(f"{abi_dir}/manifest.json"))
    deployed_hashes = {"TestUSDC": record["contracts"]["TestUSDC"]["abiSha256"]}
    for name in ("MerchantRegistry", "PaymentSettlement"):
        deployed_hashes.update(record["contracts"][name]["abiSha256"])
    require(manifest["deployment"]["abiSha256"] == deployed_hashes, "deployment ABI hash mismatch")
    for abi, digest in deployed_hashes.items():
        require(abi_manifest["abis"][abi]["sha256"] == digest, f"ABI manifest mismatch: {abi}")
        require(hashlib.sha256(pinned(f"{abi_dir}/{abi}.json")).hexdigest() == digest, f"ABI file mismatch: {abi}")

    payment = manifest["confirmedPayment"]
    gate = pinned(payment["record"]).decode()
    require(payment["finalizedHeadAtCheck"] >= payment["block"], "archived finality predates payment")
    require(f"블록 {payment['block']}(확인 시 finalized {payment['finalizedHeadAtCheck']})" in gate, "archived finality record mismatch")
    require(payment["firmwareBranchAnchor"] in gate and payment["firmwareVersion"] in gate, "archived firmware mismatch")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--require-receipt", action="store_true", help="require the git-ignored archived W7 receipt")
    args = parser.parse_args()
    try:
        manifest = load(HERE / "v1.json")
        check(manifest)
        print("PASS  v1 Git, board, app, deployment, ABI and archived finality pins", flush=True)
        if args.require_receipt or (ROOT / manifest["confirmedPayment"]["receiptPath"]).exists():
            check_receipt(manifest["confirmedPayment"])
            print("PASS  W7 archived receipt integrity and block")
        else:
            print("UNAVAILABLE  W7 private receipt archive")
    except (KeyError, OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"FAIL  {error}\n")


if __name__ == "__main__":
    main()
