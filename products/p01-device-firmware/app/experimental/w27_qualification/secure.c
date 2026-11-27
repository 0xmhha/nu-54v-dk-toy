#include <nrfx.h>
#include <psa/crypto.h>
#include <psa/service.h>
#include <hal/nrf_ficr.h>
#include "psa_manifest/partition.h"
#include "w27_qualification.h"
#include "w27_qualification_scope.h"
#include "w27_qualification_image.h"

/* TF-M reinitializes this private partition state at each secure boot. */
static uint8_t boot[W27Q_BOOT_BYTES];
static uint32_t boot_configuration;

static bool boot_identifier(const uint8_t *board, uint32_t bootconf)
{
    if (w27q_identifier_present(boot, sizeof(boot))) {
        return boot_configuration == bootconf;
    }
    uint8_t input[W27Q_SCOPE_BYTES];
    uint8_t nonce[W27Q_NONCE_BYTES];
    size_t output_len = 0;
    if (psa_generate_random(nonce, sizeof(nonce)) != PSA_SUCCESS ||
        !w27q_identifier_present(nonce, sizeof(nonce))) {
        return false;
    }
    w27q_scope_input(input, board, nonce, NU54_W27Q_IMAGE, bootconf);
    if (psa_hash_compute(PSA_ALG_SHA_256, input, sizeof(input), boot, sizeof(boot), &output_len) != PSA_SUCCESS ||
        output_len != sizeof(boot)) {
        memset(boot, 0, sizeof(boot));
        return false;
    }
    boot_configuration = bootconf;
    return w27q_identifier_present(boot, sizeof(boot));
}

static psa_status_t snapshot(const psa_msg_t *msg)
{
    uint8_t request[W27Q_REQUEST_BYTES];
    uint8_t response[W27Q_RESPONSE_BYTES];
    if (msg->in_size[0] != sizeof(request) || msg->out_size[0] != sizeof(response) ||
        msg->in_size[1] || msg->in_size[2] || msg->in_size[3] ||
        msg->out_size[1] || msg->out_size[2] || msg->out_size[3] ||
        psa_read(msg->handle, 0, request, sizeof(request)) != sizeof(request) ||
        !w27q_request_valid(request, sizeof(request), NU54_W27Q_IMAGE)) {
        return PSA_ERROR_INVALID_ARGUMENT;
    }
    uint8_t board[W27Q_FACTORY_ID_BYTES];
    for (size_t i = 0; i < sizeof(board) / 4; ++i) {
        uint32_t id = nrf_ficr_deviceid_get(NRF_FICR, i);
        for (size_t j = 0; j < 4; ++j) {
            board[4 * i + j] = (uint8_t)(id >> (8 * j));
        }
    }

    uint32_t bootconf = NRF_UICR_S->BOOTCONF;
    if (!w27q_identifier_present(board, sizeof(board)) || !boot_identifier(board, bootconf)) {
        return PSA_ERROR_SERVICE_FAILURE;
    }
    uint8_t bootconf_state = bootconf == UICR_BOOTCONF_ResetValue ?
                            W27Q_BOOTCONF_DEFAULT : W27Q_BOOTCONF_NONDEFAULT;
    w27q_response(response, boot, bootconf_state);
    psa_write(msg->handle, 0, response, sizeof(response));
    return PSA_SUCCESS;
}

void w27q_partition_main(void)
{
    for (;;) {
        psa_signal_t signals = psa_wait(PSA_WAIT_ANY, PSA_BLOCK);
        psa_msg_t msg;
        if (!(signals & W27_QUALIFICATION_SNAPSHOT_SIGNAL) ||
            psa_get(W27_QUALIFICATION_SNAPSHOT_SIGNAL, &msg) != PSA_SUCCESS) {
            psa_panic();
        }
        psa_status_t status = PSA_ERROR_NOT_SUPPORTED;
        switch (msg.type) {
        case PSA_IPC_CONNECT:
        case PSA_IPC_DISCONNECT:
            status = PSA_SUCCESS;
            break;
        case PSA_IPC_CALL:
            status = snapshot(&msg);
            break;
        }
        psa_reply(msg.handle, status);
    }
}
