// Synthetic native boundary. The app transport code and firmware approvals are real.
import assert from "node:assert/strict";
import { createHash, randomBytes, randomUUID } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { setTimeout as delay } from "node:timers/promises";
import { request as httpsRequest } from "node:https";
import { createRequire, registerHooks } from "node:module";

const root = new URL("../../../../", import.meta.url);
const require = createRequire(
  new URL("products/p04-merchant-kiosk/package.json", root)
);
const ts = require("typescript");
registerHooks({
  load(url, context, next) {
    if (
      !url.startsWith(root.href) ||
      !url.endsWith(".ts") ||
      url.includes("/node_modules/")
    )
      return next(url, context);
    const source = readFileSync(new URL(url), "utf8");
    return {
      format: "module",
      shortCircuit: true,
      source: ts.transpileModule(source, {
        compilerOptions: {
          module: ts.ModuleKind.ESNext,
          target: ts.ScriptTarget.ES2022,
        },
      }).outputText,
    };
  },
});
const {
  prepareCommercePayment,
  preparePresentedPayment,
  paymentProfile,
  commerceAmount,
  hexToBytes,
  toBase64,
  fromBase64,
  V2Link,
  V2Journal,
  V2StreamDecoder,
  encodeV2StreamFrame,
} = await import(new URL("packages/protocol/ts/src/index.ts", root));
const { OwnerSession } = await import(
  new URL("products/p02-user-app/src/wallet/ownerSession.ts", root)
);
const { boardPayment } = await import(
  new URL("products/p04-merchant-kiosk/src/kiosk/boardPayment.ts", root)
);
const input = JSON.parse(readFileSync(0, "utf8"));
const now = () => Math.floor(Date.now() / 1000);
const peer = "09".repeat(32),
  device = "02".repeat(32);
const noop = () => {};
const subscription = () => ({ remove: noop });

function syntheticBoard(profile, payment, signature, epoch, status) {
  const server = new V2Link(
    profile,
    "board",
    epoch,
    hexToBytes(peer),
    new V2Journal(8)
  );
  const stream = new V2StreamDecoder();
  return (raw) => {
    const output = [];
    stream.feed(fromBase64(raw), (frame) => {
      const result = server.receive(frame, BigInt(now()));
      if (!result) return;
      const { request } = result;
      let response;
      if (request.command === "manage") {
        assert.deepEqual(request.payload, Uint8Array.of(3));
        response = status;
      } else {
        assert.equal(request.command, "pay.prepare");
        assert.deepEqual(request.payload, payment.bytes);
        response = hexToBytes(signature);
      }
      for (const part of server.send({
        command: "reply",
        id: request.id,
        expires: request.expires,
        payload: response,
      }))
        output.push(encodeV2StreamFrame(part));
    });
    return output.length ? toBase64(Buffer.concat(output)) : "";
  };
}

if (input.forward) {
  await forward(input.forward);
} else {
  const enrolled = paymentProfile(JSON.stringify({ payment: input.profile }));
  if (input.humanUnits) {
    const expected = {
      "-1": "10.00 TestUSDC",
      0: "10.00 TestUSDC",
      1: "20.00 TestUSDC",
      2: "6.50 TestUSDC",
    }[input.approvalIndex];
    assert.equal(
      commerceAmount(
        input.current.order.amount,
        input.current.order.asset,
        enrolled
      ),
      expected
    );
    assert.equal(
      commerceAmount("27000000", input.current.order.asset, enrolled),
      "27.00 TestUSDC"
    );
  }
  const item = input.current;
  const prepared = item.quote.presentation
    ? preparePresentedPayment(item.quote, item.order, input.profile, now(), {
        branch: input.branch,
        sale: item.order.id,
        customer: item.actor,
        origin: item.quote.presentation.origin,
        claim: item.quote.presentation.claim,
      })
    : prepareCommercePayment(
        item.quote,
        item.order,
        input.profile,
        now(),
        input.branch
      );
  const identity = new Uint8Array(91).fill(5);
  const status = new Uint8Array(77);
  status.set([3, 1, 1, 1, 1], 0);
  status[12] = 1;
  status.set(hexToBytes(device), 13);
  status.set(createHash("sha256").update(identity).digest(), 45);

  function ownerSession() {
    const epoch = randomBytes(16);
    const deviceSend = syntheticBoard(
      "management",
      prepared,
      item.signature,
      epoch,
      status
    );
    return new OwnerSession(
      {
        connectOwner: async () => 128,
        onFragment: subscription,
        onDisconnect: subscription,
        writeFragment: async () => {},
        disconnect: async () => {},
      },
      {
        onTls: subscription,
        startTls: async () => JSON.stringify({ epoch: toBase64(epoch), peer }),
        sendTls: async (raw) => JSON.stringify({ clear: deviceSend(raw) }),
        stopTls: async () => {},
        randomBytes: async () => toBase64(randomBytes(16)),
        publicIdentity: async () => toBase64(identity),
      },
      {
        version: 2,
        device,
        boardSPKI: peer,
        ca: "AQ==",
        certificateChain: ["AQ=="],
      }
    );
  }
  if (input.approvalIndex === -1) {
    const preparing = ownerSession();
    await preparing.connect("fixture-board");
    preparing.close();
    process.stdout.write(JSON.stringify("synthetic-owner-session-ready"));
  } else if (input.approvalIndex === 0) {
    const phone = ownerSession();
    let presentations = 0;
    await phone.connect("fixture-board");
    const ownerSignature = await phone.approvePayment(
      prepared,
      hexToBytes(item.request),
      () => {
        presentations++;
      }
    );
    phone.close();

    assert.equal(presentations, 1);
    process.stdout.write(JSON.stringify(ownerSignature));
  } else {
    let presentations = 0;
    let notify;
    const epoch = randomBytes(16);
    const deviceSend = syntheticBoard(
      "payment",
      prepared,
      item.signature,
      epoch
    );
    const kioskSignature = await boardPayment(
      {
        onPayment: (listener) => {
          notify = listener;
          return subscription();
        },
        connectPayment: async () => {
          notify(
            JSON.stringify({
              kind: "ready",
              epoch: toBase64(epoch),
              peer,
              principal: item.actor,
              branch: input.branch,
              link: "fixture-board",
              expiresAt: now() + 30,
            })
          );
          return "fixture-board";
        },
        writePayment: async (link, raw) => {
          const clear = deviceSend(raw);
          if (clear) notify(JSON.stringify({ kind: "data", link, clear }));
        },
        closePayment: async () => {},
      },
      "fixture-board",
      input.branch,
      prepared,
      hexToBytes(item.request),
      () => {
        presentations++;
      },
      new AbortController().signal
    );
    assert.equal(presentations, 1);
    process.stdout.write(JSON.stringify(kioskSignature));
  }
}

async function forward(value) {
  const { CommerceClient, NearbyCommerceLink, encodeOrderLocator } =
    await import(new URL("packages/protocol/ts/src/index.ts", root));
  const { CustomerNearby } = await import(
    new URL("products/p02-user-app/src/merchant/nearbyClient.ts", root)
  );
  const { MerchantInbox } = await import(
    new URL("products/p04-merchant-kiosk/src/kiosk/merchantInbox.ts", root)
  );
  const { NearbyRelay } = await import(
    new URL("products/p04-merchant-kiosk/src/kiosk/nearbyRelay.ts", root)
  );
  const exchange = (identity) => (request) =>
    new Promise((resolve, reject) => {
      const call = httpsRequest(
        new URL("/v2/commerce", value.url),
        {
          method: "POST",
          ca: value.server,
          cert: identity.cert,
          key: identity.key,
          headers: { "Content-Type": "application/json" },
          timeout: 4000,
        },
        (response) => {
          const chunks = [];
          response.on("data", (chunk) => chunks.push(chunk));
          response.on("end", () =>
            resolve(Buffer.concat(chunks).toString("utf8"))
          );
          response.on("error", reject);
        }
      );
      call.on("timeout", () => call.destroy(new Error("C3_TIMEOUT")));
      call.on("error", reject);
      call.end(request);
    });
  const appProfile = (identity, role) => ({
    tenantRef: value.tenant,
    subjectRef: identity.subject,
    role,
    branchRef: value.branch,
    expiresAt: now() + 600,
  });
  const customer = new CommerceClient(
    appProfile(value.customer, "customer"),
    exchange(value.customer),
    async () => randomUUID()
  );
  const terminal = new CommerceClient(
    appProfile(value.terminal, "terminal"),
    exchange(value.terminal),
    async () => randomUUID()
  );
  const { parseOrderLocator } = await import(
    new URL("products/p02-user-app/src/qr.ts", root)
  );
  const qr = encodeOrderLocator(
    { branchRef: value.branch, orderRef: value.sale, expiresAt: now() + 35 },
    now()
  );
  const located = parseOrderLocator(qr, now());
  assert.equal(located.branchRef, value.branch);
  // The registered original is already paired. A duplicate scan resumes
  // the authorized claim and the kiosk no longer lists temporary candidates.
  const confirmed = await customer.claim(located.orderRef);
  assert.ok(confirmed);
  assert.equal(confirmed.claimDigest, value.claimDigest);
  assert.deepEqual(await terminal.pendingComparisons(located.orderRef), []);
  assert.deepEqual(
    await terminal.posNearbyCheckout(located.orderRef, value.customer.subject, randomUUID(), "resume"),
    { claim: confirmed }
  );
  let disk = "";
  const inbox = await MerchantInbox.open(
    {
      getSetting: async () => disk,
      putSetting: async (_key, contents) => {
        disk = contents;
      },
    },
    value.branch,
    value.terminal.subject
  );
  await inbox.grantClaim(value.sale, value.customer.subject, now() + 55);
  const publicHandle = inbox.openPublic(randomUUID());
  const publicIntent = randomUUID();
  await inbox.stage(
    publicHandle,
    {
      origin: "pos",
      originRef: publicIntent,
      menuVersion: 1,
      items: [{ sku: "sample", option_key: "plain", quantity: 1 }],
    },
    value.asset,
    publicIntent
  );
  // The long-running state owns an existing synthetic paid-path sale. The
  // fixture replaces only this initial external order attachment.
  await inbox.dispatch(publicHandle, async () => value.publicSale);
  assert.equal(inbox.publicOrder(), value.publicSale);
  const started = Date.now();
  const menuActivities = [];
  const selectSample = async () => {
    const menu = await terminal.catalog(0);
    assert.ok(menu.find((item) => item.sku === "sample" && item.available > 0));
    menuActivities.push(Date.now());
  };
  await selectSample();

  const relay = new NearbyRelay(
    terminal,
    inbox,
    async () => randomBytes(16),
    () => inbox.publicOrder(),
    noop
  );
  const nearbyEpoch = toBase64(randomBytes(16));
  const ready = (principal) => ({
    kind: "ready",
    link: "fixture-nearby",
    principal,
    branch: value.branch,
    expiresAt: now() + 55,
    epoch: nearbyEpoch,
    peer,
  });
  let notify;
  const merchantLink = new NearbyCommerceLink(
    ready(value.customer.subject),
    "terminal",
    {
      write: async (link, clear) => {
        notify(JSON.stringify({ kind: "data", link, clear }));
      },
      close: async () => {},
    },
    new V2Journal(32),
    (request, peerInfo) => relay.handle(request, peerInfo)
  );
  const phone = new CustomerNearby(
    {
      onNearby: (listener) => {
        notify = listener;
        return subscription();
      },
      connect: async () => {
        notify(JSON.stringify(ready(value.terminal.subject)));
        return "fixture-nearby";
      },
      write: async (_link, clear) => merchantLink.receive(clear),
      close: async () => {},
    },
    value.branch
  );
  const phoneLink = await phone.connect("fixture-nearby");
  const routes = [];
  try {
    for (let n = 0; n < 10; n++) {
      assert.equal(
        await customer.forward(
          value.sale,
          value.quoteDigest,
          value.approvalDigest,
          value.signature,
          value.requestKey
        ),
        value.sale
      );
      routes.push("mTLS");
      const reply = await phoneLink.request(
        {
          op: "forward",
          branch: value.branch,
          order: value.sale,
          key: value.requestKey,
          quoteDigest: value.quoteDigest,
          approvalDigest: value.approvalDigest,
          signature: value.signature,
        },
        randomBytes(16)
      );
      assert.deepEqual(reply, {
        status: "ok",
        value: { submitted: value.sale },
      });
      routes.push("nearby-wire");
    }
  } finally {
    phone.close();
    merchantLink.close();
  }
  const forwardedAt = Date.now();
  assert.equal(inbox.publicOrder(), value.publicSale);
  writeFileSync(value.readyFile, JSON.stringify(routes), { mode: 0o600 });
  while (Date.now() - started < value.holdMS) {
    await delay(Math.min(250, value.holdMS));
    assert.equal(inbox.publicOrder(), value.publicSale);
    if (Date.now() - menuActivities.at(-1) >= 50000) await selectSample();
  }
  const ended = Date.now();
  inbox.endPublic();
  assert.equal(inbox.publicOrder(), undefined);
  process.stdout.write(
    JSON.stringify({
      started,
      forwardedAt,
      ended,
      publicSale: value.publicSale,
      privateSale: value.sale,
      routes,
      menuActivities,
    })
  );
}
