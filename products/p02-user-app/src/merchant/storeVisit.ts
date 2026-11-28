import type {CommerceSelection} from '@nu54/protocol';

// What a customer left unfinished at a store page, for as long as the app runs: the basket they were filling, and
// whether the store holds an order of theirs that is not paid yet. The page of a store is closed whenever the
// customer leaves the payment tab, for example to top up the board account, and the same basket has to be there on
// the way back (traveller-services.md chapter 5, "원화로 결제하고, 모자라면 충전"). The app holds one of these and
// hands it to the page; nothing here is kept on disk: a basket is no order, and a held order has its own record in
// the sealed journal.

export type StoreVisit = {basket?: CommerceSelection; checkout: boolean};
export type StoreVisits = {
  /** Writes down what the page of `branch` holds now. An empty visit is forgotten. */
  keep(branch: string, visit: StoreVisit): void;
  /** The basket left at `branch`, as it was: its rows are those of the menu read then, to be read against the menu now. */
  basket(branch: string): CommerceSelection | undefined;
  /** Whether the customer has a basket or an unpaid order at `branch` to come back to. */
  unfinished(branch: string): boolean;
};

const stores = 8; // the newest stores; an older basket goes

export function storeVisits(): StoreVisits {
  const visits = new Map<string, StoreVisit>();
  return {
    keep(branch, visit) {
      visits.delete(branch);
      if (!visit.checkout && !(visit.basket && Object.keys(visit.basket).length)) return;
      visits.set(branch, visit);
      for (const oldest of visits.keys()) {
        if (visits.size <= stores) break;
        visits.delete(oldest);
      }
    },
    basket: branch => visits.get(branch)?.basket,
    unfinished: branch => visits.has(branch),
  };
}
