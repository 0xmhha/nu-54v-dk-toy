"""Reproduce a historical candidate against its immutable pre-merge checkpoint.
Current contracts are checked by validate_approval_baseline.py and validate_specs.py.
"""
import json, subprocess, sys
from pathlib import Path
ROOT = Path(__file__).resolve().parents[2]
checkpoint = ROOT / 'design-history/approval-premerge-20260918'
script = checkpoint / 'content/specifications/validate_wallet_api.py'
print(json.dumps({'scope':'historical_checkpoint_only','currentBaselineValidator':'content/specifications/validate_approval_baseline.py','originalValidator':str(script)}), flush=True)
result = subprocess.run([sys.executable, str(script)], cwd=checkpoint)
raise SystemExit(result.returncode)
