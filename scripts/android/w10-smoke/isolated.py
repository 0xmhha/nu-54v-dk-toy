#!/usr/bin/env python3
"""Build, install and exercise the W10 sponsored review with an isolated emulator and PostgreSQL."""
import contextlib
import hashlib
import json
import os
import re
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import sys
import time

sys.dont_write_bytecode = True
root = Path(__file__).resolve().parents[3]
sys.path[:0] = [str(root/"scripts"), str(root/"scripts/android")]
from integration_workspace import integration_workspace, run_command, start_postgres
from test_release_workspace import mirror_product, source_ignores, isolated_environment
from test_commercial_integrations import secure_env


def main(receipt, mode="owner"):
    renter = mode != "owner"
    sdk = Path(os.environ.get("ANDROID_HOME", Path.home()/"Library/Android/sdk"))
    java = Path(os.environ["JAVA_HOME"])
    pg = Path(os.environ["POSTGRES_BIN"])
    adb = sdk/"platform-tools/adb"
    if mode == "renter":
        inputs = json.loads((root/".github/inputs.json").read_text())
        expected = inputs["versions"]["foundry"]
        version = subprocess.check_output(["forge","--version"], text=True)
        pinned = re.search(r"forge Version: (\d+\.\d+\.\d+)", version)
        if not pinned or pinned.group(1) != expected:
            raise RuntimeError(f"W10 renter installation requires Foundry {expected}")
    with integration_workspace("nu54-w10-android-", pg/"pg_ctl", "database", secure_env()) as (work, env):
        work = work.resolve()
        def run(args, *, cwd=root, active=env, **opts):
            return run_command([str(x) for x in args],cwd=cwd,env=active,check=True,**opts)

        def capture(stage, args, *, cwd=root, active=env):
            path = work/(stage+".log")
            with path.open("w") as stream:
                try:
                    run(args,cwd=cwd,active=active,stdout=stream,stderr=stream)
                except subprocess.CalledProcessError:
                    print("".join(path.read_text(errors="replace").splitlines(keepends=True)[-45:]),flush=True)
                    raise
            print(f"PASS W10 Android {stage}",flush=True)

        def run_database():
            pg_env = env | {"PATH":str(pg)+os.pathsep+env["PATH"], "PGHOST":str(work),
                            "PGPORT":"5432", "PGUSER":"nu54_w10"}
            capture("initdb",[pg/"initdb","-D",work/"database","-U","nu54_w10","-A","trust",
                              "--no-locale","--encoding=UTF8"],active=pg_env)
            start_postgres(pg/"pg_ctl",work/"database",work/"postgres.log",f"-h '' -k {work} -p 5432",pg_env,
                           stdout=subprocess.DEVNULL)
            name = "nu54_settlement_w10_android" if renter else "nu54_commerce_w10_android"
            run([pg/"createdb",name],active=pg_env)
            capture("migration",["python3","-B","products/p10-platform/database/migrate.py","p05"],
                    active=pg_env | {"P05_MIGRATION_DATABASE_URL":name})
            if renter:
                capture("chain evidence migration",["python3","-B","products/p10-platform/database/migrate.py","p07"],
                        active=pg_env | {"P07_MIGRATION_DATABASE_URL":name})

            return name

        source = work/"source"
        mirror_product(root,"p02-user-app",source)
        shutil.copytree(root/"products/p04-merchant-kiosk",source/"products/p04-merchant-kiosk",
                        symlinks=True,ignore=source_ignores)
        android = source/"products/p02-user-app/android"
        android_work = work/"android"
        android_work.mkdir()
        gradle = isolated_environment(android_work,android,env)
        metro = work/"metro.cjs"
        fixture = source/"scripts/android/w10-smoke"
        nearby_adapter = fixture/("w10-renter-nearby.ts" if renter else "w10-owner-nearby.ts")
        metro.write_text(f"""const path=require('node:path');
const config=require({json.dumps(str(android.parent/"metro.config.js"))});
const original=config.resolver.resolveRequest;
config.cacheVersion += ':w10-qa';
config.resolver.resolveRequest=(context,name,platform)=>{{
  if (path.basename(context.originModulePath) === 'CustomerOrders.tsx') {{
    if (name === '../specs/NativeNearby.ts')
      return context.resolveRequest(context,{json.dumps(str(nearby_adapter))},platform);
    if ({str(not renter).lower()} && name === '../specs/NativeRenterBle.ts')
      return context.resolveRequest(context,{json.dumps(str(fixture/"w10-owner-ble.ts"))},platform);
  }}
  return original(context,name,platform);
}};
module.exports=config;
""")
        init = work/"isolated.gradle"
        init.write_text("""gradle.beforeProject { project ->
    if (project.path != ':app' || project.rootProject.name != 'NU54Renter') return
    project.plugins.withId('com.android.application') {
        project.androidComponents.finalizeDsl { app ->
            app.buildTypes.getByName('custom').applicationIdSuffix = '.w10qa'
            // The custom build type pins cleartext off, overriding defaultConfig; the QA control channel is loopback HTTP.
            app.buildTypes.getByName('custom').manifestPlaceholders.put('usesCleartextTraffic', 'true')
        }
    }
    project.plugins.withId('com.facebook.react') {
        project.extensions.getByName('react').bundleConfig.set(project.file(System.getenv('NU54_W10_METRO')))
    }
}
""")
        secret = os.urandom(24).hex()
        signing = {"NU54_P02_CUSTOM_KEYSTORE":str(work/"signing.p12"),"NU54_P02_CUSTOM_KEY_ALIAS":"w10qa",
                   "NU54_P02_CUSTOM_STORE_PASSWORD":secret,"NU54_P02_CUSTOM_KEY_PASSWORD":secret}
        gradle |= signing | {"NU54_W10_METRO":str(metro), "JAVA_HOME":str(java),
                             "ANDROID_HOME":str(sdk), "ANDROID_SDK_ROOT":str(sdk)}
        capture("signing",[java/"bin/keytool","-genkeypair","-alias","w10qa","-keyalg","RSA","-keysize","2048",
            "-validity","2","-dname","CN=Disposable W10 Installation","-keystore",work/"signing.p12",
            "-storepass:env","NU54_P02_CUSTOM_STORE_PASSWORD","-keypass:env","NU54_P02_CUSTOM_KEY_PASSWORD"],
            active=gradle)
        capture("APK",["./gradlew","--offline","--no-daemon","-I",init,"-PreactNativeArchitectures=arm64-v8a",
                       ":app:assembleCustom"],cwd=android,active=gradle)
        apk = android/"app/build/outputs/apk/custom/app-custom.apk"
        app_hash = hashlib.sha256(apk.read_bytes()).hexdigest()
        tools = sdk/"build-tools/36.0.0"
        capture("signature",[tools/"apksigner","verify","--verbose",apk])
        if mode == "prepare-renter":
            # Optional copy for a physical-device check; the run workspace itself is removed.
            if keep := os.environ.get("NU54_W10_KEEP_APK"):
                shutil.copyfile(apk, keep)
            return app_hash
        if release := os.environ.get("NU54_W10_DATABASE_RELEASE"):
            gate = Path(release)
            if not gate.is_absolute():
                raise ValueError("database release marker must be absolute")
            print("W10 Android waiting for a local PostgreSQL slot", flush=True)
            deadline = time.monotonic() + 360
            while not gate.is_file():
                if time.monotonic() >= deadline:
                    raise RuntimeError("local PostgreSQL slot was not released")
                time.sleep(1)
        name = run_database()
        clipboard = work/"clipboard.apk"
        capture("clipboard",["bash",root/"scripts/android/subscription-smoke/build-clipboard.sh",tools,clipboard],
                active=env | {"ANDROID_HOME":str(sdk),"JAVA_HOME":str(java)})
        board = work/"board"
        capture("C host",["cmake","-S",root/"products/p01-device-firmware/test","-B",board])
        capture("C verifier",["cmake","--build",board,"--target","owner_registration_host","test_install"])
        renter_peer = work/"renter-board"
        if renter:
            capture("C renter peer",["sh",root/"packages/protocol/v2/testdata/test-transport.sh"],
                    active=env | {"NU54_MOBILE_TEST_PEER_OUTPUT":str(renter_peer)})

        # The empty AVD uses the installed read-only system image and a private data disk.
        avd_root = work/"avd"
        avd_dir = avd_root/"w10.avd"
        avd_dir.mkdir(parents=True)
        (avd_root/"w10.ini").write_text(f"avd.ini.encoding=UTF-8\npath={avd_dir}\ntarget=android-34\n")
        (avd_dir/"config.ini").write_text("""AvdId=w10
avd.ini.displayname=w10
avd.ini.encoding=UTF-8
abi.type=arm64-v8a
hw.cpu.arch=arm64
hw.cpu.ncore=4
hw.device.name=pixel_3a
hw.lcd.width=1080
hw.lcd.height=2220
hw.lcd.density=440
hw.ramSize=2048
disk.dataPartition.size=3G
image.sysdir.1=system-images/android-34/google_apis_playstore/arm64-v8a/
tag.id=google_apis_playstore
PlayStore.enabled=true
""")
        with socket.socket() as socket_probe:
            socket_probe.bind(("127.0.0.1",0))
            adb_port = socket_probe.getsockname()[1]
        emulated = env | {"ANDROID_AVD_HOME":str(avd_root),"ANDROID_USER_HOME":gradle["ANDROID_USER_HOME"],
                          "ANDROID_ADB_SERVER_PORT":str(adb_port),"ADB_SERVER_SOCKET":f"tcp:127.0.0.1:{adb_port}",
                          "ANDROID_HOME":str(sdk)}
        serial, package = "emulator-5580", "com.nu54renter.w10qa"
        emulator = daemon = None
        with (work/"emulator.log").open("w") as output, (work/"adb.log").open("w") as adb_log:
            try:
                server_env = {key:value for key,value in emulated.items() if key != "ADB_SERVER_SOCKET"}
                daemon = subprocess.Popen([adb,"-P",str(adb_port),"nodaemon","server"],
                    env=server_env,stdout=adb_log,stderr=adb_log,start_new_session=True)
                end = time.monotonic()+10
                while time.monotonic()<end:
                    if daemon.poll() is not None:
                        raise RuntimeError("private ADB server stopped during startup")
                    try:
                        with socket.create_connection(("127.0.0.1",adb_port),timeout=0.2):
                            break
                    except OSError:
                        time.sleep(0.15)
                else:
                    raise RuntimeError("private ADB did not listen")
                emulator = subprocess.Popen([sdk/"emulator/emulator","-avd","w10","-port","5580","-no-window",
                    "-gpu",os.environ.get("NU54_W10_EMULATOR_GPU","swiftshader_indirect"),"-no-snapshot","-no-audio","-no-boot-anim"],env=emulated,
                    stdout=output,stderr=output,start_new_session=True)
                deadline = time.monotonic()+180
                while time.monotonic()<deadline:
                    if emulator.poll() is not None:
                        raise RuntimeError("isolated emulator stopped during startup")
                    boot = subprocess.run([adb,"-s",serial,"shell","getprop","sys.boot_completed"],
                                          env=emulated,capture_output=True,text=True,timeout=5)
                    if boot.stdout.strip()=="1":
                        break
                    time.sleep(2)
                else:
                    raise RuntimeError("isolated emulator did not boot")
                run([adb,"-s",serial,"shell","input","keyevent","82"],active=emulated)
                run([adb,"-s",serial,"install",apk],active=emulated)
                run([adb,"-s",serial,"install",clipboard],active=emulated)
                for permission in ("BLUETOOTH_SCAN","BLUETOOTH_CONNECT"):
                    run([adb,"-s",serial,"shell","pm","grant",package,"android.permission."+permission],active=emulated)
                native = emulated | {"PATH":str(pg)+os.pathsep+emulated["PATH"], "PGHOST":str(work),
                                      "PGPORT":"5432","PGUSER":"nu54_w10"}
                if renter:
                    dsn = f"host={work} port=5432 user=nu54_w10 dbname={name} sslmode=disable"
                    capture("installed review",["sh",root/"products/p05-operations-backoffice/test/settlement/anvil.sh",
                        "-v","-run","^TestLoopbackInstalledW10Fixture$"],active=native | {
                            "P05_SETTLEMENT_TEST_DATABASE_URL":dsn,
                            "NU54_K10_INSTALLED_RUNNER":str(root/"scripts/android/w10-smoke/run.py"),
                            "NU54_W10_RENTER_INSTALLED":"1",
                            "NU54_W10_RENTER_BOARD_BRIDGE":str(root/"scripts/android/w10-smoke/renter-board.mts"),
                            "NU54_W10_RENTER_BOARD_BIN":str(renter_peer),
                            "NU54_P01_INSTALL_TEST":str(board/"test_install"),
                            "NU54_W10_ANDROID_SERIAL":serial,"NU54_W10_ANDROID_PACKAGE":package,"NU54_W10_ADB":str(adb),
                        } | ({"NU54_W10_DIAGNOSTICS":os.environ["NU54_W10_DIAGNOSTICS"]}
                             if os.environ.get("NU54_W10_DIAGNOSTICS") else {}))
                    log = (work/"installed review.log").read_text()
                    expected = ("PASS Android W10 renter QR SW1", "PASS Android W10 renter W09 SW2",
                                "installed W10 renter decisions: one SW1 admission")
                    markers = [row.strip() for row in log.splitlines() if any(marker in row for marker in expected)]
                    assert all(sum(marker in row for row in markers) == 1 for marker in expected)
                else:
                    capture("installed review",["go","test","-count=1","-v","-run",
                        "^TestPostgresOperatorCatalogRealTLS$/^InstalledK10VirtualNearby$",
                        "./products/p05-operations-backoffice/internal/commerce"],active=native | {
                            "P05_COMMERCE_TEST_DATABASE_URL":f"host={work} port=5432 user=nu54_w10 dbname={name} sslmode=disable",
                            "NU54_K10_INSTALLED_RUNNER":str(root/"scripts/android/w10-smoke/run.py"),
                            "NU54_K10_SIGNED_REVIEW":"1","NU54_W10_INSTALLED_BOARD":"1",
                            "NU54_W10_BOARD_HOST":str(board/"owner_registration_host"),
                            "NU54_W10_BOARD_VALIDATOR":str(board/"test_install"),
                            "NU54_W10_ANDROID_SERIAL":serial,"NU54_W10_ANDROID_PACKAGE":package,"NU54_W10_ADB":str(adb),
                        })
                    log = (work/"installed review.log").read_text()
                    markers = [row.strip() for row in log.splitlines() if any(marker in row for marker in (
                        "PASS Android sponsored original review","installed W10 SW2:", "issued commands=1",
                        "installed W10 ledger proof:", "--- PASS: TestPostgresOperatorCatalogRealTLS/InstalledK10VirtualNearby"))]
                    assert len(markers)==4, "\n".join(markers)
            finally:
                if emulator:
                    with contextlib.suppress(ProcessLookupError):
                        os.killpg(emulator.pid,signal.SIGTERM)
                    try:
                        emulator.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        with contextlib.suppress(ProcessLookupError):
                            os.killpg(emulator.pid,signal.SIGKILL)
                        emulator.wait()
                if daemon:
                    with contextlib.suppress(ProcessLookupError):
                        os.killpg(daemon.pid,signal.SIGTERM)
                    try:
                        daemon.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        with contextlib.suppress(ProcessLookupError):
                            os.killpg(daemon.pid,signal.SIGKILL)
                        daemon.wait()
    assert not work.exists()
    with receipt.open("x") as output:
        json.dump({"apkSha256":app_hash,"markers":markers,"ownedScratchRemoved":True},output,indent=2)
        output.write("\n")


if __name__=="__main__":
    if len(sys.argv) == 3 and sys.argv[1] == "--prepare-renter":
        receipt = Path(sys.argv[2])
        app_hash = main(receipt, mode="prepare-renter")
        with receipt.open("x") as output:
            json.dump({"apkSha256":app_hash,"renterOverlay":True,"ownedScratchRemoved":True},output,indent=2)
            output.write("\n")
    elif len(sys.argv) == 3 and sys.argv[1] == "--renter":
        main(Path(sys.argv[2]), mode="renter")
    elif len(sys.argv) == 2:
        main(Path(sys.argv[1]))
    else:
        raise SystemExit("usage: isolated.py [--prepare-renter | --renter] /absolute/new-receipt.json")
