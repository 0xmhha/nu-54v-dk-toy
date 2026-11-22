"""Observe an enrolled local installation without assigning physical support."""

import base64
import hashlib
import json
import plistlib
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

from evidence import digest, key_sha256, require

ROOT = Path(__file__).resolve().parents[3]


def run(*cmd, cwd=None):
    process = subprocess.run(list(map(str, cmd)), cwd=cwd, capture_output=True, text=True, timeout=120)
    require(process.returncode == 0, f"{Path(cmd[0]).name} installation observation failed")
    return process.stdout.strip()


def sdk(file, row, target, folder):
    if row.get("interop",{}).get("language")=="python":
        from interop import python_adapter

        return python_adapter(file,row,target,folder)
    name = target["package"]
    require(re.fullmatch(r"(?:@[a-z0-9_-]+/)?[a-z0-9_-]+", name), "invalid npm package enrollment")
    (folder / "package.json").write_text('{"name":"nu54-install-observer","version":"1.0.0","private":true,"type":"module"}')
    cache = target.get("npmCache", folder / "npm-cache")
    run("npm", "install", "--cache", cache, "--offline", "--ignore-scripts", "--no-audit", "--no-fund", file, cwd=folder)
    installed = folder / "node_modules" / name
    info = json.loads((installed / "package.json").read_bytes())
    require(info["name"] == name and info["version"] == row["version"], "installed npm identity differs")
    match = json.loads((folder / "package-lock.json").read_bytes())["packages"]["node_modules/" + name]["integrity"]
    expected = "sha512-" + base64.b64encode(hashlib.sha512(file.read_bytes()).digest()).decode()
    require(expected == match, "installed npm archive integrity differs")
    with tarfile.open(file) as archive:
        for member in archive:
            if member.isfile():
                tail = Path(member.name).relative_to("package")
                require(".." not in tail.parts and hashlib.sha256(archive.extractfile(member).read()).hexdigest() ==
                        digest(installed / tail), "installed npm file differs")
    runtime = run("node", "--version")
    os_name = row.get("dimensions", {}).get("os", "")
    if os_name.startswith("node-"):
        require(os_name == "node-" + runtime.removeprefix("v"), "Node runtime differs from the signed matrix")
    run("node", "--input-type=module", "-e", f"await import({json.dumps(name)})", cwd=folder)
    details={"package":name,"integrity":match,"node":runtime}
    if "interop" in row:
        from interop import verify_typescript

        details["interop"]=verify_typescript(folder,row["interop"])
    return details


def android_certificate(file, apksigner):
    certs = run(apksigner,"verify","--print-certs",file)
    signers = set(re.findall(r"(?m)^(?:V[0-9.]+ )?Signer(?: #\d+)?:? certificate SHA-256 digest: ([a-fA-F0-9]{64})$", certs))
    require(len(signers)==1, "Android APK needs one observed signing certificate")
    return next(iter(signers)).lower()


def android_apk(file, aapt, apksigner):
    """Return the verified APK package, version, single signer and Android rollback policy."""
    certificate=android_certificate(file,apksigner)
    badging = run(aapt,"dump","badging",file).splitlines()[0]
    identity = re.match(r"package: name='([^']+)' versionCode='([0-9]+)' versionName='([^']+)'",badging)
    require(identity, "Android APK package identity absent")
    tree = run(aapt,"dump","xmltree",file,"AndroidManifest.xml")
    app = re.search(r"(?ms)^([ ]*)E: application\b[^\n]*\n(.*?)(?=^\1[ ]*E:|\Z)",tree)
    retain = bool(app and re.search(r"(?m)^ +A: android:rollbackDataPolicy\(0x[0-9a-f]+\)=\(type 0x10\)0x2$",app[2]))
    return {"package":identity[1],"versionCode":int(identity[2]),"versionName":identity[3],
            "certificate":certificate,"rollbackDataPolicy":"retain" if retain else "unverified"}


def android_installed(adb, device, package, folder):
    paths = run(adb,"-s",device,"shell","pm","path",package).splitlines()
    require(len(paths)==1 and paths[0].startswith("package:/data/app/") and paths[0].endswith("/base.apk"),
            "installed APK path differs")
    selected=folder/"installed.apk"
    run(adb,"-s",device,"pull",paths[0].removeprefix("package:"),selected)
    return selected


def android_device_digest(device):
    require(isinstance(device,str) and re.fullmatch(r"emulator-[0-9]+",device),"invalid Android emulator enrollment")
    return hashlib.sha256(device.encode()).hexdigest()


def android_rollback_state(adb, device, package, newer, older):
    listing=run(adb,"-s",device,"shell","dumpsys","rollback")
    states=set()
    for section in re.split(r"(?m)^\s*(?=\d+:\s*$)",listing):
        if re.search(rf"(?m)^\s*{re.escape(package)} {newer} -> {older}\s*$",section):
            state=re.search(r"(?m)^\s*-state: (available|committed)\s*$",section)
            if state:
                states.add(state[1])
    return states


def android(file, row, target, folder):
    emulator, name = target["device"], target["package"]
    require(re.fullmatch(r"emulator-[0-9]+", emulator) and re.fullmatch(r"[A-Za-z][A-Za-z0-9_.]+", name),
            "install target must be the enrolled Android emulator and package")
    adb, aapt, apksigner = target["adb"], target["aapt"], target["apksigner"]
    require(run(adb,"-s",emulator,"shell","getprop","ro.kernel.qemu")=="1",
            "physical Android device is outside this adapter")
    api = run(adb,"-s",emulator,"shell","getprop","ro.build.version.sdk")
    require(row["dimensions"]["os"]=="android-api-"+api,"Android runtime version differs from the signed matrix")
    apk=android_apk(file,aapt,apksigner)
    require((apk["package"],apk["versionName"])==(name,row["version"]),"APK package or version differs")
    for field in ("versionCode","certificate","rollbackDataPolicy"):
        if field in row:
            require(apk[field]==row[field],f"Android signed APK {field} differs")
    operation=target.get("androidOperation","install")
    require(operation in {"install","observe-installed-baseline","enable-rollback","os-rollback"},"unsupported Android installation action")
    if operation in {"enable-rollback","os-rollback"}:
        require(int(api)>=34,"Android rollback runtime is outside the observed support scope")
        source=target["androidSource"]
        installed=android_installed(adb,emulator,name,folder)
        current=digest(installed)
        prior=android_apk(installed,aapt,apksigner)
        require(prior["package"]==name and prior["certificate"]==apk["certificate"]==source["certificate"],
                "Android signing identity changed")
        backwards=operation=="os-rollback"
        newer,older=(source,apk) if backwards else (apk,source)
        require(newer["rollbackDataPolicy"]=="retain" and newer["versionCode"]>older["versionCode"],
                "Android transition does not preserve app data or version direction differs")
        if current==row["payload"]:
            require(("committed" if backwards else "available") in
                    android_rollback_state(adb,emulator,name,newer["versionCode"],older["versionCode"]),"Android OS has no matching rollback state for retry")
        else:
            require(current==source["payload"],"current installed Android bytes differ from approved transition")
            if backwards:
                require(run(adb,"-s",emulator,"shell","pm","rollback-app",name)=="Success",
                        "Android PackageManager rollback unavailable")
            else:
                require(run(adb,"-s",emulator,"install","-r","--enable-rollback",file).splitlines()[-1:] == ["Success"],
                        "Android OS did not accept rollback-enabled update")
    elif operation=="install":
        require(run(adb,"-s",emulator,"install","-r",file).splitlines()[-1:] == ["Success"],"Android APK was not installed")
    selected=android_installed(adb,emulator,name,folder)
    installed=digest(selected)
    require(installed==row["payload"],"installed Android bytes differ")
    observation={"package":name,"installed":installed,"api":api}
    if "versionCode" in row or operation!="install":
        observation.update(certificate=apk["certificate"],versionCode=apk["versionCode"],
                           rollbackDataPolicy=apk["rollbackDataPolicy"],operation=operation,
                           deviceDigest=android_device_digest(emulator))
    return observation


def ios_device_digest(device):
    require(re.fullmatch(r"[a-fA-F0-9]{8}(?:-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}",device),
            "invalid iOS simulator identifier")
    return hashlib.sha256(device.upper().encode()).hexdigest()


def bundle_content(path):
    entries=[]
    for file in sorted(path.rglob("*")):
        name=file.relative_to(path).as_posix()
        if file.is_symlink():
            require(file.resolve().is_relative_to(path.resolve()),"application link escapes the bundle")
            entries.append([name,"link",str(file.readlink())])
        elif file.is_file():
            entries.append([name,"file",digest(file)])
    require(entries,"empty iOS application bundle")
    return hashlib.sha256(json.dumps(entries,ensure_ascii=True,separators=(",",":")).encode()).hexdigest()


def ios(file, row, target, folder):
    device, bundle = target["device"], target["package"]
    session_device=ios_device_digest(device)
    require(re.fullmatch(r"[A-Za-z][A-Za-z0-9_.]+", bundle),
            "invalid iOS simulator enrollment")
    runtime = json.loads(run("xcrun", "simctl", "list", "devices", "booted", "-j"))["devices"]
    observed = next((key for key, devices in runtime.items() if any(d["udid"].upper() == device.upper() and d["state"] == "Booted" for d in devices)), None)
    require(observed, "enrolled iOS simulator is unavailable")
    require(row["dimensions"]["os"] == observed.removeprefix("com.apple.CoreSimulator.SimRuntime.").lower() + "-simulator",
            "iOS simulator runtime differs from the signed matrix")
    with tarfile.open(file) as archive:
        archive.extractall(folder, filter="data")
    applications = [path for path in folder.glob("*.app") if path.is_dir() and not path.is_symlink()]
    require(len(applications) == 1, "archive must include one iOS simulator application")
    source = applications[0]
    info = plistlib.loads((source / "Info.plist").read_bytes())
    main = info["CFBundleExecutable"]
    require(isinstance(main,str) and re.fullmatch(r"[A-Za-z0-9_+.-]+",main) and main not in {".",".."}
            and (source/main).is_file() and not (source/main).is_symlink(),"invalid iOS bundle executable path")
    require(info["CFBundleIdentifier"] == bundle and info["CFBundleShortVersionString"] == row["version"]
            and "iPhoneSimulator" in info["CFBundleSupportedPlatforms"], "iOS installation identity differs")
    require(bundle == row["bundleId"] and info["CFBundleVersion"] == row["bundleBuild"]
            and bundle_content(source)==row["content"] and digest(source/main)==row["executable"],
            "iOS source content does not match signed compatibility evidence")
    run("codesign", "--verify", "--deep", "--strict", source)
    if "profileIdentity" in row:
        require(ios_profile_identity(source,bundle)==row["profileIdentity"],"signed iOS profile entitlement differs")
    run("xcrun", "simctl", "install", device, source)
    installed = Path(run("xcrun", "simctl", "get_app_container", device, bundle, "app"))
    installed_info = plistlib.loads((installed / "Info.plist").read_bytes())
    require(installed_info["CFBundleIdentifier"] == bundle and installed_info["CFBundleShortVersionString"] == row["version"]
            and installed_info["CFBundleExecutable"] == main, "installed iOS metadata differs")
    content, executable = bundle_content(installed), digest(installed/main)
    require(installed_info["CFBundleVersion"]==row["bundleBuild"] and content==row["content"]
            and executable==row["executable"],"installed iOS content differs")
    run("codesign", "--verify", "--deep", "--strict", installed)
    details={"package":bundle,"runtime":observed,"build":installed_info["CFBundleVersion"],
             "content":content,"executable":executable,"deviceDigest":session_device}
    if "profileIdentity" in row:
        effective=ios_profile_identity(installed,bundle)
        require(effective==row["profileIdentity"],"installed iOS profile entitlement differs")
        details["profileIdentity"]=effective
    return details


def board(file, row, target, folder):
    staged = folder / "secondary.bin"
    staged.write_bytes(file.read_bytes())
    require(digest(staged) == row["payload"], "staged image differs")
    board_revision = row["dimensions"]["board"]
    key = Path(target.get("boardKeys",{}).get(row["bootKey"],""))
    require(key.is_file() and key_sha256(key)==row["bootKey"],"board boot root is not independently enrolled")
    options = ("--key", key, "--image", staged, "--version", row["version"],
               "--board", board_revision, "--counter", row["counter"])
    measurement = run("go", "run", "./products/p05-operations-backoffice/cmd/verify-firmware", *options, cwd=ROOT)
    require(measurement==row["measurement"],"counted image measurement differs from the signed matrix")
    primary = folder / "primary.bin"
    staged.replace(primary)
    require(digest(primary) == row["payload"], "accepted board slot differs")
    return {"slot": "host-file", "application": digest(primary), "measurement": measurement, "counter": row["counter"], "bootKey":row["bootKey"]}


def c_consumer(file,row,target,folder):
    from interop import c_adapter, go_adapter

    require("interop" in row,"compiled consumer observation is not enrolled")
    adapter={"go":go_adapter,"c":c_adapter}[row["interop"]["language"]]
    return adapter(file,row,target,folder)


ADAPTERS = {"sdk": sdk, "abi":c_consumer, "android-emulator": android, "ios-simulator": ios, "board-image": board}


def observe(file, row, target):
    require(row["mode"] in ADAPTERS, "no independently enrolled physical installer")
    require(digest(file) == row["payload"], "installation input digest differs")
    with tempfile.TemporaryDirectory(prefix="nu54-install-") as directory:
        details = ADAPTERS[row["mode"]](file, row, target, Path(directory))
    return {"input": row["payload"], "mode": row["mode"], "version": row["version"], "observed": True, "details": details}


def ios_profile_identity(app, package):
    """Read the effective simulator entitlements embedded by the app linker."""
    info = plistlib.loads((app/"Info.plist").read_bytes())
    executable = info["CFBundleExecutable"]
    require(isinstance(executable,str) and re.fullmatch(r"[A-Za-z0-9_+.-]+",executable) and executable not in {".",".."},
            "invalid iOS bundle executable path")
    require(info["CFBundleIdentifier"]==package, "iOS profile bundle identity differs")
    source=app/executable
    require(source.is_file() and not source.is_symlink(),"iOS profile executable unavailable")
    with tempfile.TemporaryDirectory(prefix="nu54-ios-entitlements-") as folder:
        thin=source
        if run("lipo","-archs",source)!="arm64":
            thin=Path(folder)/"app-arm64"
            run("lipo",source,"-thin","arm64","-output",thin)
        commands=run("otool","-arch","arm64","-l",thin)
        section=re.search(r"sectname __entitlements\s+segname __TEXT\s+addr 0x[0-9a-f]+\s+size (0x[0-9a-f]+)\s+offset (\d+)",
                          commands)
        require(section,"iOS profile embedded application entitlement missing")
        size,offset=int(section[1],16),int(section[2])
        require(0 < size <= 16384 and offset+size <= thin.stat().st_size,"invalid iOS profile entitlement bounds")
        with thin.open("rb") as binary:
            binary.seek(offset)
            embedded = plistlib.loads(binary.read(size).rstrip(b"\0"))
    identifier=embedded.get("application-identifier")
    require(isinstance(identifier,str) and re.fullmatch(r"[A-Z0-9]{10}\."+re.escape(package),identifier),
            "iOS profile effective application identifier differs")
    groups=embedded.get("keychain-access-groups",[identifier])
    require(groups==[identifier],"iOS profile Keychain group differs from the dedicated application identity")
    codesign_entitlements=run("codesign","-d","--entitlements","-",app)
    if codesign_entitlements:
        signed=plistlib.loads(codesign_entitlements.encode())
        require(signed.get("application-identifier",identifier)==identifier and
                signed.get("keychain-access-groups",groups)==groups,
                "embedded iOS profile entitlement differs from code-signature entitlement")
    identity={"applicationIdentifier":identifier,"keychainAccessGroups":groups}
    return hashlib.sha256(json.dumps(identity,sort_keys=True,separators=(",",":")).encode()).hexdigest()
