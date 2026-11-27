"""Independent byte-oriented review for the bounded laboratory transaction profiles."""
import hashlib

BITCOIN = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
XRPL = "rpshnaf39wBUDNEGHJKLM4PQRST7VWXYZ2bcdeCg65jkm8oFqi1tuvAxyz"
TOKEN = bytes.fromhex("06ddf6e1d765a193d9cbe146ceeb79ac1cb485ed5f5b37913a8cf5857eff00a9")


class Reader:
    def __init__(self, raw):
        self.raw = raw
        self.index = 0

    def take(self, count):
        end = self.index + count
        if end > len(self.raw):
            raise ValueError("truncated review")
        data = self.raw[self.index:end]
        self.index = end
        return data

    def expect(self, tag):
        if self.take(len(tag)) != tag:
            raise ValueError("unsupported review field")

    def byte(self):
        return self.take(1)[0]

    def peek(self):
        return self.raw[self.index] if self.index < len(self.raw) else -1

    def complete(self):
        if self.index != len(self.raw):
            raise ValueError("unexpected review suffix")

    def vector(self):
        first = self.byte()
        if first == 253:
            length = little(self.take(2))
            if length < 253:
                raise ValueError("noncanonical vector count")
        elif first > 253:
            raise ValueError("unsupported vector count")
        else:
            length = first
        return self.take(length)


def big(data):
    return int.from_bytes(data, "big")


def little(data):
    return int.from_bytes(data, "little")


def d256(data):
    return hashlib.sha256(hashlib.sha256(data).digest()).digest()


def b58(raw, alphabet=BITCOIN):
    value = big(raw)
    tail = ""
    while value:
        value, index = divmod(value, 58)
        tail = alphabet[index] + tail
    return alphabet[0] * (len(raw) - len(raw.lstrip(b"\x00"))) + tail


def classic(account):
    raw = b"\x00" + account
    return b58(raw + d256(raw)[:4], XRPL)


def segwit(script, hrp="bcrt"):
    if len(script) not in (22, 34) or script[:2] != bytes([0, len(script) - 2]):
        raise ValueError("unsupported Bitcoin output script")
    alphabet = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
    word = "".join(f"{byte:08b}" for byte in script[2:])
    word += "0" * (-len(word) % 5)
    program = [0] + [int(word[i:i+5], 2) for i in range(0, len(word), 5)]
    checksum = 1
    for value in [*(ord(c) >> 5 for c in hrp), 0, *(ord(c) & 31 for c in hrp), *program, *([0] * 6)]:
        top = checksum >> 25
        checksum = ((checksum & 0x1ffffff) << 5) ^ value
        for bit, generator in enumerate((0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3)):
            if top & 1 << bit:
                checksum ^= generator
    checksum ^= 1
    return hrp + "1" + "".join(alphabet[i] for i in program + [(checksum >> shift) & 31 for shift in range(25, -1, -5)])


def psbt_map(reader, maximum=8):
    fields = {}
    while key := reader.vector():
        if key in fields or len(fields) >= maximum:
            raise ValueError("unsupported PSBT map")
        fields[key] = reader.vector()
    return fields


def transaction(raw):
    tx = Reader(raw)
    tx.take(4)
    n = tx.byte()
    if not 1 <= n <= 4:
        raise ValueError("input count outside profile")
    inputs = []
    for _ in range(n):
        outpoint = tx.take(36)
        tx.vector()
        inputs.append((outpoint, little(tx.take(4))))
    count = tx.byte()
    if not 1 <= count <= 4:
        raise ValueError("output count outside profile")
    outputs = [(little(tx.take(8)), tx.vector()) for _ in range(count)]
    locktime = little(tx.take(4))
    tx.complete()
    return inputs, outputs, locktime


def review_btc(raw, commissioned=False):
    psbt = Reader(raw)
    psbt.expect(b"psbt\xff")
    inputs, outputs, locktime = transaction(psbt_map(psbt)[b"\x00"])
    previous = []
    for point, sequence in inputs:
        parent = psbt_map(psbt)[b"\x00"]
        if d256(parent) != point[:32]:
            raise ValueError("previous transaction ID mismatch")
        _, coins, _ = transaction(parent)
        index = little(point[32:])
        previous.append({"txid": point[:32][::-1].hex(), "vout": index, "satoshis": coins[index][0],
                         "sequence": sequence})
    reviewed = []
    for value, script in outputs:
        fields = psbt_map(psbt)
        reviewed.append({"address": segwit(script, "tb" if commissioned else "bcrt"), "satoshis": value,
                         "changePathClaim": any(len(key) == 34 and key[0] == 2 for key in fields)})
    psbt.complete()
    fee = sum(coin["satoshis"] for coin in previous) - sum(value for value, _ in outputs)
    if fee < 0:
        raise ValueError("negative fee")
    return {"chain": "Bitcoin Signet" if commissioned else "Bitcoin regtest", "allOutputs": reviewed, "feeSatoshis": fee, "inputs": previous,
            "replaceable": any(row["sequence"] == 0xfffffffd for row in previous), "locktime": locktime}


def review_sol(raw, context):
    msg = Reader(raw)
    version = 0 if msg.peek() == 0x80 else None
    if version == 0:
        msg.take(1)
    msg.expect(b"\x01\x00")
    readonly, count = msg.byte(), msg.byte()
    if (readonly, count) not in ((1, 3), (2, 5)):
        raise ValueError("Solana signer or account profile")
    keys = [msg.take(32) for _ in range(count)]
    blockhash = msg.take(32)
    if blockhash != context[:32]:
        raise ValueError("Solana blockhash review mismatch")
    msg.expect(b"\x01")
    if count == 3:
        msg.expect(b"\x02\x02\x00\x01\x0c\x02\x00\x00\x00")
        if any(keys[2]):
            raise ValueError("Solana System Program mismatch")
        detail = {"recipient": b58(keys[1]), "asset": "SOL", "amountLamports": little(msg.take(8))}
    else:
        msg.expect(b"\x04\x04\x01\x03\x02\x00\x0a\x0c")
        if keys[4] != TOKEN or keys[3] != context[56:88]:
            raise ValueError("SPL program or mint mismatch")
        detail = {"recipientTokenAccount": b58(keys[2]), "sourceTokenAccount": b58(keys[1]),
                  "mint": b58(keys[3]), "amountBaseUnits": little(msg.take(8)), "mintDecimals": msg.byte()}
    if version == 0:
        msg.expect(b"\x00")
    msg.complete()
    return {"chain": "Solana local validator", "version": version if version is not None else "legacy",
            "feePayer": b58(keys[0]), "blockhash": b58(blockhash), "height": big(context[32:40]),
            "lastValidBlockHeight": big(context[40:48]), "feeLamports": big(context[48:56]), **detail}


def xrp_amount(reader):
    value = big(reader.take(8))
    if value >> 62 == 1:
        return {"drops": value & ((1 << 62) - 1)}
    if value >> 62 != 3:
        raise ValueError("unsupported XRP Ledger amount")
    raw = reader.take(20)
    currency = raw[12:15].decode("ascii") if not any(raw[:12] + raw[15:]) else raw.hex()
    exponent = ((value >> 54) & 255) - 97
    return {"currency": currency, "issuer": classic(reader.take(20)),
            "value": str(value & ((1 << 54) - 1)) + "e" + str(exponent)}


def review_xrpl(raw):
    tx = Reader(raw)
    tx.expect(b"\x12\x00\x00")
    if tx.peek() == 0x22:
        if big(tx.take(5)[1:]) not in (0, 0x80000000):
            raise ValueError("unsupported XRP Ledger flags")
    tx.expect(b"\x24")
    seq = big(tx.take(4))
    tag = None
    if tx.peek() == 0x2e:
        tag = big(tx.take(5)[1:])
    tx.expect(b"\x20\x1b")
    last = big(tx.take(4))
    ticket = None
    if not seq:
        tx.expect(b"\x20\x29")
        ticket = big(tx.take(4))
    tx.expect(b"\x61")
    amount = xrp_amount(tx)
    tx.expect(b"\x68")
    fee = xrp_amount(tx)["drops"]
    if tx.peek() == 0x69:
        tx.take(1)
        if xrp_amount(tx) != amount:
            raise ValueError("widened XRP Ledger send maximum")
    tx.expect(b"\x73\x21")
    tx.take(33)
    tx.expect(b"\x81\x14")
    account = classic(tx.take(20))
    tx.expect(b"\x83\x14")
    destination = classic(tx.take(20))
    tx.complete()
    return {"chain": "XRP Ledger standalone", "account": account, "destination": destination,
            "destinationTag": tag, "amount": amount, "feeDrops": fee, "sequence": seq, "ticket": ticket,
            "lastLedgerSequence": last}


def decode(grant):
    chain = grant[1]
    raw = grant[217:]
    commissioned = grant[2] == 2
    if chain == 1:
        review = review_btc(raw, commissioned)
        if grant[23] == 1:
            review = dict(review, replaces=grant[24:56].hex(), previousFeeSatoshis=big(grant[56:64]))
    elif chain == 2:
        review = review_sol(raw, grant[23:113])
    elif chain == 3:
        review = review_xrpl(raw)
    else:
        raise ValueError("unknown wallet chain")
    if commissioned and chain in (2, 3):
        review["chain"] = {2: "Solana Devnet", 3: "XRP Ledger Testnet"}[chain]
    return dict(review, accountNumber=big(grant[3:7]) + 1, networkAnchorHash=grant[113:145].hex(),
                rawSha256=hashlib.sha256(raw).hexdigest())


def bip48_input_count(context):
    raw = context[b"\x04"]
    if len(raw) != 1 or raw[0] not in (1, 2):
        raise ValueError("unsupported BIP48 input count")
    return raw[0]


def bip48_original_id(raw):
    psbt = Reader(raw)
    psbt.expect(b"psbt\xff")
    context = psbt_map(psbt, 15)
    input_count = bip48_input_count(context)
    output_count = little(context[b"\x05"])
    if not 1 <= output_count <= 4:
        raise ValueError("unsupported BIP48 output count")
    original = context[b"\x02"] + bytes((input_count,))
    for _ in range(input_count):
        fields = psbt_map(psbt, 15)
        original += fields[b"\x0e"] + fields[b"\x0f"] + b"\x00" + fields[b"\x10"]
    original += bytes((output_count,))
    for _ in range(output_count):
        output = psbt_map(psbt, 15)
        script = output[b"\x04"]
        original += output[b"\x03"] + bytes((len(script),)) + script
    psbt.complete()
    return d256(original + context.get(b"\x03", bytes(4)))[::-1].hex()


def bip48_prevout(fields):
    txid, vout = fields[b"\x0e"], little(fields[b"\x0f"])
    prev = fields[b"\x00"]
    if d256(prev) != txid:
        raise ValueError("unverified previous transaction")
    _, coins, _ = transaction(prev)
    value, script = coins[vout]
    return txid[::-1].hex(), vout, value, script


def bip48_path(fields, kind, policy, program, derive, change=False):
    origins = [(key, value) for key, value in fields.items() if key[0] == kind]
    account = big(policy[3:7])
    fingerprints = {policy[slot:slot+4] for slot in (17, 86, 155)}
    if (len(origins) != 3 or {value[:4] for _, value in origins} != fingerprints or
        any(len(key) != 34 or len(value) != 28 or
            tuple(little(value[i:i+4]) for i in (4, 8, 12, 16)) !=
                (0x80000030, 0x80000001, 0x80000000 + account, 0x80000002)
            for key, value in origins)):
        raise ValueError("prevout or change is outside the registered origins")
    paths = {(little(value[20:24]), little(value[24:])) for _, value in origins}
    if len(paths) != 1:
        raise ValueError("cosigner branches disagree")
    branch, index = paths.pop()
    if branch not in (0, 1) or index > 9999 or (change and branch != 1):
        raise ValueError("invalid registered branch")
    keys = sorted(key[1:] for key, _ in origins)
    witness = b"\x52" + b"".join(b"\x21" + key for key in keys) + b"\x53\xae"
    if (len(set(keys)) != 3 or fields.get(b"\x01" if change else b"\x05") != witness or
        b"\x00\x20" + hashlib.sha256(witness).digest() != program or derive(branch, index) != program):
        raise ValueError("script differs from the registered descriptor")
    return f"m/48'/1'/{account}'/2'/{branch}/{index}"


def review_bip48(grant, policy=None, derive=None):
    psbt = Reader(grant[217:])
    psbt.expect(b"psbt\xff")
    global_map = psbt_map(psbt, 15)
    if global_map[b"\xfb"] != b"\x02\x00\x00\x00":
        raise ValueError("unsupported BIP48 PSBT format")
    input_maps = [psbt_map(psbt, 15) for _ in range(bip48_input_count(global_map))]
    inputs = []
    for fields in input_maps:
        txid, vout, value, script = bip48_prevout(fields)
        if b"\x01" in fields and fields[b"\x01"] != value.to_bytes(8, "little") + bytes([len(script)]) + script:
            raise ValueError("claimed witness UTXO differs from its previous transaction")
        if fields.get(b"\x03", b"\x01\x00\x00\x00") != b"\x01\x00\x00\x00":
            raise ValueError("unsupported sighash")
        path = bip48_path(fields, 6, policy, script, derive) if derive else None
        inputs.append({"outpoint":txid + ":" + str(vout),"satoshis":value, "scriptPubKey":script.hex(),"path":path,
                       "counterpartSignatures":sum(key[0] == 2 for key in fields)})
    if len({row["outpoint"] for row in inputs}) != len(inputs):
        raise ValueError("duplicate BIP48 prevout")
    outputs = []
    count = little(global_map[b"\x05"])
    if not 1 <= count <= 4:
        raise ValueError("unsupported BIP48 output count")
    for _ in range(count):
        fields = psbt_map(psbt, 15)
        script = fields[b"\x04"]
        change = any(key[0] in (1, 2) for key in fields)
        path = bip48_path(fields, 2, policy, script, derive, change=True) if change and derive else None
        outputs.append({"address":segwit(script, "tb" if grant[2] == 2 else "bcrt"),"scriptPubKey":script.hex(),
                        "satoshis":little(fields[b"\x03"]), "changePathClaim":change, "verifiedChangePath":path})
    psbt.complete()
    input_total = sum(row["satoshis"] for row in inputs)
    output_total = sum(row["satoshis"] for row in outputs)
    fee = input_total - output_total
    if fee < 0 or fee > big(grant[15:23]):
        raise ValueError("BIP48 fee exceeds signed cap")
    base = 10 + 41*len(inputs) + sum(9+len(bytes.fromhex(row["scriptPubKey"])) for row in outputs)
    # The marker/flag and four witness items use at most 254 bytes per input with low-S signatures.
    maximum_vsize = (4*base + 2 + 254*len(inputs) + 3) // 4
    rate = 100*fee//maximum_vsize
    return {"chain":"Bitcoin Signet" if grant[2] == 2 else "Bitcoin regtest", "threshold":"2 of 3",
            "policy":grant[23:55].hex(),"account":big(grant[3:7]),"inputs":inputs,"feeSatoshis":fee,"allOutputs":outputs,
            "inputTotal":input_total,"outputTotal":output_total,"sighash":"ALL",
            "maximumSignedVbytes":maximum_vsize,"minimumFeeRateSatPerVbyte":f"{rate//100}.{rate%100:02d}"}
