"""Verify settlement, fulfillment and finalized EVM identities without side effects."""

import json
import re
import time
import urllib.error
import urllib.parse
import urllib.request

from postgres import sql, sql_json

LEDGER_CHECKS = """
WITH balances AS (
    SELECT sale_id,
      COALESCE(sum(amount) FILTER (WHERE state IN ('requested','approved')),0) AS reserved,
      COALESCE(sum(amount) FILTER (WHERE state='inflight'),0) AS in_flight,
      COALESCE(sum(amount) FILTER (WHERE state='final'),0) AS refunded
    FROM ledger.refund GROUP BY sale_id
), p AS (
    SELECT payment.*, COALESCE(balances.reserved,0) AS expected_reserved,
      COALESCE(balances.in_flight,0) AS expected_in_flight,
      COALESCE(balances.refunded,0) AS expected_refunded,
      s.origin, {digital} AS digital
    FROM ledger.payment LEFT JOIN balances USING(sale_id)
    JOIN ledger.sale s ON s.id=payment.sale_id
    {invoice_join}
)
SELECT
  (SELECT count(*) FROM p WHERE (reserved,in_flight,refunded) <>
    (expected_reserved,expected_in_flight,expected_refunded)) AS payment_balance_differences,
  (SELECT count(*) FROM p LEFT JOIN ledger.journal j ON j.sale_id=p.sale_id AND j.refund_id IS NULL
    WHERE (j.amount,j.asset_ref,j.evidence_id) IS DISTINCT FROM (p.principal,p.asset_ref,p.evidence_id)) AS payment_journal_differences,
  (SELECT count(*) FROM ledger.refund r JOIN p ON p.sale_id=r.sale_id LEFT JOIN ledger.refund_confirmation c ON c.refund_id=r.id
    LEFT JOIN ledger.journal j ON j.refund_id=r.id
    WHERE (r.state='final' AND (c.refund_id IS NULL OR
      (j.amount,j.asset_ref,j.evidence_id) IS DISTINCT FROM (r.amount,p.asset_ref,c.evidence_id)))
      OR (r.state<>'final' AND (c.refund_id IS NOT NULL OR j.refund_id IS NOT NULL))) AS refund_journal_differences,
  (SELECT count(*) FROM p LEFT JOIN ledger.pickup USING(sale_id)
    WHERE (ledger.pickup.sale_id IS NULL) <> (p.origin='travel' OR p.digital)) AS pickup_differences,
  (SELECT count(*) FROM p LEFT JOIN ledger.fulfillment_outbox e ON e.kind='paid' AND e.sale_id=p.sale_id
    WHERE (e.event_id IS NULL) <> (p.origin='travel' OR p.digital)) AS paid_effect_differences,
  (SELECT count(*) FROM ledger.refund r JOIN p ON p.sale_id=r.sale_id
    LEFT JOIN ledger.fulfillment_outbox e ON e.kind='refunded' AND e.source_ref=r.id
    WHERE r.state='final' AND e.sale_id IS DISTINCT FROM CASE WHEN p.origin<>'travel' AND NOT p.digital THEN r.sale_id ELSE NULL END)
    AS refund_effect_differences,
  (SELECT count(*) FROM (SELECT kind,source_ref FROM ledger.fulfillment_outbox GROUP BY kind,source_ref
    HAVING count(*)>1) duplicates) AS duplicate_business_effects,
  (SELECT count(*) FROM ledger.fulfillment_outbox e FULL JOIN ledger.consumer_receipt a USING(event_id)
    WHERE (e.state='delivered' OR a.event_id IS NOT NULL) AND
      (a.effect_key,a.payload_digest) IS DISTINCT FROM (e.effect_key,e.payload_digest)) AS delivery_receipt_differences,
  (SELECT count(*) FROM ledger.fulfillment_inbox i FULL JOIN ledger.consumer_receipt a USING(event_id)
    WHERE (i.effect_key,i.payload_digest) IS DISTINCT FROM (a.effect_key,a.payload_digest)) AS inbox_receipt_differences,
  (SELECT count(*) FROM ledger.fulfillment_outbox WHERE state IN ('sending','unknown')) AS unresolved_deliveries,
  (SELECT count(*) FROM ledger.relay_attempt WHERE state IN ('signed','unknown','broadcast','observed')) AS unresolved_relays,
  {entitlement_check} AS entitlement_differences
"""

DIGITAL_ENTITLEMENTS = """
(SELECT count(*) FROM p
 LEFT JOIN ledger.invoice i USING (sale_id)
 LEFT JOIN ledger.subscription_entitlement e ON e.invoice_id=i.id
 WHERE (e.invoice_id IS NOT NULL) <> p.digital
    OR (p.digital AND (e.revoked_at IS NOT NULL) <> (p.refunded=p.principal)))
"""

PAID_REFUNDED = """
SELECT stream_id, evidence_id, tx_hash, log_index, inclusion_hash, asset_ref, principal::text AS amount, 'paid' AS kind
FROM ledger.payment
UNION ALL
SELECT c.stream_id, c.evidence_id, c.tx_hash, c.log_index, c.inclusion_hash, p.asset_ref, r.amount::text, 'refunded'
FROM ledger.refund_confirmation c JOIN ledger.refund r ON r.id=c.refund_id JOIN ledger.payment p ON p.sale_id=r.sale_id
"""

FINALIZED = """
SELECT r.stream_id, r.id AS evidence_id, r.tx_hash, r.log_index, r.inclusion_hash, r.inclusion_height, r.contract_ref,
       r.asset_ref, r.amount::text, r.kind, r.anchor_hash, r.anchor_height, r.merchant_key, r.order_key, r.refund_key,
       r.payer_ref, profile.version
FROM chainview.settlement_receipt r JOIN chainview.deployment_profile profile USING (stream_id)
WHERE r.kind IN ('paid','refunded')
"""

CHECKPOINTS = """
SELECT t.stream_id, t.revision, s.chain_ref, s.manifest_ref, s.contract_ref, g.finalized_height,
       g.finalized_hash, g.state, g.base_height, g.tip_height, profile.start_height, profile.start_hash, profile.version,
       profile.asset_ref
FROM chainview.read_target t
JOIN chainview.stream s ON s.id=t.stream_id
JOIN chainview.generation g ON g.id=t.generation_id
LEFT JOIN chainview.deployment_profile profile ON profile.stream_id=t.stream_id
"""


# Event topic hashes from the versioned IPaymentSettlement and SettlementV2 ABI.
V1_PAID = "0xeef4300dbd9217414481ce2ade0c4791c3b47c75b0c0d4b6bbe5fbb05260195f"
V2_PAID = "0x8ef858e4a5aa54b21ac8e7de48a10ab47b1192319da9540a960cc449fd68757f"
V2_REFUND = "0x5c026d6872cb44d83c4429c3604d56938b71b245f403a2555d7d6ddcf993aef7"
SETTLEMENT_TOPICS = {1: [V1_PAID], 2: [V2_PAID, V2_REFUND]}


def amount(log_word):
    return str(int(log_word, 16))


def address(log_word):
    return log_word[:2] + log_word[-40:]


def log_matches(event, log):
    topics, data = log["topics"], log["data"]
    if len(topics) != 4 or not re.fullmatch(r"0x(?:[a-f0-9]{64})+", data):
        return False
    words = ["0x" + data[i:i + 64] for i in range(2, len(data), 64)]
    if event["version"] == 1 and event["kind"] == "paid" and topics[0] == V1_PAID and len(words) == 2:
        return (address(topics[1]), topics[2], address(topics[3]), amount(words[0])) == (
            event["merchant_key"], event["order_key"], event["payer_ref"], event["amount"])
    if event["version"] != 2 or topics[1:3] != [event["merchant_key"], event["order_key"]]:
        return False
    if event["kind"] == "paid" and topics[0] == V2_PAID and len(words) == 7:
        return (address(topics[3]), address(words[1]), amount(words[2])) == (
            event["payer_ref"], event["asset_ref"], event["amount"])
    if event["kind"] == "refunded" and topics[0] == V2_REFUND and len(words) == 4:
        return (topics[3], address(words[0]), address(words[1]), amount(words[2])) == (
            event["refund_key"], event["payer_ref"], event["asset_ref"], event["amount"])
    return False


class RestrictedClient(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise ValueError("service attempted redirect")


def opener(context=None):
    handlers = [urllib.request.ProxyHandler({}), RestrictedClient()]
    if context:
        handlers.append(urllib.request.HTTPSHandler(context=context))
    return urllib.request.build_opener(*handlers)


def rpc_endpoint(endpoint, synthetic):
    url = urllib.parse.urlsplit(endpoint)
    allowed = url.scheme == "https" or (synthetic and url.scheme == "http" and url.hostname == "127.0.0.1")
    if not allowed or url.username or url.password or url.query or url.fragment or not url.hostname:
        raise ValueError("RPC requires an independently configured HTTPS endpoint or synthetic loopback")
    return endpoint


class Chain:
    def __init__(self, endpoint, synthetic, deadline):
        self.endpoint = rpc_endpoint(endpoint, synthetic)
        self.deadline = deadline
        self.client = opener()
        self.id = 0
        self.cached = {}

    def call(self, method, *params):
        key = (method, json.dumps(params, sort_keys=True))
        if key not in self.cached:
            self.id += 1
            req = urllib.request.Request(self.endpoint, json.dumps({
                "jsonrpc": "2.0", "id": self.id, "method": method, "params": params,
            }).encode(), headers={"Content-Type": "application/json"})
            remaining = self.deadline - time.monotonic()
            if remaining <= 0:
                raise ValueError("finalized receipt verification exceeded the enrolled RTO")
            try:
                with self.client.open(req, timeout=min(5, remaining)) as response:
                    result = json.loads(response.read(4 * 1024 * 1024))
            except (ValueError, OSError) as exc:
                raise ValueError(f"trusted RPC {method} failed") from exc
            if result.get("id") != self.id or result.get("jsonrpc") != "2.0" or result.get("result") is None:
                raise ValueError(f"trusted RPC has no {method} result")
            self.cached[key] = result["result"]
        return self.cached[key]

    def block(self, number):
        return self.call("eth_getBlockByNumber", hex(number) if isinstance(number, int) else number, False)

    def receipt_matches(self, event):
        receipt = self.call("eth_getTransactionReceipt", event["tx_hash"])
        return (receipt["status"] == "0x1" and receipt["transactionHash"] == event["tx_hash"] and
                receipt["blockHash"] == self.block(event["inclusion_height"])["hash"] == event["inclusion_hash"] and
                any(int(log["logIndex"], 16) == event["log_index"] and log["address"] == event["contract_ref"] and
                    log["blockHash"] == event["inclusion_hash"] and not log.get("removed", False) and log_matches(event, log)
                    for log in receipt["logs"]))

    def settlement_keys(self, target, finalized_height):
        start = target["start_height"]
        if finalized_height < start:
            raise ValueError("finalized RPC coverage predates the deployment")
        keys = set()
        for height in range(start, finalized_height + 1, 1000):
            logs = self.call("eth_getLogs", {"fromBlock": hex(height), "toBlock": hex(min(height + 999, finalized_height)),
                "address": target["contract_ref"], "topics": [SETTLEMENT_TOPICS[target["version"]]]})
            for log in logs:
                key = (target["stream_id"], log["transactionHash"], int(log["logIndex"], 16))
                if log.get("removed", False) or key in keys:
                    raise ValueError("RPC settlement range returned a removed or duplicate log")
                keys.add(key)
        return keys


def reconcile(quarantine, reference, chain):
    db = quarantine.env
    digital = sql(db, "p05", "SELECT to_regclass('ledger.subscription_entitlement') IS NOT NULL;") == "t"
    ledger_sql = LEDGER_CHECKS.format(
        digital="COALESCE(i.fulfillment_kind='digital',false)" if digital else "false",
        invoice_join="LEFT JOIN ledger.invoice i ON i.sale_id=payment.sale_id AND s.origin='subscription'" if digital else "",
        entitlement_check=DIGITAL_ENTITLEMENTS if digital else "0",
    )
    ledger = sql_json(db, "p05", ledger_sql)[0]
    p05 = sql_json(db, "p05", PAID_REFUNDED)
    p07 = sql_json(db, "p07", FINALIZED)
    checkpoints = sql_json(db, "p07", CHECKPOINTS)
    canonical = {row["evidence_id"]: row for row in p07}
    differences = 0
    for row in p05:
        found = canonical.get(row["evidence_id"])
        if found is None or any(found[key] != value for key, value in row.items()):
            differences += 1
    used = {row["evidence_id"] for row in p05}
    missing_ledger = sum(row["evidence_id"] not in used and row["version"] == 2 for row in p07)
    finalized = chain.block("finalized")
    chain_ref = int(chain.call("eth_chainId"), 16)
    checkpoint_differences = finality_differences = 0
    on_chain = set()
    for target in checkpoints:
        number = target["finalized_height"]
        valid = (target["state"] == "ready" and target["chain_ref"] == str(chain_ref) and number is not None and
                 number <= int(finalized["number"], 16) and target["start_height"] is not None and
                 target["version"] in SETTLEMENT_TOPICS and
                 chain.block(number)["hash"] == target["finalized_hash"] and
                 chain.block(target["start_height"])["hash"] == target["start_hash"])
        checkpoint_differences += not valid
        if valid:
            on_chain.update(chain.settlement_keys(target, int(finalized["number"], 16)))
    for event in p07:
        finality_differences += not (chain.receipt_matches(event) and
            chain.block(event["anchor_height"])["hash"] == event["anchor_hash"] and
            event["inclusion_height"] <= event["anchor_height"] <= int(finalized["number"], 16))
    indexed = {tuple(row[k] for k in ("stream_id", "tx_hash", "log_index")) for row in p07}
    missing_chain_events, orphaned_chain_events = len(on_chain - indexed), len(indexed - on_chain)
    duplicate_keys = len(p07) - len(indexed)
    open_incidents = int(sql(db, "p07", "SELECT count(*) FROM chainview.incident WHERE resolved_at IS NULL;"))
    deltas = quarantine.reconcile(reference)
    return {"inventoryDifferences": deltas, "ledger": ledger, "paidRefunded": len(p05), "finalizedEvents": len(p07),
            "missingLedgerEvents": missing_ledger, "differentLedgerEvents": differences, "duplicateEventKeys": duplicate_keys,
            "rpcReceiptDifferences": finality_differences, "checkpointDifferences": checkpoint_differences,
            "missingChainEvents": missing_chain_events, "orphanedChainEvents": orphaned_chain_events,
            "openFinalityIncidents": open_incidents, "checkpoints": checkpoints,
            "matched": bool(checkpoints) and not (any(deltas.values()) or any(ledger.values()) or duplicate_keys or
                differences or missing_ledger or finality_differences or checkpoint_differences or open_incidents
                or missing_chain_events or orphaned_chain_events)}
