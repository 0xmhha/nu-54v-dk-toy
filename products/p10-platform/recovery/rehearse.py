"""Run the isolated P05/P07 recovery commands against the live synthetic fixture."""

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from uuid import uuid4

HERE = Path(__file__).resolve().parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dsn", required=True)
    parser.add_argument("--rpc", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(exist_ok=True, mode=0o700)
    with tempfile.TemporaryDirectory(prefix="n54dr-", dir="/tmp") as directory:
        root = Path(directory)
        key, public = root / "backup.pem", root / "backup.pub"
        subprocess.run(["openssl", "genpkey", "-algorithm", "ED25519", "-out", str(key)], check=True)
        key.chmod(0o600)
        subprocess.run(["openssl", "pkey", "-in", str(key), "-pubout", "-out", str(public)], check=True)
        archive, scope, goal, job = root / "snapshot", str(uuid4()), str(uuid4()), str(uuid4())
        command = [sys.executable, str(HERE / "recover.py")]
        if path := os.environ.get("POSTGRES_BIN"):
            command.extend(("--postgres-bin", path))
        subprocess.run(command + ["backup", "--scope", scope, "--p05", args.dsn, "--p07", args.dsn,
                                   "--archive", str(archive), "--sign-key", str(key)], check=True)
        report = args.output / "p05-p07-restore.json"
        result = subprocess.run(command + ["inspect", "--scope", scope, "--job", job, "--archive", str(archive),
                    "--backup-key", str(public), "--isolation-root", str(root / "q"), "--rpc", args.rpc,
                    "--synthetic", "--goal-ref", goal, "--rpo", "900", "--rto", "3600", "--report", str(report)])
        evidence = json.loads(report.read_bytes())
        print(json.dumps(evidence, sort_keys=True))
        if result.returncode:
            return result.returncode
        if evidence["reconciliation"]["paidRefunded"] < 3 or not evidence["ordinaryTrafficFenced"]:
            raise ValueError("rehearsal requires non-empty actual settlement, refund and quarantine checks")
        if os.environ.get("NU54_MANAGED_RECOVERY_REHEARSAL") == "1":
            subprocess.run([sys.executable,str(HERE/"test_preparation.py"),"--existing-postgres",
                            "--source",args.dsn,"--rpc",args.rpc,"--output",str(args.output)],check=True)
        payload = archive / "p05.pgdump"
        with payload.open("r+b") as stream:
            stream.seek(-1, os.SEEK_END)
            original = stream.read(1)
            stream.seek(-1, os.SEEK_END)
            stream.write(bytes([original[0] ^ 1]))
        mutated_report = root / "bad-payload-report.json"
        mutated = subprocess.run(command + ["inspect", "--scope", scope, "--job", job, "--archive", str(archive),
                    "--backup-key", str(public), "--isolation-root", str(root / "bad-payload"), "--rpc", args.rpc,
                    "--synthetic", "--goal-ref", goal, "--rpo", "900", "--rto", "3600", "--report", str(mutated_report)],
                    capture_output=True)
        if mutated.returncode == 0 or json.loads(mutated_report.read_bytes()).get("error") != "p05 archive digest differs":
            raise ValueError("backup archive mutation was not rejected")
        public.write_bytes(b"bad key")
        tampered = subprocess.run(command + ["inspect", "--scope", scope, "--job", job, "--archive", str(archive),
                    "--backup-key", str(public), "--isolation-root", str(root / "tampered"), "--rpc", args.rpc,
                    "--synthetic", "--goal-ref", goal, "--rpo", "900", "--rto", "3600",
                    "--report", str(root / "bad-report.json")], capture_output=True)
        if tampered.returncode == 0 or (root / "tampered").exists():
            raise ValueError("unauthenticated archive entered quarantine")
        return 0


if __name__ == "__main__":
    sys.exit(main())
