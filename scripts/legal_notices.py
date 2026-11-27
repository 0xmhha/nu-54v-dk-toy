#!/usr/bin/env python3
"""Write and check the legal notices shown by the two mobile apps.

    python3 -B scripts/legal_notices.py                    write both apps' notices.generated.json
    python3 -B scripts/legal_notices.py --check            fail when a generated file is outdated
    python3 -B scripts/legal_notices.py --release --app p02-user-app
                                                           fail unless the app may be given to the public
    python3 -B scripts/legal_notices.py --blanks           list what an owner still has to decide
    python3 -B scripts/legal_notices.py --refresh-android --app p02-user-app --resolved-runtime FILE
                                                           record the resolved Android modules

Everything is read from the checkout and its node_modules; nothing is downloaded. The documents are
docs/content/commercial/legal, the licence inputs that node_modules does not have are
scripts/legal-licenses. --refresh-android also reads the POMs of the local Gradle cache.

--release has no switch that turns it off. Fill the blanks, finish the documents and run the script.
"""

import argparse
from collections import deque
from datetime import date
import json
import os
from pathlib import Path
import re
import sys
import xml.etree.ElementTree as ET
from zipfile import ZipFile


ROOT = Path(__file__).resolve().parents[1]
SOURCES = Path("docs/content/commercial/legal")
LICENSES = Path("scripts/legal-licenses")
APPS = {"p02-user-app": "user-app", "p04-merchant-kiosk": "kiosk"}
DOCUMENTS = ("terms", "privacy", "location", "test-assets")
LANGUAGES = ("ko", "en")
BLANK = {"ko": "[아직 정해지지 않음]", "en": "[not decided yet]"}
EDITION_LIMIT = 32  # The app stores the edition with the agreement.
LICENSE_FILE = re.compile(r"(licen[cs]e|copying)([._-].*)?", re.IGNORECASE)
NOTICE_FILE = re.compile(r"notice([._-].*)?", re.IGNORECASE)
SOURCE_SUFFIXES = (".js", ".cjs", ".mjs", ".ts", ".flow", ".kt", ".java", ".h", ".py")


class LegalError(Exception):
    """One or more problems, one per line."""


def read_json(path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise LegalError(f"{path}: {error}") from error


def clean(text):
    return "\n".join(line.rstrip() for line in text.replace("\r\n", "\n").replace("\r", "\n").split("\n")).strip("\n") + "\n"


def output_path(root, app):
    return root / "products" / app / "src/legal/notices.generated.json"


# Documents

def read_publisher(root):
    data = read_json(root / SOURCES / "publisher.json")
    if not all(isinstance(values, dict) for values in data.values()) or set(data.get("publisher", {})) != {"name", "contact"}:
        raise LegalError("publisher.json needs publisher {name, contact} and groups of values")
    for group, values in data.items():
        for key, value in values.items():
            if value is not None and not isinstance(value, (str, dict)):
                raise LegalError(f"publisher.json {group}.{key} must be text, {{ko, en}} or null")
    return data


def fill(text, publisher, language):
    """Puts the decided values into a text. Returns the text and the blanks it still has."""
    blanks = []

    def value(match):
        group, key = match[1].split(".")
        if group not in publisher or key not in publisher[group]:
            raise LegalError(f"unknown placeholder {match[0]}")
        found = publisher[group][key]
        if isinstance(found, dict):
            found = found.get(language)
        if not isinstance(found, str) or not found.strip():
            blanks.append(f"{group}.{key}")
            return BLANK[language]
        return found.strip()

    return re.sub(r"\{\{([a-z]+\.[a-zA-Z]+)\}\}", value, text), blanks


def read_document(path, publisher, name, language):
    text = path.read_text(encoding="utf-8").replace("\r\n", "\n")
    match = re.match(r"---\n(.*?)\n---\n(.*)", text, re.DOTALL)
    if not match:
        raise LegalError(f"{path}: expected a header between --- lines")
    header = dict(line.split(": ", 1) for line in match[1].splitlines() if ": " in line)
    if set(header) != {"id", "language", "title", "edition", "status", "effective"}:
        raise LegalError(f"{path}: the header needs id, language, title, edition, status and effective")
    if header["id"] != name or header["language"] != language:
        raise LegalError(f"{path}: the header names another document")
    if header["status"] not in ("draft", "final"):
        raise LegalError(f"{path}: status is draft or final")
    if not 0 < len(header["edition"]) <= EDITION_LIMIT:
        raise LegalError(f"{path}: edition needs 1 to {EDITION_LIMIT} characters")
    effective = None if header["effective"] == "null" else header["effective"]
    if effective is not None:
        try:
            if date.fromisoformat(effective).isoformat() != effective:
                raise ValueError
        except ValueError:
            raise LegalError(f"{path}: effective is null or YYYY-MM-DD") from None
    intro, *parts = re.split(r"^## ", match[2], flags=re.MULTILINE)
    if header["status"] == "draft" and not re.search("unreviewed draft|검토받지 않은 초안", intro):
        raise LegalError(f"{path}: a draft says above its first section that it is an unreviewed draft")
    sections, blanks = [], []
    for part in parts:
        heading, _, body = part.partition("\n")
        body, missing = fill(clean(body).strip(), publisher, language)
        if not heading.strip() or not body:
            raise LegalError(f"{path}: a section has no heading or no text")
        blanks += missing
        sections.append({"heading": heading.strip(), "body": body})
    if not sections:
        raise LegalError(f"{path}: no sections")
    return {"id": name, "language": language, "title": header["title"], "edition": header["edition"],
            "status": header["status"], "effective": effective, "sections": sections}, blanks


def read_documents(root, app):
    """The app's documents, and what is still undecided about them."""
    publisher = read_publisher(root)
    documents, open_points, errors = [], [], []
    for group, values in publisher.items():
        for key in sorted(values):
            if fill("{{%s.%s}}" % (group, key), publisher, "ko")[1] or fill("{{%s.%s}}" % (group, key), publisher, "en")[1]:
                open_points.append(f"{SOURCES}/publisher.json: {group}.{key} is blank")
    for language in LANGUAGES:
        for name in DOCUMENTS:
            path = SOURCES / APPS[app] / f"{name}.{language}.md"
            try:
                document, _ = read_document(root / path, publisher, name, language)
            except (OSError, LegalError) as error:
                errors.append(str(error).replace(str(root) + "/", ""))
                continue
            documents.append(document)
            if document["status"] != "final":
                open_points.append(f"{path}: status is draft")
            if document["effective"] is None:
                open_points.append(f"{path}: effective date is blank")
    if not errors:
        for name in DOCUMENTS:
            versions = {(item["edition"], item["status"], item["effective"]) for item in documents if item["id"] == name}
            if len(versions) != 1:
                errors.append(f"{SOURCES}/{APPS[app]}/{name}: the languages differ in edition, status or effective date")
    if errors:
        raise LegalError("\n".join(errors))
    named = {key: fill("{{publisher.%s}}" % key, publisher, "ko") for key in ("name", "contact")}
    return {key: None if blanks else text for key, (text, blanks) in named.items()}, documents, open_points


# Licence texts

def spdx_text(root, identifier):
    path = root / LICENSES / "texts" / f"{identifier}.txt"
    return clean(path.read_text(encoding="utf-8")) if path.is_file() else None


def standard_license(expression):
    """The licence to apply when metadata offers a choice; None when it is not one plain choice."""
    first = expression.strip().strip("()").split(" OR ")[0].strip()
    return first if re.fullmatch(r"[A-Za-z0-9.+-]+", first) else None


def substituted_text(root, name, expression, holders, origin):
    chosen = standard_license(expression)
    text = chosen and spdx_text(root, chosen)
    if not text:
        raise LegalError(f"{name}: no licence text, and {expression!r} has no standard text in {LICENSES}/texts")
    if not holders:
        raise LegalError(f"{name}: no licence text and no author or copyright holder to name")
    lines = [item if item.lower().startswith(("copyright", "portions copyright")) else f"Copyright (c) {item}" for item in holders]
    note = (f"This package carries no licence file. {origin} declares {expression}; the standard {chosen} text is "
            "shown with the holder or author the package names.")
    if "{copyright}" in text:
        return clean(f"{note}\n\n" + text.replace("{copyright}", "\n".join(lines)))
    return clean(f"{note}\nNamed by the package: {'; '.join(holders)}\n\n{text}")


def agrees(expression, text):
    """False when the text is plainly another licence than the declared one."""
    marks = {"Apache-2.0": "Apache License", "MIT": "Permission is hereby granted", "ISC": "Permission to use, copy, modify",
             "BSD-3-Clause": "Redistribution and use", "BSD-2-Clause": "Redistribution and use"}
    flat = " ".join(text.split())
    if expression not in marks or marks[expression] in flat:
        return True
    return not any(mark in flat for mark in marks.values())


def person(value):
    if isinstance(value, dict):
        value = value.get("name")
    return re.sub(r"\s*[<(].*", "", value).strip() if isinstance(value, str) else ""


def copyright_holders(directory, manifest):
    """The holder named by the metadata, or by the header of the package's own sources."""
    if isinstance(manifest.get("copyright"), str):
        return [manifest["copyright"]]
    found = set()
    for base, folders, files in os.walk(directory):
        # A local Gradle build leaves files in build/, so they are not the package's own.
        folders[:] = sorted(name for name in folders if name not in ("node_modules", "build") and not name.startswith("."))
        for name in files:
            if name.endswith(SOURCE_SUFFIXES):
                head = (Path(base) / name).read_bytes()[:2000].decode("utf-8", "replace")
                found.update(item.strip(" .*/") for item in re.findall(r"^[ \t/*#]*Copyright \(c\) ([^\n]{2,120})", head, re.MULTILINE | re.IGNORECASE))
    if found:
        return sorted(found)
    author = person(manifest.get("author"))
    return [author] if author else []


def package_files(directory):
    files = sorted(item for item in directory.iterdir() if item.is_file())
    texts = [item for item in files if LICENSE_FILE.fullmatch(item.name)]
    return texts + [item for item in files if texts and NOTICE_FILE.fullmatch(item.name)]


# JavaScript packages

def find_package(directory, name):
    for base in (directory, *directory.parents):
        manifest = base / "node_modules" / name / "package.json"
        if manifest.is_file():
            return manifest.parent.resolve()
    return None


def production_closure(root, app):
    """Every package the app's dependencies need, as {directory: manifest}. Build tools of React Native are in it."""
    start = (root / "products" / app).resolve()
    queue, seen, errors = deque([start]), {}, []
    while queue:
        directory = queue.popleft()
        if directory in seen:
            continue
        manifest = seen[directory] = read_json(directory / "package.json")
        optional = manifest.get("peerDependenciesMeta", {})
        names = set(manifest.get("dependencies", {}))
        names |= {name for name in manifest.get("peerDependencies", {}) if not optional.get(name, {}).get("optional")}
        for name in sorted(names):
            found = find_package(directory, name)
            if found is None:
                errors.append(f"{manifest.get('name')}: dependency {name} is not installed; run pnpm install")
            else:
                queue.append(found)
    if errors:
        raise LegalError("\n".join(errors))
    del seen[start]
    return seen


def package_license(root, directory, manifest, corrections):
    name, version = manifest["name"], manifest["version"]
    expression = manifest.get("license")
    if not isinstance(expression, str) or not expression:
        raise LegalError(f"{name} {version}: package.json declares no licence")
    entry = {"name": name, "version": version, "license": expression}
    files = package_files(directory)
    if files:
        parts = [clean(item.read_text(encoding="utf-8", errors="replace")) for item in files]
        entry["text"] = parts[0] if len(parts) == 1 else "\n".join(f"{item.name}\n\n{part}" for item, part in zip(files, parts))
        if not entry["text"].strip():
            raise LegalError(f"{name} {version}: the licence file is empty")
        if not agrees(expression, entry["text"]):
            fix = corrections.get(name)
            if not fix or fix["declared"] != expression:
                raise LegalError(f"{name} {version}: declares {expression}, but its licence file is another licence")
            entry["license"] = fix["license"]
    elif "node_modules" not in directory.relative_to(root.resolve()).parts:
        # A package of this repository: the licence at the repository root covers it.
        text = spdx_text(root, expression)
        if not text:
            raise LegalError(f"{name}: no licence text for {expression}")
        entry["text"] = text
    else:
        entry["text"] = substituted_text(root, f"{name} {version}", expression, copyright_holders(directory, manifest), "Its package.json")
        entry["substituted"] = True
    return entry


# Native code

def gradle_inputs(root, app):
    """The build lines that choose Android modules. The recorded module list belongs to exactly these."""
    files = [f"products/{app}/android/app/build.gradle", "scripts/android/runtime-constraints.gradle",
             "packages/protocol/platform/android/nu54-mobile.gradle"]
    wanted = re.compile(r"""['"][\w.-]+:[\w.-]+(:[^'"]+)?['"]|\bstrictly\b|^def \w+Version\b""")
    lines = []
    for name in files:
        rows = (root / name).read_text(encoding="utf-8").splitlines()
        lines += [f"{Path(name).name}: {row.strip()}" for row in rows if not row.strip().startswith(("*", "//")) and wanted.search(row.strip())]
    for name in ("react-native", "react-native-safe-area-context"):
        directory = find_package(root / "products" / app, name)
        if directory:
            lines.append(f"{name}: {read_json(directory / 'package.json')['version']}")
    return lines


def pods(root, app):
    text = next(iter(sorted((root / "products" / app / "ios").glob("Podfile.lock")))).read_text(encoding="utf-8")
    block = text.split("PODS:\n", 1)[1].split("\n\n", 1)[0]
    return {match[1]: match[2] for match in re.finditer(r'^  - "?([\w.+-]+)(?:/[^ ]+)? \(([^)]+)\)"?:?$', block, re.MULTILINE)}


def native_text(root, name, row):
    if "copyright" in row:
        return substituted_text(root, name, row["license"], row["copyright"], row["origin"]), True
    parts = []
    for item in row["texts"]:
        path = root / item if "/" in item else root / LICENSES / "texts" / f"{item}.txt"
        if not path.is_file():
            raise LegalError(f"{name}: licence text {item} is missing")
        parts.append(clean(path.read_text(encoding="utf-8")))
    if not parts or not all(part.strip() for part in parts):
        raise LegalError(f"{name}: no licence text")
    return "\n".join(([clean(row["note"])] if "note" in row else []) + parts), bool(row.get("substituted"))


def native_licenses(root, app, native):
    record = native["android"][app]
    errors, entries = [], []
    current = gradle_inputs(root, app)
    if current != record["inputs"]:
        changes = sorted(set(current) ^ set(record["inputs"])) or ["order of the lines"]
        errors.append(f"{app}: Android build inputs changed ({'; '.join(changes)}). Resolve the runtime and use --refresh-android")
    for coordinates in record["modules"]:
        group, module, version = coordinates.split(":")
        row = native["modules"].get(coordinates)
        if not row:
            errors.append(f"{coordinates}: no recorded licence")
            continue
        entries.append((f"{group}:{module}", version, row))
    chosen = pods(root, app)
    toml = root / "node_modules/react-native/gradle/libs.versions.toml"
    versions = dict(re.findall(r'^(\w+)\s*=\s*"([^"]+)"', toml.read_text(encoding="utf-8"), re.MULTILINE)) if toml.is_file() else {}
    covered = set()
    for row in native["bundled"]:
        if app not in row["apps"]:
            continue
        version = row.get("version")
        if "toml" in row:
            version = versions.get(row["toml"], "").replace("_", ".")
        elif "pod" in row:
            covered.add(row["pod"])
            version = chosen.get(row["pod"])
        if not version:
            errors.append(f"{row['name']}: its version is no longer found; check {LICENSES}/native.json")
            continue
        entries.append((row["name"], version, row))
    react = find_package(root / "products" / app, "react-native")
    react_version = read_json(react / "package.json")["version"] if react else None
    for pod, version in sorted(chosen.items()):
        if pod in covered or version == react_version or pod in native["podPackages"]:
            continue
        errors.append(f"iOS pod {pod} {version}: no recorded licence")
    result = []
    for name, version, row in entries:
        try:
            text, substituted = native_text(root, f"{name} {version}", row)
        except LegalError as error:
            errors.append(str(error))
            continue
        result.append({"name": name, "version": version, "license": row["license"], "text": text, **({"substituted": True} if substituted else {})})
    if errors:
        raise LegalError("\n".join(errors))
    return result


def read_runtime(path):
    """Rows of nu54PrintResolvedRuntime: group:module:version=configuration."""
    rows = sorted(row.split("=")[0] for row in path.read_text(encoding="utf-8").splitlines() if row.strip())
    if not rows or any(len(row.split(":")) != 3 for row in rows):
        raise LegalError(f"{path}: not a resolved Android runtime")
    return rows


def runtime_problems(root, app, path):
    recorded, resolved = set(read_json(root / LICENSES / "native.json")["android"][app]["modules"]), set(read_runtime(path))
    return [f"{item}: in the Android build, not in the notices" for item in sorted(resolved - recorded)] + \
        [f"{item}: in the notices, no longer in the Android build" for item in sorted(recorded - resolved)]


# The generated file

def licenses(root, app):
    native = read_json(root / LICENSES / "native.json")
    rows, errors = {}, []
    for directory, manifest in production_closure(root, app).items():
        try:
            entry = package_license(root, directory, manifest, native["corrections"])
        except LegalError as error:
            errors.append(str(error))
            continue
        # A package installed twice in one version is listed once; differing texts are both shown.
        same = rows.setdefault((entry["name"], entry["version"]), [])
        if entry not in same:
            same.append(entry)
    try:
        for entry in native_licenses(root, app, native):
            rows.setdefault((entry["name"], entry["version"]), []).append(entry)
    except LegalError as error:
        errors.append(str(error))
    if errors:
        raise LegalError("\n".join(errors))
    return [entry for key in sorted(rows) for entry in sorted(rows[key], key=lambda item: item["text"])]


def build(root, app):
    publisher, documents, open_points = read_documents(root, app)
    texts, rows = [], []
    for entry in licenses(root, app):
        if entry["text"] not in texts:
            texts.append(entry["text"])
        rows.append({**entry, "text": texts.index(entry["text"])})
    return {"version": 1, "publisher": publisher, "documents": documents, "licenses": rows, "texts": texts}, open_points


def render(data):
    return json.dumps(data, ensure_ascii=False, indent=1) + "\n"


def outdated(root, app, data):
    path = output_path(root, app)
    return not path.is_file() or path.read_text(encoding="utf-8") != render(data)


def check(root, app):
    data, _ = build(root, app)
    if outdated(root, app, data):
        raise LegalError(f"{output_path(root, app).relative_to(root)} is outdated; run python3 -B scripts/legal_notices.py")
    return data


def release(root, app, resolved=None):
    data, problems = build(root, app)
    if outdated(root, app, data):
        problems.append(f"{output_path(root, app).relative_to(root)} is outdated; run python3 -B scripts/legal_notices.py")
    if resolved:
        problems += runtime_problems(root, app, resolved)
    if problems:
        raise LegalError("\n".join(problems))


# Recording the Android modules

LICENSE_NAMES = ((r"apache", "Apache-2.0"), (r"\bmit\b", "MIT"))


def pom_facts(cache, coordinates):
    group, module, version = coordinates.split(":")
    for _ in range(4):
        found = sorted((cache / group / module / version).glob("*/*.pom"))
        if not found:
            raise LegalError(f"{coordinates}: no POM in the local Gradle cache")
        tree = ET.parse(found[0]).getroot()
        for node in tree.iter():  # Old POMs have no namespace.
            if isinstance(node.tag, str):
                node.tag = node.tag.rsplit("}", 1)[-1]
        names = [item.findtext("name", "") for item in tree.iter("license")]
        if names:
            people = [item.findtext("organization") or item.findtext("name") or item.findtext("id") or "" for item in tree.iter("developer")]
            return names, sorted({item.strip() for item in people if item.strip()})
        parent = tree.find("parent")
        if parent is None:
            break
        group, module, version = (parent.findtext(key) for key in ("groupId", "artifactId", "version"))
    raise LegalError(f"{coordinates}: its POM declares no licence")


def embedded_notices(cache, coordinates):
    group, module, version = coordinates.split(":")
    found = {}
    for archive in sorted((cache / group / module / version).glob("*/*.[aj]ar")):
        if archive.name.endswith("-sources.jar"):
            continue
        with ZipFile(archive) as content:
            for name in content.namelist():
                if NOTICE_FILE.fullmatch(Path(name).name):
                    found[name] = clean(content.read(name).decode("utf-8", "replace"))
    return found


def refresh_android(root, app, resolved, cache):
    path = root / LICENSES / "native.json"
    native = read_json(path)
    modules = read_runtime(resolved)
    overrides, errors = native["moduleOverrides"], []
    for coordinates in modules:
        module = coordinates.rsplit(":", 1)[0]
        if module in overrides:
            native["modules"][coordinates] = overrides[module]
            continue
        try:
            names, people = pom_facts(cache, coordinates)
            extra = embedded_notices(cache, coordinates)
        except LegalError as error:
            errors.append(str(error))
            continue
        chosen = [spdx for pattern, spdx in LICENSE_NAMES if len(names) == 1 and re.search(pattern, names[0], re.IGNORECASE)]
        if len(chosen) != 1 or extra or not people:
            errors.append(f"{coordinates}: licence {names}, holders {people}, notices {sorted(extra)} need a moduleOverrides entry")
            continue
        native["modules"][coordinates] = {"license": chosen[0], "copyright": people, "origin": "Its Maven POM"}
    if errors:
        raise LegalError("\n".join(errors))
    native["android"][app] = {"inputs": gradle_inputs(root, app), "modules": modules}
    used = {item for record in native["android"].values() for item in record["modules"]}
    native["modules"] = {key: native["modules"][key] for key in sorted(native["modules"]) if key in used}
    path.write_text(json.dumps(native, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")


def main(arguments=None, root=ROOT):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = parser.add_mutually_exclusive_group()
    for name in ("check", "release", "blanks", "refresh-android"):
        mode.add_argument(f"--{name}", action="store_true")
    parser.add_argument("--app", choices=tuple(APPS), help="only this app")
    parser.add_argument("--resolved-runtime", type=Path, help="output of :app:nu54PrintResolvedRuntime")
    args = parser.parse_args(arguments)
    apps = [args.app] if args.app else list(APPS)
    if args.resolved_runtime and (not args.app or args.blanks):
        parser.error("--resolved-runtime needs --app and --check, --release or --refresh-android")
    if args.refresh_android and not args.resolved_runtime:
        parser.error("--refresh-android needs --app and --resolved-runtime")
    failed = False
    for app in apps:
        try:
            if args.release:
                release(root, app, args.resolved_runtime)
                print(f"{app}: legal notices allow distribution")
            elif args.check:
                data = check(root, app)
                problems = runtime_problems(root, app, args.resolved_runtime) if args.resolved_runtime else []
                if problems:
                    raise LegalError("\n".join(problems))
                print(f"{app}: {len(data['documents'])} documents and {len(data['licenses'])} licences are current")
            elif args.blanks:
                print("\n".join(read_documents(root, app)[2]) or f"{app}: nothing left to decide")
            elif args.refresh_android:
                home = Path(os.environ.get("GRADLE_USER_HOME") or Path.home() / ".gradle")
                refresh_android(root, app, args.resolved_runtime, home / "caches/modules-2/files-2.1")
            else:
                data, open_points = build(root, app)
                path = output_path(root, app)
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(render(data), encoding="utf-8")
                replaced = sum(1 for item in data["licenses"] if item.get("substituted"))
                print(f"{app}: {len(data['documents'])} documents ({len(open_points)} open points), "
                      f"{len(data['licenses'])} licences ({replaced} with a standard text), {len(data['texts'])} texts")
        except (LegalError, OSError) as error:
            failed = True
            title = "is not ready for distribution" if args.release else "legal notices failed"
            print(f"{app} {title}:", *(f"  - {line}" for line in str(error).splitlines()), sep="\n", file=sys.stderr)
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
