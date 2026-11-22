#include <assert.h>
#include <openssl/sha.h>
#include <string.h>
#include "w27_qualification.h"
#include "w27_qualification_scope.h"

static const char image[] =
    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
static const char other_image[] =
    "1023456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

static void scoped_id(uint8_t *output, const uint8_t *board, const uint8_t *nonce,
                      const char *tag, uint32_t bootconf)
{
    uint8_t input[W27Q_SCOPE_BYTES];
    w27q_scope_input(input, board, nonce, tag, bootconf);
    assert(SHA256(input, sizeof(input), output) == output);
}

int main(void)
{
    uint8_t request[W27Q_REQUEST_BYTES];
    uint8_t response[W27Q_RESPONSE_BYTES];
    uint8_t old_response[112] = {1, 112};
    uint8_t board[W27Q_FACTORY_ID_BYTES] = {0xab, 0xcd, 0xef, 0x11, 0x22, 0x33, 0x44, 0x55};
    uint8_t nonce[W27Q_NONCE_BYTES] = {0x42};
    uint8_t boot[W27Q_BOOT_BYTES];
    uint8_t other_boot[W27Q_BOOT_BYTES];
    const uint32_t bootconf = 0x12345678;

    scoped_id(boot, board, nonce, image, bootconf);
    scoped_id(other_boot, board, nonce, image, bootconf);
    assert(memcmp(boot, other_boot, sizeof(boot)) == 0);
    nonce[0]++;
    scoped_id(other_boot, board, nonce, image, bootconf);
    assert(memcmp(boot, other_boot, sizeof(boot)) != 0);
    nonce[0]--;
    scoped_id(other_boot, board, nonce, other_image, bootconf);
    assert(memcmp(boot, other_boot, sizeof(boot)) != 0);
    scoped_id(other_boot, board, nonce, image, bootconf + 1);
    assert(memcmp(boot, other_boot, sizeof(boot)) != 0);
    board[0]++;
    scoped_id(other_boot, board, nonce, image, bootconf);
    assert(memcmp(boot, other_boot, sizeof(boot)) != 0);
    board[0]--;

    w27q_request(request, image);
    assert(w27q_request_valid(request, sizeof(request), image));
    assert(!w27q_request_valid(request, sizeof(request)-1, image));
    assert(!w27q_request_valid(request, sizeof(request), other_image));
    request[0] = 1;
    assert(!w27q_request_valid(request, sizeof(request), image));
    request[0] = W27Q_VERSION;
    request[2] |= 0x80;
    assert(!w27q_request_valid(request, sizeof(request), image));

    w27q_response(response, boot, W27Q_BOOTCONF_NONDEFAULT);
    assert(w27q_response_valid(response, sizeof(response)));
    assert(response[3] == W27Q_BOOTCONF_NONDEFAULT);
    assert(sizeof(response) == 4 + W27Q_BOOT_BYTES);
    assert(memcmp(response + W27Q_BOOT_OFFSET, boot, sizeof(boot)) == 0);
    assert(!w27q_response_valid(response, sizeof(response)-1));
    assert(!w27q_complete(response, sizeof(response)));
    assert(!w27q_response_valid(old_response, sizeof(old_response)));
    response[0] = 1;
    assert(!w27q_response_valid(response, sizeof(response)));
    response[0] = W27Q_VERSION;
    response[2] |= W27Q_CURRENT_DEBUG;
    assert(!w27q_response_valid(response, sizeof(response)));
    response[2] &= (uint8_t)~W27Q_CURRENT_DEBUG;
    response[3] = 2;
    assert(!w27q_response_valid(response, sizeof(response)));
    response[3] = W27Q_BOOTCONF_DEFAULT;
    assert(w27q_response_valid(response, sizeof(response)));
    memset(response + W27Q_BOOT_OFFSET, 0, W27Q_BOOT_BYTES);
    assert(!w27q_response_valid(response, sizeof(response)));
    memset(response + W27Q_BOOT_OFFSET, 0xff, W27Q_BOOT_BYTES);
    assert(!w27q_response_valid(response, sizeof(response)));
    return 0;
}
