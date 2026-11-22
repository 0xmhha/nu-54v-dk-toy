"""Check the built MCUboot serial-recovery layout and interrupted application images."""

from pathlib import Path
import re
import struct
import sys
import tempfile


def partition(dts: str, name: str) -> tuple[int, int]:
    match = re.search(rf"\b{name}: partition@\w+\s*\{{[^}}]+reg = <\s*(0x\w+)\s+(0x\w+)\s*>", dts)
    if not match:
        raise ValueError(f"Cannot identify release partition {name}")
    start, size = (int(value, 16) for value in match.groups())
    return start, start + size


def security_counter(data: bytes, image) -> int | None:
    _, _, header_size, protected_size, body_size = struct.unpack_from("<IIHHI", data)
    if not protected_size:
        return None
    offset = header_size + body_size
    magic, size = struct.unpack_from("<HH", data, offset)
    if magic != image.TLV_PROT_INFO_MAGIC or size != protected_size:
        raise ValueError("Invalid signed image protected TLV bounds")
    end = offset + size
    offset += 4
    counters = []
    while offset < end:
        kind, _, length = struct.unpack_from("<BBH", data, offset)
        offset += 4
        if offset + length > end:
            raise ValueError("Invalid signed image protected TLV length")
        if kind == image.TLV_VALUES["SEC_CNT"]:
            if length != 4:
                raise ValueError("Invalid image security counter size")
            counters.append(struct.unpack_from("<I", data, offset)[0])
        offset += length
    if len(counters) > 1:
        raise ValueError("Duplicate signed image security counters")
    return counters[0] if counters else None


def main() -> int:
    imgtool, public, build_root, expected_counter = sys.argv[1:]
    counter = int(expected_counter)
    sys.path.insert(0, str(Path(imgtool).parent))
    from imgtool import image, keys
    from intelhex import IntelHex

    key = keys.load(public)
    if type(key) is not keys.Ed25519Public:
        sys.exit("Recovery verification requires a public Ed25519 key")
    image.ALLOWED_KEY_SHA.setdefault(keys.Ed25519Public, image.ALLOWED_KEY_SHA[keys.Ed25519])
    root = Path(build_root)
    boot, app = root / "mcuboot/zephyr", root / "app/zephyr"
    boot_config = set((boot / ".config").read_text().splitlines())
    slot_mode = "CONFIG_BOOT_SWAP_USING_MOVE=y" if counter else "CONFIG_SINGLE_APPLICATION_SLOT=y"
    boot_transport = "# CONFIG_MCUBOOT_SERIAL is not set" if counter else "CONFIG_MCUBOOT_SERIAL=y"
    if not {"CONFIG_BOOT_VALIDATE_SLOT0=y", boot_transport, slot_mode} <= boot_config:
        sys.exit("Recovery build must validate the primary slot and match the requested serial update mode")
    dts = (boot / "zephyr.dts").read_text()
    slot_start, slot_end = partition(dts, "slot0_partition")
    storage_start, _ = partition(dts, "storage_partition")
    if slot_end > storage_start:
        sys.exit("Application update partition overlaps protected settings")
    segments = IntelHex(str(app / "zephyr.signed.hex")).segments()
    if not segments or segments[0][0] != slot_start or any(start < slot_start or end > slot_end for start, end in segments):
        sys.exit("Signed application HEX is outside its update partition")

    artifact = app / "zephyr.signed.bin"
    image_bytes = artifact.read_bytes()
    if counter:
        app_config = set((app / ".config").read_text().splitlines())
        if not {"CONFIG_NU54_COUNTED_UPDATE=y", "CONFIG_MCUMGR_TRANSPORT_UART=y",
                "CONFIG_MCUMGR_SMP_COMMAND_STATUS_HOOKS=y", "CONFIG_MCUMGR_GRP_IMG=y"} <= app_config:
            sys.exit("Missing counted application staging and command authorization")
        update_start, update_end = partition(dts, "slot1_partition")
        if update_start < slot_end or update_end > storage_start or len(image_bytes) >= update_end - update_start:
            sys.exit("Signed update does not fit into a separate settings-preserving staging slot")
    hardware_counter = "CONFIG_MCUBOOT_HW_DOWNGRADE_PREVENTION=y" in boot_config
    if hardware_counter != bool(counter) or security_counter(image_bytes, image) != (counter or None):
        sys.exit("MCUboot anti-rollback configuration or signed image security counter differs from the build request")
    with tempfile.TemporaryDirectory() as temp:
        interrupted = Path(temp) / "interrupted.bin"
        for cut in (1, 32, len(image_bytes) // 2, len(image_bytes) - 1):
            interrupted.write_bytes(image_bytes[:cut])
            try:
                result, *_ = image.Image.verify(interrupted, key)
            except (struct.error, ValueError, IndexError):
                continue
            if result == image.VerifyResult.OK:
                sys.exit(f"Signature accepted an interrupted update at byte {cut}")
    print("Recovery evidence: verified primary slot, signed HEX inside app partition, four interrupted writes rejected")
    if counter:
        print(f"Verified protected image counter {counter} and hardware anti-rollback configuration")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
