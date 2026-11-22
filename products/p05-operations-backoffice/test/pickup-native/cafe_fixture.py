"""Bind the requested K05 journey to both enrolments and named tests."""

import hashlib
import json
from pathlib import Path
import re
import sys

TESTS = {
    "unknown": ("testStaffPrepareAndHandover", "testPurchasedReceiptReadyAndCollectedAcrossRelaunch"),
    "nearby": ("testStaffNearbyReadyThenReturnToPublicOriginal", "testNearbyCustomerReadsReadyOriginalAcrossRelaunch"),
}
UUID = re.compile(r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}")
CONTROL = re.compile(r"http://127\.0\.0\.1:[0-9]+")
NEARBY_FIELDS = ("publicSale", "publicEvent", "publicNumber")


def staged_app_inputs(stage):
    apps = {
        "p02-user-app": ("NU54Renter", ("src/merchant/CustomerOrders.tsx",)),
        "p04-merchant-kiosk": ("NU54Kiosk", ("src/kiosk/MerchantScreen.tsx", "src/kiosk/PickupKitchen.tsx")),
    }
    manifests = {}
    for product, (target, references) in apps.items():
        app = stage / "products" / product
        if not app.resolve().is_relative_to(stage.resolve()):
            raise ValueError("application source escaped the private stage")
        files = [app / name for name in ("app.json", "babel.config.js", "index.js", "metro.config.js", "package.json")]
        for directory in (app / "src", app / "ios" / target):
            if not directory.is_dir() or not directory.resolve().is_relative_to(app.resolve()):
                raise ValueError("missing private application source")
            files.extend(file for file in directory.rglob("*") if file.is_file())
        manifest = {}
        for file in files:
            if not file.resolve().is_relative_to(app.resolve()):
                raise ValueError("application input is outside the private stage")
            manifest[str(file.relative_to(app))] = hashlib.sha256(file.read_bytes()).hexdigest()
        combined = json.dumps(manifest, sort_keys=True, separators=(",", ":"))
        manifests[product] = {"sha256": hashlib.sha256(combined.encode()).hexdigest(), "files": len(manifest),
                              "references": {name: manifest[name] for name in references}}
    return manifests


def read_pair(lab, variant):
    if variant not in TESTS:
        raise ValueError("unsupported café fixture variant")
    documents = [json.loads((lab / (role + ".json")).read_text()) for role in ("staff", "customer")]
    keys = ("sale", "control") + (NEARBY_FIELDS if variant == "nearby" else ())
    for doc in documents:
        if doc.get("variant", "unknown") != variant:
            raise ValueError("unexpected café enrollment variant")
        if not UUID.fullmatch(doc["sale"]) or not CONTROL.fullmatch(doc["control"]):
            raise ValueError("invalid café enrollment scope")
        if variant == "nearby":
            if (not UUID.fullmatch(doc["publicSale"]) or not UUID.fullmatch(doc["publicEvent"]) or
                    doc["publicSale"] == doc["sale"] or type(doc["publicNumber"]) is not int or doc["publicNumber"] <= 0):
                raise ValueError("invalid separate public original")
        elif any(key in doc for key in NEARBY_FIELDS):
            raise ValueError("nearby enrollment cannot verify an unknown payment")
    if any(documents[0][key] != documents[1][key] for key in keys):
        raise ValueError("café enrollments refer to different originals")
    return documents[0]


def rendered_test_passed(log, name):
    pattern = re.escape("Test Case '-[SceneSmoke.CafePickupInstalledApps " + name + "]' passed (")
    return len(re.findall("^" + pattern, log, flags=re.MULTILINE)) == 1


if __name__ == "__main__":
    lab, template, variant = sys.argv[1:]
    lab = Path(lab)
    data = read_pair(lab, variant)
    content = Path(template).read_text()
    substitutions = {"SALE": "sale", "CONTROL": "control", "PUBLIC_SALE": "publicSale",
                     "PUBLIC_EVENT": "publicEvent", "PUBLIC_NUMBER": "publicNumber"}
    for token, key in substitutions.items():
        content = content.replace("@" + token + "@", str(data.get(key, "")))
    (lab / "SceneSmoke.swift").write_text(content)
    print(*TESTS[variant], sep="\n")
