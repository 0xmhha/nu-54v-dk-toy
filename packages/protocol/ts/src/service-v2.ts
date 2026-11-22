// Client-side shape and binding checks for authenticated C3–C9 transports.
// Service owners use packages/protocol/go/service.Authorize for verified domain authority.
import { sha256 } from "@noble/hashes/sha256";
import { utf8Encode } from "./bytes.ts";
import { SERVICE_V2_RULES } from "./service-rules-v2.ts";

export { SERVICE_V2_RULES };

type Mode = { readonly mode: string; readonly roles: readonly string[]; readonly ttlSeconds: number;
  readonly fields: Readonly<Record<string, string>>; readonly response: string };
const operations: Readonly<Record<string, Mode>> = SERVICE_V2_RULES.operations;
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const digest = /^[0-9a-f]{64}$/;
const address = /^0x[0-9a-f]{40}$/;
const core = ["version", "contract", "operation", "requestId", "domainRef", "objectRef", "expiresAt", "body"];

export interface ServiceScopeV2 {
  tenantRef: string;
  subjectRef: string;
  role: string;
  expiresAt: number;
  grants: readonly { domainRef: string; objectRef: string; operations: readonly string[] }[];
}

export interface ServiceRequestV2 {
  version: number;
  contract: string;
  operation: string;
  requestId: string;
  domainRef: string;
  objectRef: string;
  expiresAt: number;
  idempotencyKey?: string;
  body: Record<string, string>;
}

export interface ServiceBindingV2 {
  requestId: string;
  requestDigest: string;
  intentDigest: string;
}

export interface ServiceEvidenceV2 {
  eventId: string;
  generationRef: string;
  kind: string;
  anchorHash: string;
  fact: string;
  merchantKey: string;
  orderKey: string;
  txHash: string;
  observedAt: number;
  expiresAt: number;
}

export interface ServiceReplyV2 {
  requestId: string;
  requestDigest: string;
  status: string;
  error?: ServiceErrorV2;
  resultRef?: string;
  evidence?: ServiceEvidenceV2;
}

export type ServiceErrorV2 = typeof SERVICE_V2_RULES.errorCodes[number];

function fail(code: ServiceErrorV2): never {
  throw new Error(code);
}

function field(kind: string, value: unknown): boolean {
  if (typeof value !== "string") return false;
  if (kind === "uuid") return uuid.test(value);
  if (kind === "code8") return /^[0-9a-f]{8}$/.test(value);
  if (kind === "digest") return digest.test(value);
  if (kind === "address") return address.test(value) && value !== "0x" + "0".repeat(40);
  if (kind.includes("|")) return kind.split("|").includes(value);
  if (value.length > 78 || !/^(0|[1-9][0-9]*)$/.test(value)) return false;
  const n = BigInt(value);
  const bits = ({ uint32: 32, uint64: 64, uint256: 256, positive64: 64, positive256: 256, time: 63 } as Record<string, number>)[kind];
  return !!bits && n < (1n << BigInt(bits)) && (kind.startsWith("uint") || n > 0n);
}

function hashParts(parts: readonly string[]): string {
  const encoded = parts.map(value => `${utf8Encode(value).length}:${value}`).join("");
  return Array.from(sha256(utf8Encode(encoded)), byte => byte.toString(16).padStart(2, "0")).join("");
}

export function bindServiceRequestV2(request: ServiceRequestV2, scope: ServiceScopeV2, now: number): ServiceBindingV2 {
  if (request.version !== SERVICE_V2_RULES.schemaVersion) fail("UNSUPPORTED_VERSION");
  const operation = `${request.contract}/${request.operation}`;
  if (!Object.hasOwn(operations, operation)) fail("UNSUPPORTED_CAPABILITY");
  const rule = operations[operation];
  const envelopeKeys = Object.keys(request);
  if (envelopeKeys.some(name => !core.includes(name) && name !== "idempotencyKey") ||
      core.some(name => !Object.hasOwn(request, name)) ||
      [request.requestId, request.domainRef, request.objectRef].some(value => !field("uuid", value)) ||
      !Number.isSafeInteger(request.expiresAt) || !Number.isSafeInteger(now) ||
      (rule.mode === "command" ? !field("uuid", request.idempotencyKey) : Object.hasOwn(request, "idempotencyKey")) ||
      !request.body || typeof request.body !== "object" || Array.isArray(request.body) ||
      Object.keys(request.body).length !== Object.keys(rule.fields).length ||
      Object.entries(rule.fields).some(([name, type]) => !Object.hasOwn(request.body, name) || !field(type, request.body[name]))) {
    fail("BAD_REQUEST");
  }
  if (!field("uuid", scope.tenantRef) || !field("uuid", scope.subjectRef) || !rule.roles.includes(scope.role) ||
      !Number.isSafeInteger(scope.expiresAt) || scope.expiresAt <= now ||
      !scope.grants.some(grant => grant.domainRef === request.domainRef && grant.objectRef === request.objectRef &&
        grant.operations.includes(operation))) fail("NOT_AUTHORIZED");
  if (request.expiresAt <= now || request.expiresAt - now > rule.ttlSeconds || request.expiresAt > scope.expiresAt) fail("EXPIRED");

  if (operation === "C5/transaction.submit") {
    const direct = request.body.mode === "direct";
    if (direct ? scope.role !== "payer" || request.body.payer !== request.body.feePayer
      : scope.role !== "sponsor" || request.body.payer === request.body.feePayer) fail("NOT_AUTHORIZED");
  }

  const parts = ["nu54-intent-v2", operation, scope.tenantRef, scope.subjectRef, request.domainRef,
    request.objectRef, request.idempotencyKey ?? ""];
  for (const name of Object.keys(request.body).sort()) parts.push(name, request.body[name]);
  const intentDigest = hashParts(parts);
  return { requestId: request.requestId, intentDigest,
    requestDigest: hashParts(["nu54-request-v2", intentDigest, request.requestId, String(request.expiresAt)]) };
}

export function validateServiceReplyV2(request: ServiceRequestV2, binding: ServiceBindingV2, reply: ServiceReplyV2, now: number): void {
  if (!Number.isSafeInteger(now) || request.expiresAt <= now) fail("EXPIRED");
  if (reply.requestId !== binding.requestId || reply.requestId !== request.requestId ||
      !field("digest", binding.requestDigest) || reply.requestDigest !== binding.requestDigest) fail("MISMATCHED_EVIDENCE");
  const rule = operations[`${request.contract}/${request.operation}`];
  if (reply.status === "error") {
    if (!reply.error || !SERVICE_V2_RULES.errorCodes.includes(reply.error) || reply.resultRef || reply.evidence) fail("BAD_REQUEST");
    return;
  }
  if (reply.error) fail("BAD_REQUEST");
  if (rule.response !== "evidence") {
    if (!["ok", "accepted"].includes(reply.status) || !field("uuid", reply.resultRef) || reply.evidence) fail("BAD_REQUEST");
    return;
  }
  if (reply.resultRef) fail("BAD_REQUEST");
  if (reply.status !== request.body.kind) fail("MISMATCHED_EVIDENCE");
  const e = reply.evidence;
  if (reply.status === "unobserved") {
    if (e) fail("MISMATCHED_EVIDENCE");
    return;
  }
  if (!e || e.eventId !== request.objectRef || e.generationRef !== request.domainRef || e.kind !== reply.status ||
      !field("digest", e.anchorHash) ||
      (reply.status === "finalized" && (!["paid", "refunded", "invalidated"].includes(e.fact) ||
        (!field("digest", e.merchantKey) && !field("address", e.merchantKey)) || !field("digest", e.orderKey) || !field("digest", e.txHash))) ||
      (reply.status === "revoked" && (e.fact !== "revoked" || !field("digest", e.orderKey) || e.txHash !== "")) || !Number.isSafeInteger(e.observedAt) || !Number.isSafeInteger(e.expiresAt) ||
      e.observedAt <= 0 || e.observedAt > now || e.expiresAt <= now ||
      e.expiresAt - e.observedAt > SERVICE_V2_RULES.maxEvidenceAgeSeconds) fail("MISMATCHED_EVIDENCE");
}
