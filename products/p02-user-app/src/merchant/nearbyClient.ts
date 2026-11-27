import { NearbyCommerceLink, readNearbyReady, V2Journal } from '@nu54/protocol';
import type { Spec } from '../specs/NativeNearby.ts';

export class CustomerNearby {
  private current?: NearbyCommerceLink;
  private currentHandle?: string;
  private connecting = false;
  private closed = false;
  private wait?: {
    resolve: (link: NearbyCommerceLink) => void;
    reject: (error: Error) => void;
  };
  private pending = new Map<string, unknown>();
  private journal = new V2Journal(48);
  private module: Spec;
  private expectedBranch: string;
  private listener: { remove(): void };

  constructor(module: Spec, branch: string, private onDisconnect: () => void = () => {}) {
    this.module = module;
    this.expectedBranch = branch;
    this.listener = module.onNearby(raw => this.notice(raw));
  }

  private notice(raw: string) {
    try {
      const data = JSON.parse(raw);
      if (typeof data.link !== 'string') return;
      if (data.kind === 'ready') {
        if (this.connecting && !this.currentHandle && this.pending.size < 8)
          this.pending.set(data.link, data);
        else if (data.link === this.currentHandle) this.activate(data);
      } else if (
        data.link === this.currentHandle &&
        data.kind === 'data' &&
        typeof data.clear === 'string'
      ) {
        this.current?.receive(data.clear);
      } else if (data.link === this.currentHandle && data.kind === 'closed') {
        const wasConnected = this.current !== undefined;
        this.disconnect();
        if (wasConnected) this.onDisconnect();
      }
    } catch {
      this.wait?.reject(new Error('NEARBY_TRUST_REQUIRED'));
    }
  }

  private activate(data: unknown) {
    const peer = readNearbyReady(data);
    if (this.closed || peer.branch !== this.expectedBranch || this.current)
      throw new Error('NEARBY_WRONG_TERMINAL');
    this.current = new NearbyCommerceLink(
      peer,
      'customer',
      this.module,
      this.journal,
    );
    this.wait?.resolve(this.current);
  }

  async scan(rawEnrollment: string) {
    await this.module.configure(rawEnrollment);
    const value = JSON.parse(await this.module.scan(5)) as { handle: string }[];
    if (
      !Array.isArray(value) ||
      value.length > 16 ||
      value.some(
        item => typeof item?.handle !== 'string' || !item.handle.length || item.handle.length > 80,
      ) || new Set(value.map(row => row.handle)).size !== value.length
    ) {
      throw new Error('NEARBY_SCAN');
    }
    return value;
  }

  async connect(handle: string) {
    if (this.closed || this.connecting) throw new Error('NEARBY_ALREADY_CONNECTED');
    this.disconnect();
    this.connecting = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      const handleFromNative = await this.module.connect(handle);
      if (typeof handleFromNative !== 'string' || !handleFromNative.length ||
          handleFromNative.length > 80)
        throw new Error('NEARBY_LINK');
      this.currentHandle = handleFromNative;
      if (this.closed) throw new Error('NEARBY_DISCONNECTED');
      const pending = this.pending.get(handleFromNative);
      if (pending) {
        this.activate(pending);
        return this.current!;
      }
      const ready = new Promise<NearbyCommerceLink>((resolve, reject) => {
        this.wait = {resolve, reject};
      });
      timer = setTimeout(() => this.wait?.reject(new Error('NEARBY_TIMEOUT')), 8000);
      return await ready;
    } catch (error) {
      this.disconnect();
      throw error;
    } finally {
      this.connecting = false;
      this.wait = undefined;
      this.pending.clear();
      clearTimeout(timer);
    }
  }

  disconnect() {
    const link = this.current;
    const handle = this.currentHandle;
    this.current = undefined;
    this.currentHandle = undefined;
    this.pending.clear();
    this.wait?.reject(new Error('NEARBY_DISCONNECTED'));
    if (link) link.close();
    else if (handle) this.module.close(handle).catch(() => {});
  }

  close() {
    this.closed = true;
    this.listener.remove();
    this.disconnect();
  }
}
