# Third-party code

Copied unmodified from upstream at the commits below. Do not edit these files; to update,
copy a new release over them, record the commit here and rerun the host tests.

| Directory | Upstream | Version | License | What is used |
|---|---|---|---|---|
| `secp256k1/` | https://github.com/bitcoin-core/secp256k1 | v0.8.0, commit `6e2c8bc4ecdc6e71dbe7a368f360d8d453ce435d` | MIT (`secp256k1/COPYING`) | Signature parsing, low-s normalization and public-key recovery (`ENABLE_MODULE_RECOVERY`) |
| `keccak/` | https://github.com/XKCP/XKCP, `Standalone/CompactFIPS202/C/Keccak-readable-and-compact.c` | commit `4affab454735d54e78156880b3b44e38dcbf765c` | CC0 (header of the file) | Keccak-256 with the original 0x01 padding |

From libsecp256k1 only the library sources, the public headers `secp256k1.h`,
`secp256k1_recovery.h`, `secp256k1_preallocated.h` and the recovery module are copied; tests,
benchmarks, build files and the other modules are left out.

Build settings (host tests in `test/CMakeLists.txt`; the device build must use the same):
`ENABLE_MODULE_RECOVERY=1`, `ECMULT_WINDOW_SIZE=4`. The precomputed tables are the upstream
files; the window size picks the part that is compiled in.

The device key is not handled here: the TF-M secure partition signs with CRACEN and
`core/src/nu54_sig.c` only normalizes the result and finds `v` (design 3).
