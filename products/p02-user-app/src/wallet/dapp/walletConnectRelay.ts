export type RelayMessage={topic:string;message:string;tag:number};
export type Relay={
  subscribe(topic:string):Promise<void>;
  publish(topic:string,message:string,tag:number,ttl:number):Promise<void>;
  listen(receive:(message:RelayMessage)=>void):void;
  close():void;
};

export class IrnRelay implements Relay {
  private serial=0;
  private readonly subscriptions=new Map<string,string>();
  private readonly pending=new Map<string,{done:(result:unknown)=>void;fail:(error:Error)=>void}>();
  private readonly connected:Promise<void>;
  private receiver?:(message:RelayMessage)=>void;
  private readonly socket:WebSocket;
  private closed=false;

  constructor(endpoint:string) {
    this.socket=new WebSocket(endpoint);
    this.connected=new Promise((resolve,reject)=>{
      const timeout=setTimeout(()=>{this.close();reject(new Error('WC_RELAY_TIMEOUT'));},8000);
      this.socket.onopen=()=>{clearTimeout(timeout);resolve();};
      this.socket.onclose=()=>{clearTimeout(timeout);reject(new Error('WC_RELAY_CLOSED'));this.close();};
      this.socket.onerror=()=>{clearTimeout(timeout);reject(new Error('WC_RELAY_CONNECTION'));this.close();};
    });
    this.connected.catch(()=>{}); // A connection can close before the first subscription.
    this.socket.onmessage=event=>{
      if (typeof event.data!=='string' || event.data.length>15000) return;
      try {
        const packet=JSON.parse(event.data);
        if (packet.jsonrpc!=='2.0' || !(/^\d{1,19}$/.test(String(packet.id)))) return;
        if (packet.method==='irn_subscription') {
          const data=packet.params?.data;
          if (!data || typeof data.topic!=='string' || this.subscriptions.get(data.topic)!==packet.params.id ||
              typeof data.message!=='string' || !Number.isSafeInteger(data.tag)) return;
          this.socket.send(JSON.stringify({jsonrpc:'2.0',id:packet.id,result:true}));
          this.receiver?.({topic:data.topic,message:data.message,tag:data.tag});
          return;
        }
        const operation=this.pending.get(String(packet.id));
        if (!operation) return;
        this.pending.delete(String(packet.id));
        if (packet.error || !Object.hasOwn(packet,'result')) operation.fail(new Error('WC_RELAY_REJECTED'));
        else operation.done(packet.result);
      } catch {
        // A malformed relay packet has no session authority.
      }
    };
  }

  private async rpc(method:string,params:object) {
    await this.connected;
    if (this.closed || this.pending.size===8) throw new Error('WC_RELAY_BUSY');
    const id=String(++this.serial);
    return new Promise<unknown>((resolve,reject)=>{
      const timeout=setTimeout(()=>{this.pending.delete(id);reject(new Error('WC_RELAY_TIMEOUT'));},8000);
      this.pending.set(id,{done:value=>{clearTimeout(timeout);resolve(value);},fail:error=>{clearTimeout(timeout);reject(error);}});
      try { this.socket.send(JSON.stringify({jsonrpc:'2.0',id,method,params})); }
      catch {clearTimeout(timeout);this.pending.delete(id);reject(new Error('WC_RELAY_SEND'));}
    });
  }

  async subscribe(topic:string) {
    const id=await this.rpc('irn_subscribe',{topic});
    if (typeof id!=='string' || id.length<1 || id.length>128) throw new Error('WC_RELAY_SUBSCRIPTION');
    this.subscriptions.set(topic,id);
  }

  async publish(topic:string,message:string,tag:number,ttl:number) {
    if (await this.rpc('irn_publish',{topic,message,tag,ttl,prompt:false})!==true) throw new Error('WC_RELAY_PUBLISH');
  }

  listen(receive:(message:RelayMessage)=>void) {this.receiver=receive;}
  close() {
    if (this.closed) return;
    this.closed=true;
    this.receiver=undefined;
    for (const operation of this.pending.values()) operation.fail(new Error('WC_RELAY_CLOSED'));
    this.pending.clear();
    this.subscriptions.clear();
    this.socket.close();
  }
}
