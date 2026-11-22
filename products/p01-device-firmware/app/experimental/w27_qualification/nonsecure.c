#include <errno.h>
#include <psa/client.h>
#include <psa_manifest/sid.h>
#include <zephyr/init.h>
#include "device_owner_limits_ps.h"
#include "w27_qualification.h"
#include "w27_qualification_image.h"

_Static_assert(W27_QUALIFICATION_SNAPSHOT_VERSION == W27Q_VERSION, "W27 PSA version mismatch");

static bool read_snapshot(uint8_t response[W27Q_RESPONSE_BYTES])
{
    uint8_t request[W27Q_REQUEST_BYTES];
    const psa_invec input = {request, sizeof(request)};
    psa_outvec output = {response, W27Q_RESPONSE_BYTES};

    w27q_request(request, NU54_W27Q_IMAGE);
    psa_handle_t handle = psa_connect(W27_QUALIFICATION_SNAPSHOT_SID, W27Q_VERSION);
    if (!PSA_HANDLE_IS_VALID(handle)) {
        return false;
    }
    psa_status_t status = psa_call(handle, PSA_IPC_CALL, &input, 1, &output, 1);
    psa_close(handle);
    return status == PSA_SUCCESS && w27q_response_valid(response, output.len);
}

static int w27q_init(void)
{
    uint8_t response[W27Q_RESPONSE_BYTES];
    return read_snapshot(response) ? 0 : -EIO;
}

/* Runs after the SDK's POST_KERNEL TF-M NS interface initialization. */
SYS_INIT(w27q_init, APPLICATION, CONFIG_KERNEL_INIT_PRIORITY_DEFAULT);

bool device_owner_limits_platform_qualified(void)
{
    uint8_t response[W27Q_RESPONSE_BYTES];
    return read_snapshot(response) && w27q_complete(response, sizeof(response));
}
