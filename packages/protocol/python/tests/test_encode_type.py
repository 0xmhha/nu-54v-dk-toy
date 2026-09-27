import json
from pathlib import Path

from nu54_protocol import ENCODE_TYPE

VECTORS = Path(__file__).resolve().parents[4] / "docs/content/specifications/protocol/eip712-vectors.json"


def test_encode_type_matches_vectors():
    for v in json.loads(VECTORS.read_text())["vectors"]:
        assert ENCODE_TYPE[v["primaryType"]] == v["encodeType"], v["id"]
