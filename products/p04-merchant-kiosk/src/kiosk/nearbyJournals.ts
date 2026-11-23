import { V2Journal, type NearbyReady } from '@nu54/protocol';

// One phone reads its order every few seconds and a record stays about 320 s.
const perPhone = 512;
const phones = 32;

/**
 * One replay journal per phone identity, so a busy store or a flooding phone cannot use up another
 * phone's room. A journal outlives its links and the longest idle one makes place for a new phone.
 * Dropping it is safe: a frame from an earlier link still fails the TLS epoch and sequence checks.
 */
export class NearbyJournals {
  private readonly kept = new Map<string, { journal: V2Journal; links: Set<string> }>();

  open(peer: Pick<NearbyReady, 'peer' | 'link'>): V2Journal {
    const row = this.kept.get(peer.peer) ?? { journal: new V2Journal(perPhone), links: new Set<string>() };
    this.kept.delete(peer.peer);
    if (this.kept.size >= phones) {
      const idle = [...this.kept].find(([, item]) => !item.links.size)?.[0];
      if (idle === undefined) throw new Error('NEARBY_BOUNDS');
      this.kept.delete(idle);
    }
    row.links.add(peer.link);
    this.kept.set(peer.peer, row);
    return row.journal;
  }

  release(link: string) {
    for (const [peer, row] of this.kept) {
      if (!row.links.delete(link)) continue;
      // Idle age starts when the last link ends.
      this.kept.delete(peer);
      this.kept.set(peer, row);
      return;
    }
  }

  get size() {
    return this.kept.size;
  }
}
