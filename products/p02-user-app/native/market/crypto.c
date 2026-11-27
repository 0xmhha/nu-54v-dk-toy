// Compile the same pinned secp256k1 and Keccak code used by the P01 firmware.
#define ENABLE_MODULE_RECOVERY 1
#include "../../../p01-device-firmware/third_party/secp256k1/src/secp256k1.c"
#include "../../../p01-device-firmware/third_party/secp256k1/src/precomputed_ecmult.c"
#include "../../../p01-device-firmware/third_party/secp256k1/src/precomputed_ecmult_gen.c"
#include "../../../p01-device-firmware/third_party/keccak/Keccak-readable-and-compact.c"
