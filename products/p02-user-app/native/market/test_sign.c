// Cross-runtime test adapter. The scalar here belongs only to the disposable contract fixture.
#include "wallet.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

int main(int argc, char **argv)
{
    uint8_t seed[32] = {0}, entropy[32] = {0}, scope[LAB_SCOPE_LENGTH], target[20];
    uint8_t data[LAB_QUOTE_LENGTH], quote[LAB_QUOTE_LENGTH], raw[LAB_TX_LENGTH];
    char encoded[2 * LAB_TX_LENGTH + 3] = "0x";
    size_t out_length = 0;
    if (argc != 8 || !lab_decode(argv[1], strlen(argv[1]), scope, sizeof scope) ||
        !lab_decode(argv[2], strlen(argv[2]), target, sizeof target) ||
        !lab_decode(argv[6], strlen(argv[6]), quote, sizeof quote)) return 2;
    size_t length = strlen(argv[5]) / 2;
    if ((length != 68 && length != LAB_BOUND_APPROVE_LENGTH && length != LAB_QUOTE_LENGTH) || !lab_decode(argv[5], strlen(argv[5]), data, length)) return 2;
    seed[31] = 106;
    entropy[31] = 1;
    if (!lab_sign(seed, entropy, scope, target, strtoull(argv[3], NULL, 10), strtoull(argv[4], NULL, 10),
            strtoull(argv[7], NULL, 10), data, length, quote, raw, &out_length)) return 2;
    lab_encode(raw, out_length, encoded + 2);
    puts(encoded);
    return 0;
}
