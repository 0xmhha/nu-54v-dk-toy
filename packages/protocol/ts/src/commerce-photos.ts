// Photos a screen asked for, kept by their names. Names are their SHA-256, so a kept photo never goes stale.

type Reader = (name: string) => Promise<string>;

export class CommercePhotoCache {
  private kept = new Map<string, string>();
  private failed = new Map<string, number>();
  private queued = new Set<string>();
  private line: Promise<void> = Promise.resolve();
  private listeners = new Set<() => void>();

  /** `keep`: how many photos stay in memory; the oldest go first. */
  private read: Reader;
  private keep: number;
  private clock: () => number;
  constructor(read: Reader, keep = 400, clock = () => Date.now()) {
    this.read = read;
    this.keep = keep;
    this.clock = clock;
  }

  /** Base64 of the JPEG, once it has arrived. */
  get(name: string | undefined): string | undefined {
    return name ? this.kept.get(name) : undefined;
  }

  /** Asks for a photo. Photos come one at a time, in the order they were asked for, so a menu never floods the link. */
  want(name: string | undefined) {
    if (!name || this.kept.has(name) || this.queued.has(name) || this.clock() < (this.failed.get(name) ?? 0)) return;
    this.queued.add(name);
    this.line = this.line.then(async () => {
      try {
        const data = await this.read(name);
        this.kept.delete(name);
        this.kept.set(name, data);
        while (this.kept.size > this.keep) this.kept.delete(this.kept.keys().next().value!);
        this.failed.delete(name);
      } catch {
        // A photo that did not come is asked for again a minute later; the screen shows the name meanwhile.
        this.failed.set(name, this.clock() + 60_000);
      } finally {
        this.queued.delete(name);
        for (const listener of [...this.listeners]) listener();
      }
    });
  }

  subscribe(listener: () => void) {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  /** Waits until everything asked for so far has arrived or failed. */
  settled() {
    return this.line;
  }
}
