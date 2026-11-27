"""Carry signed P09 journal pages over pinned mTLS endpoints without rewriting them."""

import json
import re
import ssl
import urllib.parse
import urllib.request
from uuid import UUID

from reconcile import opener


def cursor(value):
    if not isinstance(value, str) or not re.fullmatch(r"0|[1-9][0-9]{0,19}", value) or int(value) >= 2**63:
        raise ValueError("invalid P09 erasure cursor")
    return int(value)


class Endpoint:
    def __init__(self, config):
        addr = urllib.parse.urlsplit(config["origin"])
        if addr.scheme != "https" or not addr.hostname or addr.username or addr.password or addr.path or addr.query or addr.fragment:
            raise ValueError("P09 restore requires a pinned HTTPS origin")
        self.origin = config["origin"]
        tls = ssl.create_default_context(cafile=config["ca"])
        tls.minimum_version = ssl.TLSVersion.TLSv1_3
        tls.load_cert_chain(config["cert"], config["key"])
        self.client = opener(tls)

    def request(self, path, body=None):
        req = urllib.request.Request(self.origin + path, body, headers={"Content-Type": "application/json"})
        try:
            with self.client.open(req, timeout=20) as response:
                raw = response.read(2 * 1024 * 1024 + 1)
                if len(raw) > 2 * 1024 * 1024:
                    raise ValueError("P09 journal or ACK exceeds bounded page size")
                return raw, json.loads(raw)
        except (ValueError, OSError) as exc:
            raise ValueError("P09 erasure transport failed") from exc


def apply_and_recheck(config):
    namespace = str(UUID(config["namespace"]))
    source, target = Endpoint(config["journal"]), Endpoint(config["workload"])
    if source.origin == target.origin:
        raise ValueError("P09 erasure journal and workload must use distinct endpoints")
    high_water = None
    after = cursor(config.get("backupCursor", "0"))
    while high_water is None or after < high_water:
        raw, page = source.request(f"/v1/restore/fences?after={after}")
        after_page = cursor(page["after"])
        through, advertised = cursor(page["through"]), cursor(page["highWater"])
        if high_water is None:
            high_water = advertised
        entries = page["entries"]
        if (page["namespace"] != namespace or after_page != after or high_water != advertised or
                not isinstance(page.get("signature"), str) or not page["signature"] or
                not after <= through <= high_water or (through == after < high_water) or
                len(entries) != through - after or len(entries) > 500 or
                any(cursor(entry["sequence"]) != after + i for i, entry in enumerate(entries, 1))):
            raise ValueError("P09 journal namespace, signature, high water or page order changed")
        _, ack = target.request("/v1/restore/apply", raw)
        progress = cursor(ack["appliedThrough"])
        if (ack["namespace"] != namespace or cursor(ack["highWater"]) != high_water or
                not through <= progress <= high_water or ack["ready"] is not (progress == high_water)):
            raise ValueError("P09 workload did not acknowledge authenticated fences")
        after = through
    _, final = source.request(f"/v1/restore/fences?after={high_water}")
    if (final["namespace"] != namespace or cursor(final["highWater"]) != high_water or
            cursor(final["through"]) != high_water or cursor(final["after"]) != high_water or final["entries"] or
            not final.get("signature")):
        raise ValueError("P09 journal advanced during recovery")
    return {"namespace": namespace, "highWater": str(high_water), "appliedThrough": str(after)}
