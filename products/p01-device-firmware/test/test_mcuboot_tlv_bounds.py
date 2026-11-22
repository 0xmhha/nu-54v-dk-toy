"""Check the pinned MCUboot iterator on a small, read-only host fixture."""

import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import mcuboot_input

PRELUDE = """
#include <assert.h>
#include <stdbool.h>
#include <stdint.h>
#include <string.h>

#define BOOT_LOG_DBG(...) ((void)0)
#define IMAGE_TLV_ANY 0xffff
struct image_header { uint16_t ih_protect_tlv_size; };
struct image_tlv { uint16_t it_type, it_len; };
struct image_tlv_info { uint16_t magic, length; };
struct flash_area { struct image_tlv entry; };
struct image_tlv_iter {
    const struct image_header *hdr;
    const struct flash_area *fap;
    uint32_t tlv_off, tlv_end, prot_end;
    uint16_t type;
    bool prot;
};

static unsigned reads;
static int load(const struct flash_area *area, uint32_t off, void *dst, size_t len)
{
    assert(off == 0 && len == sizeof(area->entry));
    ++reads;
    memcpy(dst, &area->entry, len);
    return 0;
}
#define LOAD_IMAGE_DATA(h, area, off, dst, len) load(area, off, dst, len)
"""

TEST = """
int main(void)
{
    const struct flash_area flash = {{42, 2}};
    const struct image_header header = {6};
    const struct image_tlv_iter exact = {&header, &flash, 0, 6, 6, 42, true};
    struct image_tlv_iter iter = exact;
    uint32_t off = UINT32_MAX;
    uint16_t len = UINT16_MAX;

    assert(bootutil_tlv_iter_next(&iter, &off, &len, NULL) == 0);
    assert(reads == 1 && off == 4 && len == 2 && iter.tlv_off == 6);
    assert(bootutil_tlv_iter_next(&iter, &off, &len, NULL) == 1 && reads == 1);

    iter = exact;
    iter.tlv_end = iter.prot_end = 3;
    assert(bootutil_tlv_iter_next(&iter, &off, &len, NULL) == -1 && reads == 1);

    iter = exact;
    iter.tlv_end = iter.prot_end = 5;
    off = UINT32_MAX;
    len = UINT16_MAX;
    assert(bootutil_tlv_iter_next(&iter, &off, &len, NULL) == -1);
    assert(reads == 2 && iter.tlv_off == 0 && off == UINT32_MAX && len == UINT16_MAX);

    iter = exact;
    iter.tlv_off = 6;
    iter.tlv_end = 10;
    assert(bootutil_tlv_iter_next(&iter, &off, &len, NULL) == 1 && reads == 2);
    iter.prot = false;
    assert(bootutil_tlv_iter_next(&iter, &off, &len, NULL) == 1 && reads == 2);

    iter = exact;
    iter.prot = false;
    iter.tlv_end = 5;
    iter.type = 43;
    assert(bootutil_tlv_iter_next(&iter, &off, &len, NULL) == -1 && reads == 3);
}
"""


def main(source: Path) -> None:
    with tempfile.TemporaryDirectory(prefix="nu54-tlv-") as tmp:
        scratch = Path(tmp)
        module = mcuboot_input.prepare(source, scratch)
        text = (module / mcuboot_input.TLV).read_text()
        next_function = re.search(
            r"^int\nbootutil_tlv_iter_next\(.*?\n\}(?=\n\n)", text, re.MULTILINE | re.DOTALL)
        if not next_function:
            raise SystemExit("Pinned MCUboot iterator unavailable")
        unit = scratch / "tlv_host.c"
        unit.write_text(PRELUDE + next_function[0] + TEST)
        binary = scratch / "tlv_host"
        subprocess.run([os.environ.get("CC", "cc"), "-std=c11", "-Wall", "-Wextra", "-Werror",
                        str(unit), "-o", str(binary)], check=True)
        subprocess.run([str(binary)], check=True)
        print("Pinned MCUboot TLV boundary checks passed")


if __name__ == "__main__":
    main(Path(sys.argv[1]))
