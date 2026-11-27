// Executed only in the main document. The native side supplies the actual origin and document identity.
export const browserProvider = `(() => {
  if (window.top !== window || Object.prototype.hasOwnProperty.call(window, 'ethereum')) return;
  const pending = new Map();
  let lastId = 0;
  let transport = null;
  function answer(packet) {
    let reply;
    try { reply = JSON.parse(packet); } catch { return; }
    const operation = pending.get(reply.id);
    if (!operation) return;
    pending.delete(reply.id);
    clearTimeout(operation.timeout);
    if (reply.error) {
      const refused = new Error(reply.error.message);
      refused.code = reply.error.code;
      operation.reject(refused);
    } else operation.resolve(reply.result);
  }
  function attach(send) {
    if (transport) return;
    transport = send;
    window.dispatchEvent(new Event('ethereum#initialized'));
  }
  Object.defineProperty(window, 'nu54WalletReply', {value:answer, writable:false});
  Object.defineProperty(window, 'ethereum', {value:Object.freeze({
    isConnected: () => transport !== null,
    request: ({method, params = []}) => new Promise((resolve, reject) => {
      if (!transport || pending.size >= 4 || typeof method !== 'string' || method.length > 48 || !Array.isArray(params)) {
        reject(Object.assign(new Error('Wallet session unavailable'), {code:4900})); return;
      }
      const id = ++lastId;
      const body = JSON.stringify({id,method,params});
      if (body.length > 4096) { reject(Object.assign(new Error('Request is too large'), {code:4200})); return; }
      const timeout = setTimeout(() => {
        pending.delete(id);
        reject(Object.assign(new Error('Wallet request expired'), {code:4900}));
      }, (method === 'personal_sign' || method === 'eth_sendTransaction') ? 120000 : 30000);
      pending.set(id,{resolve,reject,timeout});
      transport(body);
    }),
  }), writable:false});
  if (window.webkit?.messageHandlers?.nu54Wallet) {
    attach(body => window.webkit.messageHandlers.nu54Wallet.postMessage(body));
  } else {
    window.addEventListener('message', function ready(event) {
      if (event.data !== 'Nu54WalletProvider' || event.ports.length !== 1) return;
      if (event.origin !== '' && event.origin !== 'null' && event.origin !== window.location.origin) return;
      const port = event.ports[0];
      window.removeEventListener('message', ready);
      port.onmessage = message => answer(message.data);
      attach(body => port.postMessage(body));
    });
  }
})();`;
