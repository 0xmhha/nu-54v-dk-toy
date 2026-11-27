// What the owner agreed to on this phone. A new edition of a document is asked again.
import {notice, notices, type Notice} from './notices.ts';

export type ConsentId = 'terms' | 'privacy' | 'test-assets' | 'alerts';
/** `declined` keeps an optional agreement that was given and taken back, or never given. */
export type Consent = {id: ConsentId; edition: string; at: number; declined?: true};

export const requiredConsents = ['terms', 'privacy', 'test-assets'] as const;
const ids: string[] = [...requiredConsents, 'alerts'];
/** Order-ready notices are a choice of this app's own; it has no document behind it. */
const alertsEdition = '1';

export function validConsents(value: unknown): value is Consent[] {
  return Array.isArray(value) && value.length <= ids.length && new Set(value.map(item => item?.id)).size === value.length && value.every((item: Consent) =>
    !!item && typeof item === 'object' && Object.keys(item).every(key => ['id', 'edition', 'at', 'declined'].includes(key)) && ids.includes(item.id) &&
    typeof item.edition === 'string' && item.edition.length > 0 && item.edition.length <= 32 && Number.isSafeInteger(item.at) && item.at > 0 &&
    (item.declined === undefined || (item.declined === true && item.id === 'alerts')));
}

export const editionOf = (id: ConsentId, documents: Notice[] = notices.documents) =>
  id === 'alerts' ? alertsEdition : notice(id, 'ko', documents)?.edition;

/** Required documents that were not agreed to in their present edition. */
export function missingConsents(given: Consent[] = [], documents: Notice[] = notices.documents) {
  return requiredConsents.filter(id => {
    const edition = editionOf(id, documents);
    return edition !== undefined && !given.some(item => item.id === id && item.edition === edition);
  });
}

export function withConsent(given: Consent[] = [], id: ConsentId, agreed: boolean, at: number, documents?: Notice[]): Consent[] {
  const edition = editionOf(id, documents);
  if (edition === undefined || (!agreed && id !== 'alerts')) return given;
  return [...given.filter(item => item.id !== id), {id, edition, at, ...(agreed ? {} : {declined: true as const})}];
}

/** Order-ready notices stay off once the owner said so; before any answer the phone's own permission decides. */
let alertsOff = false;
export const noteConsents = (given: Consent[] = []) => { alertsOff = !!given.find(item => item.id === 'alerts')?.declined; };
export const orderAlertsOff = () => alertsOff;
