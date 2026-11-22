"""P09 published place to P05 authenticated hold on disposable PostgreSQL."""
from contextlib import contextmanager
import json
import os
from pathlib import Path
import socket
import struct
import sys
import time
from urllib.error import URLError
from urllib.request import urlopen
import zlib

if __name__ == "__main__":
    sys.dont_write_bytecode = True

from integration_workspace import OwnedProcess, cleanup_stack, integration_workspace, run_command, start_postgres

ROOT = Path(__file__).resolve().parents[1]
BRANCH = "d6d10219-1c27-492c-9bdf-ec14d7d092bd"


def run(*args, env=None):
    return run_command(args, cwd=ROOT, env=env, check=True)


def pg(tool):
    directory = os.getenv("POSTGRES_BIN")
    return str(Path(directory) / tool) if directory else tool


def free_loopback_port():
    with socket.socket() as address:
        address.bind(("127.0.0.1", 0))
        return address.getsockname()[1]


def map_fixture(root: Path):
    """Portable fixture for the normal XYZ route and image validation."""
    path = root / "map" / "0" / "0" / "0.png"
    path.parent.mkdir(parents=True)
    def chunk(kind, body):
        return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body))
    image = b"".join(b"\0" + bytes([137, 192, 201]) * 256 for _ in range(256))
    path.write_bytes(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 256, 256, 8, 2, 0, 0, 0))
                     + chunk(b"IDAT", zlib.compress(image)) + chunk(b"IEND", b""))
    return root / "map", "Synthetic raster route fixture", "0/0/0.png", "0", "0"


@contextmanager
def published_fixture(work: Path, env: dict[str, str], dsn09: str, branch: str = BRANCH, capabilities=("booking",)):
    run(pg("psql"), "-Xv", "ON_ERROR_STOP=1", dsn09, "-c",
        "CREATE ROLE travelread LOGIN IN ROLE nu54_p09_public; "
        "CREATE ROLE travelwrite LOGIN IN ROLE nu54_p09_publisher", env=env)
    stamp = "2026-11-20T00:00:00Z"
    place = {"region": "seoul", "regionName": "서울", "address": "Example square", "source": "Fixture curator", "updatedAt": stamp,
             "planningUnit": "KRW", "opening": [{"weekday": day, "timezone": "Asia/Seoul", "opens": "09:00", "closes": "23:00"} for day in range(7)]}
    (work / "catalog.json").write_text(json.dumps({
        "version": 1, "provider": "fixture", "environment": "synthetic", "updatedAt": stamp,
        "places": [
            {**place, "id": "landmark", "name": "공개 명소", "category": "attraction", "planningCost": "0", "latitude": 37.53, "longitude": 126.91},
            {**place, "id": "merchant", "name": "지역 맛집", "category": "restaurant", "planningCost": "8500", "latitude": 37.57, "longitude": 126.92,
             "branchRef": branch, "capabilities": list(capabilities)},
        ],
    }))
    openssl = os.getenv("OPENSSL", "openssl")
    run(openssl, "genpkey", "-algorithm", "Ed25519", "-out", work / "key.pem", env=env)
    run(openssl, "pkey", "-in", work / "key.pem", "-pubout", "-outform", "DER", "-out", work / "public.der", env=env)
    (work / "public.hex").write_text((work / "public.der").read_bytes()[-32:].hex())
    run(openssl, "pkeyutl", "-sign", "-inkey", work / "key.pem", "-rawin", "-in", work / "catalog.json",
        "-out", work / "signature", env=env)
    run("go", "build", "-o", work / "placesd", "./products/p09-travel-ai/cmd/placesd", env=env)
    service = (work / "placesd", "-trust-key", work / "public.hex")
    run(*service, "-install", work / "catalog.json", "-signature", work / "signature",
        env={**env, "P09_DATABASE_URL": dsn09 + " user=travelwrite"})
    addr = f"127.0.0.1:{free_loopback_port()}"
    archive, label, tile, low, high = map_fixture(work)
    if env.get("P09_TEST_MAP_DIR"):
        archive, label, tile = Path(env["P09_TEST_MAP_DIR"]), env["P09_TEST_MAP_ATTRIBUTION"], env["P09_TEST_MAP_TILE"]
        low, high = env["P09_TEST_MAP_MIN"], env["P09_TEST_MAP_MAX"]
    with cleanup_stack() as stack:
        process = OwnedProcess()
        stack.callback(process.stop)
        server = process.start([*service, "-listen", addr, "-map-dir", archive, "-map-attribution", label,
                                "-map-min-zoom", low, "-map-max-zoom", high], cwd=ROOT,
                               env={**env, "P09_DATABASE_URL": dsn09 + " user=travelread"})
        for _ in range(40):
            if server.poll() is not None:
                raise RuntimeError("P09 exited before listening")
            try:
                with urlopen(f"http://{addr}/v1/regions", timeout=1) as response:
                    if json.load(response) != ["seoul"]:
                        raise RuntimeError("Unexpected P09 region lookup")
                break
            except URLError:
                time.sleep(0.1)
        else:
            raise RuntimeError("P09 did not start")
        with urlopen(f"http://{addr}/v1/region-options", timeout=2) as response:
            if json.load(response) != [{"code": "seoul", "name": "서울"}]:
                raise RuntimeError("Localized region label not loaded")
        with urlopen(f"http://{addr}/v1/map", timeout=2) as response:
            if json.load(response) != {"attribution": label, "minZoom": int(low), "maxZoom": int(high)}:
                raise RuntimeError("Map attribution or bounds differ")
        with urlopen(f"http://{addr}/v1/map/{tile}", timeout=2) as response:
            if response.read() != (archive / tile).read_bytes() or response.headers["Content-Type"] != "image/png":
                raise RuntimeError("Published map tile differs from approved archive")
        yield f"http://{addr}"


def verify(work: Path, env: dict[str, str], dsn05: str, dsn09: str):
    publication_dsn = dsn09 + "_publication"
    for owner, database, dsn in (
        ("p05", "nu54_commerce_travel", dsn05), ("p09", "nu54_p09_travel", dsn09),
        ("p09", "nu54_p09_travel_publication", publication_dsn),
    ):
        run(pg("createdb"), "-h", str(work), database, env=env)
        run("python3", "products/p10-platform/database/migrate.py", owner, env={
            **env, f"{owner.upper()}_MIGRATION_DATABASE_URL": dsn,
        })
    with published_fixture(work, env, dsn09) as public_url:
        run("go", "test", "-count=1", "-v", "./products/p05-operations-backoffice/internal/commerce",
            "-run", "^TestTravel", env={**env, "P05_TRAVEL_TEST_DATABASE_URL": dsn05,
                                       "P09_TRAVEL_TEST_PUBLIC_URL": public_url})
    run("go", "test", "-count=1", "-v", "./products/p09-travel-ai/internal/places", env={
        **env, "P09_TEST_READ_URL": publication_dsn + " user=travelread",
        "P09_TEST_WRITE_URL": publication_dsn + " user=travelwrite",
    })


def main():
    with integration_workspace("nu54-travel-", pg("pg_ctl"), "pgdata", dict(os.environ)) as (work, env):
        data = work / "pgdata"
        port = str(free_loopback_port())
        env.update(PGPORT=port, PGHOST=str(work))
        if os.getenv("POSTGRES_BIN"):
            env["PATH"] = str(Path(pg("psql")).parent) + os.pathsep + env["PATH"]
        run(pg("initdb"), "-D", data, "-A", "trust", "--no-locale", "--encoding=UTF8", env=env)
        start_postgres(pg("pg_ctl"), data, work / "postgres.log",
                       f"-k {work} -p {port} -c listen_addresses=''", env, cwd=ROOT)
        base = f"host={work} port={port} dbname="
        verify(work, env, base + "nu54_commerce_travel", base + "nu54_p09_travel")



if __name__ == "__main__":
    main()
