// The journal names the native commerce module of this app accepts: packages/protocol platform/android
// CommerceHttp.kt and platform/ios CommerceHttp.h (journalName, as built for the user app). A test journal that
// takes any name hides a name the device refuses: order sessions were written under one for a while and no real
// phone kept them. test/journalNames.test.ts holds this list against both native sources.

export const journalNames = ['commerce-inbox-v2', 'customer-order-v2', 'customer-payment-v2', 'customer-qr-v2', 'customer-stores-v1', 'terminal-payment-v2',
  'terminal-mode-request-v2', 'terminal-mode-applied-v2', 'terminal-public-menu-v1', 'travel-pickup-kitchen-v1', 'subscription-alerts-v2', 'market-lab-8283-v1'];
export const journalPatterns = 'cloud-p03-[a-f0-9]{64}|travel-local-[a-f0-9-]{36}|travel-hold-[a-f0-9]{64}|pickup-v2-[a-f0-9]{64}|print-v1-[a-f0-9]{64}|' +
  'pickup-legacy-[a-f0-9]{64}|customer-(order|payment|qr|legacy|sessions)-[a-f0-9]{64}';
/** The longest name a pattern makes: `customer-sessions-` and 64 characters. iOS refuses a longer one before it looks. */
export const journalNameLimit = 82;

export const journalAllowed = (name: string) =>
  journalNames.includes(name) || (name.length <= journalNameLimit && new RegExp(`^(?:${journalPatterns})$`).test(name));

/** The sealed journal as a device keeps it. A name outside the list is refused the way the native module refuses it. */
export function journalStore(kept = new Map<string, string>()) {
  const check = (name: string) => {
    if (!journalAllowed(name)) throw Object.assign(new Error('The enrolled commerce connection is unavailable'), {code: 'COMMERCE_REFUSED'});
  };
  return {
    kept,
    loadJournal: async (name: string) => { check(name); return kept.get(name) ?? ''; },
    saveJournal: async (name: string, value: string) => { check(name); if (value) kept.set(name, value); else kept.delete(name); },
  };
}
