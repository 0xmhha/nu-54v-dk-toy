#ifndef W27_QUALIFICATION_SCOPE_H
#define W27_QUALIFICATION_SCOPE_H

#include "w27_qualification.h"

/* Used by the secure partition and host tests. */
#define W27Q_FACTORY_ID_BYTES 8
#define W27Q_NONCE_BYTES 32
#define W27Q_SCOPE_DOMAIN "nu54/w27/boot-observation/v2"
#define W27Q_SCOPE_BYTES (sizeof(W27Q_SCOPE_DOMAIN) + W27Q_FACTORY_ID_BYTES +                          W27Q_IMAGE_BYTES + 4 + W27Q_NONCE_BYTES)

static inline void w27q_scope_input(uint8_t *input, const uint8_t *board, const uint8_t *nonce,
                                    const char *image, uint32_t bootconf)
{
    memcpy(input, W27Q_SCOPE_DOMAIN, sizeof(W27Q_SCOPE_DOMAIN));
    input += sizeof(W27Q_SCOPE_DOMAIN);
    memcpy(input, board, W27Q_FACTORY_ID_BYTES);
    input += W27Q_FACTORY_ID_BYTES;
    memcpy(input, image, W27Q_IMAGE_BYTES);
    input += W27Q_IMAGE_BYTES;
    for (size_t i = 0; i < 4; ++i) {
        input[i] = (uint8_t)(bootconf >> (8 * i));
    }
    memcpy(input + 4, nonce, W27Q_NONCE_BYTES);
}

#endif
