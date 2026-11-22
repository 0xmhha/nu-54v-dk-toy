"""Measure interoperability using the exact installed Protocol SDK consumer."""

import hashlib
from pathlib import Path
from zipfile import ZipFile

from evidence import digest, require

ROOT=Path(__file__).resolve().parents[3]
VECTORS=ROOT/"packages/protocol/v2/testdata/quote.vectors"
CONSUMER=Path(__file__).with_name("consumers")/"quote.mts"
CHECKS={
    "c":("quote-bytes-and-signature","invalid-vectors","fragments-and-reconnect"),
    "typescript":("quote-bytes-and-signature","invalid-vectors","service-denial","fragments-and-reconnect"),
    "go":("quote-bytes-and-signature","invalid-vectors","service-denial","v1-fragments-reset"),
    "python":("quote-bytes-and-signature","invalid-vectors"),
}


def expectation(language, vectors):
    from fleet import text_hash

    require(language in CHECKS,"unrecognized SDK consumer")
    return {"language":language,"vectors":text_hash(vectors),"checks":list(CHECKS[language]),"scope":"host-consumer"}


def measured_bytes(checksum,entry):
    require(checksum==entry["vectors"]==digest(VECTORS),"installed protocol vectors differ from canonical consent fixture")
    return expectation(entry["language"],checksum)


def measured(path,entry):
    return measured_bytes(digest(path),entry)


def c_adapter(file,row,target,folder):
    from build_sdk import c_consumer, unpack

    entry=row["interop"]
    require(entry["language"]=="c","C host observation required")
    checked=folder/"preflight"
    unpack(file,checked)
    report=measured(checked/"nu54-protocol/spec-v2/testdata/quote.vectors",entry)
    c_consumer(file,folder)
    return {"archive":digest(file),"interop":report}


def verify_typescript(folder, entry):
    from observe import run

    require(entry["language"]=="typescript","TypeScript host observation required")
    result=measured(folder/"node_modules/@nu54/protocol/spec-v2/testdata/quote.vectors",entry)
    local=folder/"check.mts"
    local.write_bytes(CONSUMER.read_bytes())
    run("node","--experimental-strip-types",local.name,cwd=folder)
    return result


def go_adapter(file,row,target,folder):
    from build_sdk import go_consumer, unpack

    probe=folder/"preflight"
    unpack(file,probe)
    module_archives=list(probe.rglob("*.zip"))
    require(len(module_archives)==1,"Go SDK proxy does not contain an exact module archive")
    with ZipFile(module_archives[0]) as package:
        vectors=next(name for name in package.namelist() if name.endswith("/spec-v2/testdata/quote.vectors"))
        result=measured_bytes(hashlib.sha256(package.read(vectors)).hexdigest(),row["interop"])
    go_consumer(file,folder,row["version"],Path(target["goProxy"]),collect_inventory=False)
    return {"archive":digest(file),"interop":result}


def python_adapter(file,row,target,folder):
    from build_sdk import python_consumer

    with ZipFile(file) as package:
        vector=package.read("nu54_protocol/spec_v2/testdata/quote.vectors")
        result=measured_bytes(hashlib.sha256(vector).hexdigest(),row["interop"])
    python_consumer(file,folder,row["version"],Path(target["pythonWheels"]))
    return {"archive":digest(file),"interop":result}
