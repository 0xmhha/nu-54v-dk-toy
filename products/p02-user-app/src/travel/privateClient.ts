import {commerceRequestId, fromBase64} from '@nu54/protocol';
import Native from '../specs/NativeCommerceHttp.ts';

export const opaqueTravelID = /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/;
export type Purpose = 'location' | 'voice' | 'transcription' | 'recommendation' | 'review' | 'challenge';
export type TravelPolicy = {
  purpose: Purpose;
  version: number;
  recipient: string;
  maxLifetimeSeconds: number;
  targetRequired: boolean;
};
export type Consent = {
  purpose: Purpose;
  revision: number;
  policyVersion: number;
  recipient: string;
  accepted: boolean;
  object?: string;
  expiresAt: string;
  receipt: string;
};

const limits: Record<Purpose, number> = {
  location: 2592000, voice: 604800, transcription: 2592000,
  recommendation: 1209600, review: 31536000, challenge: 31536000,
};

export async function privateTravelPolicies(): Promise<TravelPolicy[]> {
  const raw = await Native?.loadProfile('travel');
  if (!raw) return [];
  const {scope, policies} = JSON.parse(raw);
  if (scope?.role !== 'private-user' || scope.expiresAt <= Date.now() / 1000 || !Array.isArray(policies)) return [];
  return policies.filter((value: TravelPolicy) => value && typeof value === 'object' &&
    Object.hasOwn(limits, value.purpose) && Number.isSafeInteger(value.version) && value.version > 0 &&
    typeof value.recipient === 'string' && /^[a-z0-9][a-z0-9_-]{0,63}$/.test(value.recipient) &&
    Number.isSafeInteger(value.maxLifetimeSeconds) && value.maxLifetimeSeconds > 0 &&
    value.maxLifetimeSeconds <= limits[value.purpose] && typeof value.targetRequired === 'boolean',
  );
}

export async function travelID() {
  if (!Native) throw new Error('Private transport unavailable');
  return commerceRequestId(fromBase64(await Native.randomBytes(16)));
}

export const erasureInstallationProof = /^[a-f0-9]{64}$/;

export async function newErasureInstallationProof() {
  if (!Native) throw new Error('Private transport unavailable');
  const bytes = fromBase64(await Native.randomBytes(32));
  if (bytes.length !== 32) throw new Error('Private randomness unavailable');
  return Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
}

export class TravelAuthorityError extends Error {
  constructor(readonly reason: string) { super('Travel authority unavailable'); }
}

export async function privateTravel<T>(path: string, body?: object): Promise<T> {
  if (!Native) throw new Error('Private transport unavailable');
  const answer = JSON.parse(await Native.exchangeTravel(body ? 'POST' : 'GET', path, body ? JSON.stringify(body) : ''));
  if (!answer || typeof answer !== 'object') throw new Error('Travel authority unavailable');
  if (typeof answer.error === 'string') throw new TravelAuthorityError(answer.error);
  return answer as T;
}

export function consentScope(
  policy: TravelPolicy, current: Consent | undefined, target: string,
  requestKey: string, accept: boolean, now = Date.now(),
) {
  if (!opaqueTravelID.test(requestKey)) throw new Error('Invalid consent request');
  if (!accept) {
    if (!current?.accepted || current.purpose !== policy.purpose) throw new Error('No active purpose to withdraw');
    return {purpose: current.purpose, expectedRevision: current.revision,
      policyVersion: current.policyVersion, recipient: current.recipient, object: current.object, requestKey, accept};
  }
  if (policy.targetRequired && !opaqueTravelID.test(target)) throw new Error('Exact consent target is required');
  return {purpose: policy.purpose, expectedRevision: current?.revision ?? 0,
    policyVersion: policy.version, recipient: policy.recipient,
    object: policy.targetRequired ? target : undefined, requestKey, accept,
    expiresAt: new Date(now + Math.max(1, policy.maxLifetimeSeconds - 5) * 1000).toISOString()};
}
