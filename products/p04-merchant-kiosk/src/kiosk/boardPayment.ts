import {
  bytesToHex,
  encodeV2StreamFrame,
  fromBase64,
  hexToBytes,
  readNearbyReady,
  rentalReviewPrepare,
  toBase64,
  V2Journal,
  V2Link,
  V2StreamDecoder,
  verifyCommerceApproval,
  verifyRentalPaymentReview,
  verifyRentalPaymentDecision,
  type RentalReviewExpectation,
  type VerifiedCommercePayment,
} from '@nu54/protocol';
import type { Spec } from '../specs/NativeNearby.ts';

type Native = Spec;
const now = () => Math.floor(Date.now() / 1000);

export async function findPaymentBoards(module: Native, enrollment: string) {
  await module.configurePayment(enrollment);
  const result = JSON.parse(await module.scanPayment(5)) as {
    handle: string;
  }[];
  if (
    !Array.isArray(result) ||
    result.length > 16 ||
    result.some(
      item =>
        typeof item?.handle !== 'string' ||
        !item.handle.length ||
        item.handle.length > 80,
    )
  ) {
    throw new Error('PAYMENT_BOARD_SCAN');
  }
  return result;
}

type RentalPaymentRoute = {
  scope: Omit<RentalReviewExpectation, 'now'> & {expiresAt:number};
  reviewed: (receipt:string)=>void;
  decided?: (decision:string)=>void;
  begin: ()=>void;
};

class UnavailablePaymentCandidate extends Error {}

export async function reviewRentalBoard(
  module:Native,
  enrollment:string,
  branch:string,
  payment:VerifiedCommercePayment,
  signal:AbortSignal,
  rental:RentalPaymentRoute,
) {
  if (signal.aborted || rental.scope.expiresAt <= now()) throw new Error('PAYMENT_EXPIRY');
  const devices=await findPaymentBoards(module,enrollment);
  for (const handle of new Set(devices.map(device=>device.handle))) {
    if (signal.aborted || rental.scope.expiresAt <= now()) throw new Error('PAYMENT_EXPIRY');
    try {
      return await boardPayment(module,handle,branch,payment,hexToBytes(rental.scope.request),()=>{},signal,rental);
    } catch (error) {
      if (!(error instanceof UnavailablePaymentCandidate)) throw error;
    }
  }
  throw new Error('PAYMENT_BOARD_UNAVAILABLE');
}

/** C2 sends the exact canonical signed quote and returns the independently checked payer signature. */
export async function boardPayment(
  module: Native,
  handle: string,
  branch: string,
  payment: VerifiedCommercePayment,
  request: Uint8Array,
  preparing: () => void,
  signal: AbortSignal,
  rental?: RentalPaymentRoute,
) {
  const stream = new V2StreamDecoder();
  const journal = new V2Journal(4);
  let linkHandle: string | undefined;
  let link: V2Link | undefined;
  let fail: (error: Error) => void = () => {};
  let resolveReady: () => void = () => {};
  let resolveApproval: (value: string) => void = () => {};
  const pending = new Map<string, unknown>();
  let rentalReview: Uint8Array | undefined;
  let disconnected = false;
  let retryable = false;
  const activate = (raw: unknown) => {
    const trusted = readNearbyReady(raw);
    if (trusted.branch !== branch || signal.aborted || link ||
        rental && trusted.peer !== rental.scope.boardPin)
      throw new Error('PAYMENT_BOARD_MISMATCH');
    link = new V2Link(
      'payment',
      'terminal',
      fromBase64(trusted.epoch),
      hexToBytes(trusted.peer),
      journal,
    );
    resolveReady();
  };
  const sub = module.onPayment(raw => {
    try {
      const event = JSON.parse(raw);
      if (typeof event.link !== 'string') return;
      if (!linkHandle) {
        if ((event.kind === 'ready' || event.kind === 'closed') && (pending.has(event.link) || pending.size < 8))
          pending.set(event.link,event.kind === 'closed' ? null : pending.get(event.link) === null ? null : event);
        return;
      }
      if (event.link !== linkHandle) return;
      if (event.kind === 'ready') { activate(event); return; }
      if (event.kind === 'closed') {
        disconnected = true;
        throw new Error('PAYMENT_RESULT_UNKNOWN');
      }
      if (event.kind !== 'data' || typeof event.clear !== 'string' || !link)
        return;
      const own = link;
      const bytes = fromBase64(event.clear);
      if (bytes.length > 3072) throw new Error('PAYMENT_BOARD_BOUNDS');
      stream.feed(bytes, frame => {
        const reply = own.receive(frame, BigInt(now()))?.request;
        if (reply?.command === 'rental.review') {
          if (!rental) throw new Error('RENTAL_REVIEW_SCOPE');
          verifyRentalPaymentReview(reply.payload, payment, { ...rental.scope, now: now() });
          rentalReview = reply.payload;
          rental.reviewed(bytesToHex(reply.payload, false));
          return;
        }
        if (reply?.command !== 'reply') return;
        if (rental) {
          if (!rentalReview) throw new Error('PAYMENT_RESULT_UNKNOWN');
          const result = verifyRentalPaymentDecision(reply.payload, rentalReview, payment, {...rental.scope, now:now()});
          rental.decided?.(bytesToHex(reply.payload, false));
          if (result.decision === 'refused') throw new Error('PAYMENT_REFUSED');
          resolveApproval(result.signature);
          return;
        }
        if (reply.payload.length === 1 && reply.payload[0] === 0)
          throw new Error('PAYMENT_REFUSED');
        resolveApproval(verifyCommerceApproval(payment, reply.payload, now()));
      });
    } catch (error) {
      disconnected = true;
      fail(
        error instanceof Error ? error : new Error('PAYMENT_RESULT_UNKNOWN'),
      );
    }
  });
  const cancel = () => fail(new Error('PAYMENT_RESULT_UNKNOWN'));
  signal.addEventListener('abort', cancel);
  let timeout: ReturnType<typeof setTimeout> | undefined;
  try {
    try {
      linkHandle = await module.connectPayment(handle);
      if (signal.aborted) throw new Error('PAYMENT_RESULT_UNKNOWN');
      const ready = new Promise<void>((resolve, reject) => {
        resolveReady = resolve;
        fail = reject;
      });
      ready.catch(() => {});
      timeout = setTimeout(cancel, 7000);
      if (pending.get(linkHandle) === null) throw new Error('PAYMENT_RESULT_UNKNOWN');
      if (pending.has(linkHandle)) activate(pending.get(linkHandle));
      pending.clear();
      await ready;
      if (disconnected) throw new Error('PAYMENT_RESULT_UNKNOWN');
      clearTimeout(timeout);
    } catch (error) {
      if (!rental || signal.aborted || rental.scope.expiresAt <= now()) throw error;
      retryable = true;
      throw new UnavailablePaymentCandidate('PAYMENT_BOARD_PROBE_FAILED');
    }
    const open = linkHandle;
    const remaining = payment.deadline - now();
    if (
      !link ||
      !Number.isInteger(remaining) ||
      remaining <= 0 ||
      remaining > 120
    )
      throw new Error('PAYMENT_EXPIRY');
    const codec: V2Link = link;
    const approval = new Promise<string>((resolve, reject) => {
      resolveApproval = resolve;
      fail = reject;
    });
    approval.catch(() => {});
    timeout = setTimeout(cancel, remaining * 1000);
    if (rental && (payment.payerRole !== 'renter' || rental.scope.request !== bytesToHex(request, false)))
      throw new Error('RENTAL_REVIEW_SCOPE');
    const packets = codec.send({
      command: 'pay.prepare',
      id: request,
      expires: BigInt(payment.deadline),
      payload: rental ? rentalReviewPrepare(hexToBytes(rental.scope.challenge), payment.bytes) : payment.bytes,
    });
    preparing();
    if (signal.aborted || disconnected || rental && rental.scope.expiresAt <= now())
      throw new Error('PAYMENT_RESULT_UNKNOWN');
    rental?.begin();
    try {
      for (const packet of packets) {
        if (signal.aborted || disconnected || rental && rental.scope.expiresAt <= now()) {
          cancel();
          break;
        }
        await module.writePayment(open, toBase64(encodeV2StreamFrame(packet)));
      }
    } catch {
      cancel();
    }
    return await approval;
  } finally {
    if (timeout) clearTimeout(timeout);
    signal.removeEventListener('abort', cancel);
    sub.remove();
    stream.close();
    (link as V2Link | undefined)?.close();
    if (linkHandle) await module.closePayment(linkHandle).catch(() => {
      if (retryable) throw new Error('PAYMENT_BOARD_CLOSE_UNCONFIRMED');
    });
  }
}
