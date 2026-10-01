// Minimal JSON-RPC client over fetch (React Native and Node both provide fetch).

export type Hex = `0x${string}`;

export interface Log {
  address: Hex;
  topics: Hex[];
  data: Hex;
  blockNumber: Hex;
  transactionHash: Hex;
}

export interface Receipt {
  status: Hex;
  blockNumber: Hex;
  transactionHash: Hex;
  logs: Log[];
}

export class RpcError extends Error {
  readonly code: number;
  /** Revert data of a failed eth_call / eth_estimateGas, when the node returns it. */
  readonly data?: Hex;
  constructor(code: number, message: string, data?: Hex) {
    super(message);
    this.code = code;
    this.data = data;
  }
}

/** The chain calls the kiosk makes; tests replace it with a fake. */
export interface Chain {
  call(tx: { from: Hex; to: Hex; data: Hex }, block: "latest" | "finalized"): Promise<Hex>;
  estimateGas(tx: { from: Hex; to: Hex; data: Hex }): Promise<bigint>;
  balance(address: Hex): Promise<bigint>;
  pendingNonce(address: Hex): Promise<bigint>;
  priorityFee(): Promise<bigint>;
  block(tag: "latest" | "finalized"): Promise<{ number: bigint; timestamp: bigint; baseFee: bigint }>;
  sendRaw(raw: Hex): Promise<Hex>;
  receipt(hash: Hex): Promise<Receipt | null>;
  logs(filter: { address: Hex; topics: (Hex | null)[]; fromBlock: Hex; toBlock: "finalized" }): Promise<Log[]>;
}

export class JsonRpcChain implements Chain {
  private id = 0;
  private readonly url: string;
  constructor(url: string) {
    this.url = url;
  }

  private async rpc<T>(method: string, params: unknown[]): Promise<T> {
    const res = await fetch(this.url, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ jsonrpc: "2.0", id: ++this.id, method, params }),
    });
    const j = (await res.json()) as { result?: T; error?: { code: number; message: string; data?: Hex } };
    if (j.error) throw new RpcError(j.error.code, j.error.message, j.error.data);
    return j.result as T;
  }

  call(tx: { from: Hex; to: Hex; data: Hex }, block: "latest" | "finalized") {
    return this.rpc<Hex>("eth_call", [tx, block]);
  }
  async estimateGas(tx: { from: Hex; to: Hex; data: Hex }) {
    return BigInt(await this.rpc<Hex>("eth_estimateGas", [tx]));
  }
  async balance(address: Hex) {
    return BigInt(await this.rpc<Hex>("eth_getBalance", [address, "latest"]));
  }
  async pendingNonce(address: Hex) {
    return BigInt(await this.rpc<Hex>("eth_getTransactionCount", [address, "pending"]));
  }
  async priorityFee() {
    return BigInt(await this.rpc<Hex>("eth_maxPriorityFeePerGas", []));
  }
  async block(tag: "latest" | "finalized") {
    const b = await this.rpc<{ number: Hex; timestamp: Hex; baseFeePerGas: Hex }>("eth_getBlockByNumber", [tag, false]);
    return { number: BigInt(b.number), timestamp: BigInt(b.timestamp), baseFee: BigInt(b.baseFeePerGas ?? "0x0") };
  }
  sendRaw(raw: Hex) {
    return this.rpc<Hex>("eth_sendRawTransaction", [raw]);
  }
  receipt(hash: Hex) {
    return this.rpc<Receipt | null>("eth_getTransactionReceipt", [hash]);
  }
  logs(filter: { address: Hex; topics: (Hex | null)[]; fromBlock: Hex; toBlock: "finalized" }) {
    return this.rpc<Log[]>("eth_getLogs", [filter]);
  }
}
