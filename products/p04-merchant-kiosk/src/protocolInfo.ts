import { DEVICE_SIGNED_TYPES, OUTCOMES, PROTOCOL_VERSION } from "@nu54/protocol";

// Values the kiosk UI shows while the payment flow is built (WBS2-P04-01).
export const protocolInfo = {
  version: PROTOCOL_VERSION,
  deviceSignedTypes: DEVICE_SIGNED_TYPES,
  outcomes: OUTCOMES,
};
