import {entranceAnchor} from './customerEntrance.ts';
import {approvedCustomerEntrance, approvedOrderPayment} from './customerEntranceDeployment.ts';
import {orderPaymentPin} from './orderPaymentTrust.ts';

/**
 * Whether this build orders without a store registration: both its customer entrance and its order contract are
 * bound (customerEntranceDeployment.ts). Such a build never asks a customer to register a phone or a board at a
 * store, and shows no way to. A build without either keeps the earlier route, registration and all; the repository
 * ships that one.
 */
export const ordersWithoutRegistration = (anchor = approvedCustomerEntrance, payment = approvedOrderPayment): boolean =>
  !!entranceAnchor(anchor) && !!orderPaymentPin(payment);
