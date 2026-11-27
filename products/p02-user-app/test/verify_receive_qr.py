"""Decode the actual product GIFs using an independent decoder."""
import base64
import io
import json
import sys

from PIL import Image
import zxingcpp

for sample in json.load(sys.stdin):
    image = base64.b64decode(sample["image"].split(",", 1)[1], validate=True)
    codes = zxingcpp.read_barcodes(
        Image.open(io.BytesIO(image)).convert("RGB"),
        formats=zxingcpp.BarcodeFormat.QRCode,
    )
    if len(codes) != 1 or codes[0].text != sample["payload"]:
        raise ValueError(f"QR content mismatch for {sample['name']}")
    print(f"PASS {sample['name']}")
