import React from 'react';
import {AppState, type AppStateStatus} from 'react-native';
import Renderer from 'react-test-renderer';
import {readFileSync} from 'node:fs';
import {resolve as resolvePath} from 'node:path';
import {ed25519} from '@noble/curves/ed25519';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha2';
import {
  bindServiceRequestV2 as mockBind, decodeNearbyRequest,
  addressOfPrivateKey, bigToBytes, bytesToBig, bytesToHex, concat, hexToBytes, preparePresentedPayment, presentationMessage, gasSponsorMessage, rentalEscrowMessage, rentalReviewAuthorityMessage, utf8Encode, quoteDigest, signDigest,
  encodeOrderLocator, packNearbyPresentation as mockPack, quoteV2Layout as q, toBase64 as mockBase64,
  type CommerceClient, type CommerceOrder, type CommercePrice, type CommerceQuote, type NearbyReady, type RentalReviewAuthority, type RentalReviewChallenge, type NearbyRequest, type ServiceRequestV2,
} from '@nu54/protocol';
import {CustomerOrders} from '../src/merchant/CustomerOrders';
import type {OwnerApprovalProgress} from '../src/wallet/ownerSession';
import type {TravelBookingPayment} from '../src/travel/BookingPaymentTerms';
import {NearbyRelay} from '../../p04-merchant-kiosk/src/kiosk/nearbyRelay';
import type {MerchantInbox} from '../../p04-merchant-kiosk/src/kiosk/merchantInbox';

const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea';
const customer = '0fbc54ef-2827-4ab5-bfcd-3a27611c39eb';
const sale = 'f0551c76-7592-4850-a414-54464d24dafe';
const [, , , unsigned, , clock] = readFileSync(
  resolvePath(__dirname, '../../../packages/protocol/v2/testdata/quote.vectors'), 'utf8',
).split('\n')[1].split('|');
const raw = hexToBytes(unsigned);
const merchantKey = new Uint8Array(32).fill(31);
const payerKey = new Uint8Array(32).fill(32);
raw.set(hexToBytes(addressOfPrivateKey(payerKey)), q.payerOffset + 12);
const word = (offset: number) => raw.subarray(offset, offset + 32);
const address = (offset: number) => bytesToHex(word(offset).subarray(12));
const value = (offset: number) => bytesToBig(word(offset));
const now = Number(clock);

const payer = address(q.payerOffset);
const generation = value(q.payerGenerationOffset);
const presentationSeed = new Uint8Array(32).fill(44);
const policy = {
  chain: 8283, contract: address(q.verifyingContractOffset), asset: address(q.assetOffset),
  beneficiary: address(q.beneficiaryOffset), merchantId: bytesToHex(word(q.merchantIdOffset), false),
  signer: addressOfPrivateKey(merchantKey), keyVersion: Number(value(q.merchantKeyVersionOffset)),
  beneficiaryVersion: Number(value(q.beneficiaryVersionOffset)), maxBaseUnits: '1000000',
  presentationKey: bytesToHex(ed25519.getPublicKey(presentationSeed), false),
};
const orderKey = bytesToHex(word(q.orderIdOffset), false);
function mockQuoteFor(statedAmount: bigint, origin = sale, payerRole = raw[q.payerRoleOffset + 31]) {
  const words = raw.slice();
  words.set(bigToBytes(statedAmount, 32), q.amountOffset);
  words[q.payerRoleOffset + 31] = payerRole;
  const digest = quoteDigest(words);
  const quote = {
    saleRef: sale, branchRef: branch, orderKey, quoteDigest: bytesToHex(digest, false),
    signedQuote: bytesToHex(concat(words, signDigest(digest, merchantKey)), false),
    expiresAt: Number(value(q.expiresAtOffset)),
    presentation: {customer, origin, claim: 'a'.repeat(64), signature: ''},
  };
  quote.presentation.signature = bytesToHex(ed25519.sign(presentationMessage(quote, generation), presentationSeed), false);
  return quote;
}
const quote = mockQuoteFor(value(q.amountOffset));
const order: CommerceOrder = {
  id: sale, amount: value(q.amountOffset).toString(), asset: policy.asset, state: 'issued', revision: 1,
  expiresAt: new Date(quote.expiresAt * 1000).toISOString(), quoteDigest: quote.quoteDigest, orderKey,
};
const mockProfile = {scope: {role: 'customer', branchRef: branch, subjectRef: customer, tenantRef: branch,
  expiresAt: now + 120}, pin: 'a'.repeat(64), payment: policy};
const pending = {
  version: 2, branch, customer, order, quote, scope: {branch, customer, sale, origin: sale, claim: 'a'.repeat(64)},
  request: 'c'.repeat(32), submitKey: sale, route: 'direct',
};

const mockReviewKey = new Uint8Array(32).fill(17);
const mockReviewPublic = p256.getPublicKey(mockReviewKey, false);
const mockBoardPin = bytesToHex(sha256(concat(
  hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200'), mockReviewPublic,
)), false);
let mockRentalRelay: NearbyRelay | undefined;
let mockRentalDecisionOverride: string | undefined;
const mockPhonePeer: NearbyReady = {kind: 'ready', link: 'fixture-renter-link', epoch: 'fixture',
  peer: 'a'.repeat(64), principal: customer, branch, expiresAt: now + 60};
function mockReviewReceipt(request: RentalReviewChallenge) {
  const signed = hexToBytes(mockQuote.signedQuote);
  const body = concat(utf8Encode('W10R'), hexToBytes(mockBoardPin), new Uint8Array(32).fill(10),
    new Uint8Array(16).fill(11), hexToBytes(request.request), hexToBytes(request.challenge),
    hexToBytes(orderKey), sha256(signed), hexToBytes(payer), bigToBytes(generation, 8),
    bigToBytes(BigInt(mockQuote.expiresAt), 8));
  const digest = sha256(concat(utf8Encode('NU54 W10 rental payment review v1\0'), body));
  return bytesToHex(concat(body, p256.sign(digest, mockReviewKey, {prehash: false, lowS: true}).toCompactRawBytes()), false);
}

function mockDispatchRentalReview(request: NearbyRequest) {
  const decoded = decodeNearbyRequest(utf8Encode(JSON.stringify(request)), mockPhonePeer.branch);
  if (decoded.op === 'rental-decision' && mockRentalDecisionOverride !== undefined)
    return {status: 'ok', value: {decision: mockRentalDecisionOverride}};
  return mockRentalRelay?.handle(decoded, mockPhonePeer) ?? {status: 'denied'};
}

function mockQuoteWithReview(rentalReview: RentalReviewChallenge) {
  const proof: RentalReviewAuthority = {...rentalReview, boardPin: mockBoardPin,
    terminalPin: '0a'.repeat(32), signer: bytesToHex(mockReviewPublic, false), expiresAt: now + 5, signature: ''};
  const reviewed = {...mockQuote, presentation: {...mockQuote.presentation!, rentalReview: proof}};
  proof.signature = bytesToHex(ed25519.sign(rentalReviewAuthorityMessage(reviewed, generation), presentationSeed), false);
  return reviewed;
}
const mockJournals = new Map<string, string>();
const mockCalls: string[] = [];
const mockOrderByID = new Map<string, CommerceOrder>();
const mockConnect = jest.fn(async (_handle: string) => {});
const mockClose = jest.fn();
const mockApprove = jest.fn<Promise<string>, [unknown, Uint8Array, () => void, ((value:OwnerApprovalProgress|undefined)=>void)?]>();
const mockProgressNegotiation=jest.fn(async()=>{});
const mockMaintenanceRead=jest.fn(async()=>({device:'pinned',generation,owner:'owner',state:'active',backedUp:true}));
const mockCampaign=jest.fn(async()=> 'same-board-firmware-proof');
let mockOrder = order;
let mockQuote: CommerceQuote = quote;
let mockForwardLost = false;
let mockShopBranch = branch;
let mockStatusOffline = false;
let mockShopWait: Promise<void> | undefined;
let mockPickupCatalog: CommercePrice[] | undefined;
let mockExtraOrder: CommerceOrder | undefined;
let mockRelayStatus: CommerceOrder | undefined;
let mockNearbyEnabled = false;
let mockNearbySponsored = false;
let mockNearbyGrant = 'a'.repeat(64);
let mockNearbyWait: Promise<void> | undefined;
const mockNearbyRequests: NearbyRequest[] = [];
const mockForwardRequests: (ServiceRequestV2 & {approvalSignature?: string})[] = [];
const mockWallet = {mode: 'personal', enrollment: {device: 'pinned',boardSPKI:'cd'.repeat(32),certificateChain:['MA==']}, verified: {network: '8283', address: payer,
  generation: generation.toString(), boardFingerprint: 'pinned'}};

function mockNearbyQuote(amount: bigint, origin: string, sponsored = mockNearbySponsored, payerRole?: number): import('@nu54/protocol').CommerceQuote {
  const presentation: import('@nu54/protocol').CommerceQuote = mockQuoteFor(amount, origin, payerRole);
  if (sponsored) {
    const sponsor = {account:'0x'+'8'.repeat(40), expiresAt:now+5, grant:mockNearbyGrant, signature:''};
    presentation.presentation!.gasSponsor = sponsor;
    sponsor.signature = bytesToHex(ed25519.sign(gasSponsorMessage(presentation,generation),presentationSeed),false);
  }
  return presentation;
}

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: async () => JSON.stringify({...mockProfile, nearby: mockNearbyEnabled ? {} : undefined}),
    loadJournal: async (name: string) => mockJournals.get(name) ?? '',
    saveJournal: async (name: string, journal: string) => {mockJournals.set(name, journal);},
    randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(7)),
    exchange: async (_role: string, rawRequest: string) => {
      const {command, rentalReview, approvalSignature} = JSON.parse(rawRequest) as {
        command: ServiceRequestV2; rentalReview?: RentalReviewChallenge; approvalSignature?: string;
      };
      mockCalls.push(command.operation);
      const {requestId, requestDigest} = mockBind(command, {...mockProfile.scope,
        grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}],
      }, Math.floor(Date.now() / 1000));
      const ok = {requestId, requestDigest, status: 'ok', resultRef: command.objectRef};
      if (command.operation === 'order.list') {
        const {id, amount, asset, state} = mockOrder;
        const originals = [{id, amount, asset, state}];
        if (mockExtraOrder) {
          const {id: extra, amount: total, asset: token, state: stage} = mockExtraOrder;
          originals.push({id: extra, amount: total, asset: token, state: stage});
        }
        return JSON.stringify({...ok, orders: originals.sort((a, b) => a.id.localeCompare(b.id))});
      }
      if (command.operation === 'catalog.read') {
        const branch = mockShopBranch;
        await mockShopWait;
        return JSON.stringify({...ok, shop: {branch, branchLabel: '여울 커피 성수점'},
          ...(mockPickupCatalog ? {catalog: mockPickupCatalog, catalogVersion: mockPickupCatalog[0]?.version ?? 1} : {})});
      }
      if (command.operation === 'order.create' && mockPickupCatalog) return JSON.stringify({...ok, resultRef: sale});
      if (command.operation === 'invoice.list') return JSON.stringify({...ok, invoices: []});
      if (command.operation === 'order.status') {
        if (mockStatusOffline) throw new Error('offline');
        return JSON.stringify({...ok, order: mockOrderByID.get(command.objectRef!) ?? mockOrder});
      }
      if (command.operation === 'checkout.claim') return JSON.stringify({...ok, claim: {
        claimDigest: 'a'.repeat(64), expiresAt: Math.floor(Date.now() / 1000) + 60,
      }});
      if (command.operation === 'quote.issue') {
        if (!rentalReview) return JSON.stringify({...ok, quote: mockQuote});
        return JSON.stringify({...ok, quote: mockQuoteWithReview(rentalReview)});
      }
      if (command.operation === 'payment.forward') {
        mockForwardRequests.push({...command, approvalSignature});
        if (mockForwardLost) throw new Error('connection lost after dispatch');
        return JSON.stringify(ok);
      }
      throw new Error('unexpected commerce operation');
    },
  },
}));
jest.mock('../src/specs/NativeNearby.ts', () => ({__esModule: true, default: {}}));
jest.mock('../src/merchant/nearbyClient.ts', () => ({
  CustomerNearby: class {
    close() {}
    async scan() { return [{handle: 'terminal'}]; }
    async connect() {
      return {
        peer: {principal: sale},
        request: async (request: NearbyRequest) => {
          mockNearbyRequests.push(request);
          if (request.op === 'discover')
            return {status: 'ok', value: {branch, terminal: sale, branchLabel: '여울 커피 성수점'}};
          if (request.op === 'catalog')
            return {status: 'ok', value: {rows: [{
              sku: 'coffee', optionKey: 'iced', label: '커피', asset: mockOrder.asset,
              version: 1, unitPrice: mockOrder.amount, available: 1, expiresAt: mockOrder.expiresAt,
            }]}};
          if (request.op === 'rental-review-source' || request.op === 'rental-review' || request.op === 'rental-decision')
            return mockDispatchRentalReview(request);
          const reservation = mockNearbyRequests.find(candidate => candidate.op === 'reserve');
          if (!reservation) throw Error('MISSING_TEST_CART');
          if (request.op === 'reserve') {
            await mockNearbyWait;
            return {status: 'ok', value: {...mockOrder, state: 'preview', revision: 0}};
          }
          const presentation = mockNearbyQuote(BigInt(mockOrder.amount), reservation.cart.originRef);
          const issued = {...mockOrder, quoteDigest: presentation.quoteDigest};
          if (request.op === 'status') return {status: 'ok', value: mockRelayStatus ?? issued};
          if (request.op === 'checkout') {
            if (mockNearbyRequests.filter(req => req.op === 'checkout').length === 1)
              throw new Error('NEARBY_DISCONNECTED');
            return {status: 'ok', value: mockPack(issued, presentation, mockBase64)};
          }
          throw new Error('UNEXPECTED_TEST_REQUEST');
        },
      };
    }
  },
}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {loadProfile: async () => ''}}));
jest.mock('../src/specs/NativeRenterBle.ts', () => ({
  __esModule: true, default: {
    onScan: (receive: (raw: string) => void) => {
      receive(JSON.stringify({address: 'fixture', name: '시험 보드'}));
      return {remove() {}};
    },
    scanOwner: async () => {},
  },
}));
jest.mock('../src/wallet/profile.ts', () => ({decodeWalletStorage: () => mockWallet}));
jest.mock('../src/wallet/ownerSession.ts', () => ({
  OwnerSession: class {
    connect = mockConnect;
    close = mockClose;
    approvePayment = mockApprove;
    negotiateApprovalProgress = mockProgressNegotiation;
    maintenanceStatus = mockMaintenanceRead;
    firmwareProof = mockCampaign;
  },
}));

let tree: Renderer.ReactTestRenderer | undefined;
let appState: ((state: AppStateStatus) => void) | undefined;
const travelPayment: TravelBookingPayment = {
  scope: {branch, customer, tenant: branch},
  kind: 'deposit',
  booking: {
    id: '0fbc54ef-2827-4ab5-bfcd-3a27611c39ef', slot: '0fbc54ef-2827-4ab5-bfcd-3a27611c39ed',
    partySize: 1, state: 'pending_payment', expiresAt: order.expiresAt, asset: order.asset,
    deposit: order.amount, balance: '20000000', termsVersion: 3, priceVersion: 2, chainId: 8283,
    cancellation: '취소 조건', noShow: '미방문 시 예약금 환불 불가', cancellationDeadline: order.expiresAt,
  },
};
async function open(bookingPayment?: TravelBookingPayment, originalSale = sale, openPayments?: () => void) {
  const travelCheckout = bookingPayment && {branch, sale: originalSale, bookingPayment};
  await Renderer.act(async () => {
    tree = Renderer.create(<CustomerOrders back={() => {}} initialLocator="" permission={async () => true}
      travelCheckout={travelCheckout} openPayments={openPayments} />);
  });
}
async function press(label: string) {
  const button = tree?.root.findAll(node =>
    (node.props.label === label && typeof node.props.run === 'function') ||
    (node.props.title === label && typeof node.props.onPress === 'function'))[0];
  expect(button).toBeDefined();
  expect(button?.props.disabled).toBeFalsy();
  await Renderer.act(async () => {(button?.props.run ?? button?.props.onPress)();});
}
const screen = () => JSON.stringify(tree?.toJSON());

beforeEach(() => {
  jest.spyOn(Date, 'now').mockReturnValue(now * 1000);
  jest.spyOn(AppState, 'addEventListener').mockImplementation((_, callback) => {
    appState = callback;
    return {remove() {}};
  });
  mockCalls.length = 0;
  mockOrderByID.clear();
  mockForwardRequests.length = 0;
  mockForwardLost = false;
  mockShopBranch = branch;
  mockStatusOffline = false;
  mockShopWait = undefined;
  mockPickupCatalog = undefined;
  mockExtraOrder = undefined;
  mockRelayStatus = undefined;
  mockNearbyEnabled = false;
  mockNearbySponsored = false;
  mockNearbyGrant = 'a'.repeat(64);
  mockNearbyWait = undefined;
  mockRentalRelay = undefined;
  mockRentalDecisionOverride = undefined;
  mockNearbyRequests.length = 0;
  mockJournals.clear();
  mockConnect.mockClear();
  mockClose.mockClear();
  mockApprove.mockReset();
  mockProgressNegotiation.mockClear();
  mockMaintenanceRead.mockClear();
  mockCampaign.mockClear();
  mockOrder = order;
  mockQuote = quote;
  mockJournals.set('customer-payment-v2', JSON.stringify(pending));
});

test('T06 stops changed menu terms and only reopens after explicit current review', async () => {
  mockJournals.clear();
  const pickup = {window: sale, startsAt: order.expiresAt, label: '다음 픽업 시간'};
  const selected: CommercePrice = {sku: 'coffee', optionKey: 'milk', label: '커피', unitPrice: '3500000',
    asset: policy.asset, version: 1, available: 3, expiresAt: order.expiresAt,
    metadata: {optionLabels: {ko: '우유'}}};
  mockPickupCatalog = [selected];
  mockOrder = {...order, amount: '4500000', state: 'preview'};
  await Renderer.act(async () => {
    tree = Renderer.create(<CustomerOrders back={() => {}} initialLocator="" permission={async () => true}
      travelCheckout={{branch, pickup}} />);
  });
  await press('지점 메뉴 보기');
  await press('커피 우유 추가');
  mockPickupCatalog = [{...selected, version: 2, metadata: {optionLabels: {ko: '귀리 음료'}}}];
  await press('픽업 시간과 결제 금액 확인 후 주문 접수');
  expect(screen()).toContain('재고 또는 가격이 변경됐어요');
  expect(screen()).toContain('우유 → 귀리 음료');
  expect(mockCalls).not.toContain('order.create');
  expect(mockCalls).not.toContain('quote.issue');
  expect(mockCalls).not.toContain('payment.forward');
  expect(mockApprove).not.toHaveBeenCalled();
  mockPickupCatalog = [{...mockPickupCatalog[0], version: 3, unitPrice: '4500000'}];
  await press('변경된 옵션과 금액 확인 후 주문 접수');
  expect(screen()).toContain('새 합계');
  expect(mockCalls).not.toContain('order.create');
  await press('변경된 옵션과 금액 확인 후 주문 접수');
  expect(mockCalls.filter(op => op === 'order.create')).toHaveLength(1);
  expect(mockCalls).not.toContain('quote.issue');
  expect(mockCalls).not.toContain('payment.forward');
  expect(mockApprove).not.toHaveBeenCalled();
});

test.each(['active','removed','unconfigured','replaced-after-board','expired','relay-reports-paid'] as const)('K10 resumes the original quote into W10 with %s gas sponsorship', async sponsorship => {
  if (sponsorship === 'expired') jest.useFakeTimers({now:now*1000});
  mockJournals.clear();
  mockNearbyEnabled = true;
  mockNearbySponsored = sponsorship !== 'unconfigured';
  let release: () => void = () => {};
  mockNearbyWait = new Promise<void>(resolve => {release = resolve;});
  const openShop = async () => {
    await Renderer.act(async () => {
      tree = Renderer.create(<CustomerOrders back={() => {}} initialLocator="" shopEntry permission={async () => true}/>);
    });
    await press('근처 단말 찾기');
    await press('근거리 단말 후보 1 인증');
    await press('이 매장 메뉴 열기');
  };
  await openShop();
  await press('커피 iced 추가');
  await press('주문 접수 요청');
  expect(screen()).toContain('결제 견적을 확인하고 있습니다');
  expect(screen()).toContain('이 요청 다시 확인');
  await Renderer.act(async () => release());
  expect(screen()).toContain('발급 여부 미확인');
  expect(screen()).toContain(sale);
  expect(mockApprove).not.toHaveBeenCalled();
  await Renderer.act(async () => tree?.unmount());
  await openShop();
  await press('원래 요청 재조회');
  expect(screen()).toContain('원견적 상세');
  expect(screen()).toContain(sale);
  expect(screen()).toContain('커피 · iced ×1');
  const requests = mockNearbyRequests.filter(req => 'key' in req);
  expect(requests.map(req => req.op)).toEqual(['reserve', 'checkout', 'status', 'checkout']);
  expect(new Set(requests.map(req => req.key)).size).toBe(1);
  expect(requests.slice(1).every(req => 'order' in req && req.order === sale)).toBe(true);
  expect(mockApprove).not.toHaveBeenCalled();
  expect(screen()).toContain('인증한 가맹점 견적');
  if (sponsorship === 'unconfigured') {
    expect(screen()).toContain('가스 부담 근거를 받지 못했습니다');
    expect(screen()).not.toContain('검증한 보드 찾기');
    return;
  }
  expect(screen()).toContain('가스 비용 · 매장 부담');
  if (sponsorship === 'expired') {
    await Renderer.act(async () => {jest.advanceTimersByTime(5000);});
    expect(screen()).toContain('가스 부담 근거가 만료되었습니다.');
    expect(screen()).not.toContain('가스 비용 · 매장 부담');
    expect(screen()).not.toContain('검증한 보드 찾기');
    expect(mockApprove).not.toHaveBeenCalled();
    return;
  }
  expect(screen()).toContain('보드의 원견적 검증 대기');
  let boardReady: () => void = () => {};
  let decline: () => void = () => {};
  mockApprove.mockImplementation(async (_payment, _request, onBoard) => {
    boardReady = onBoard;
    await new Promise<void>(resolve => {decline = resolve;});
    throw new Error('PAYMENT_REFUSED');
  });
  if (sponsorship === 'replaced-after-board' || sponsorship === 'relay-reports-paid') {
    mockApprove.mockImplementation(async (payment, _request, onBoard) => {
      onBoard();
      const {approvalDigest} = payment as {approvalDigest:string};
      return bytesToHex(signDigest(hexToBytes(approvalDigest),payerKey),false);
    });
  }
  await press('검증한 보드 찾기');
  if (sponsorship === 'removed') mockNearbySponsored = false;
  await press('시험 보드에서 결제 검토');
  if (sponsorship === 'relay-reports-paid') {
    expect(screen()).toContain('보드 서명 확인');
    mockRelayStatus = {...order, state: 'paid', pickupNo: 4};
    mockOrder = {...order, state: 'checking', revision: 2, orderKey: undefined, quoteDigest: undefined,
      payment: {stage: 'received', chainId: 8283}};
    await press('지급 요청 제출');
    expect(screen()).toContain(`${sale} 지급 결과 확인 중`);
    expect(screen()).not.toContain('픽업 번호');
    expect(mockCalls).toContain('order.status');
    expect(mockNearbyRequests.filter(request => request.op === 'forward')).toEqual([]);
    expect(mockApprove).toHaveBeenCalledTimes(1);
    const kept = [...mockJournals.entries()].find(([name]) => /^customer-payment-[a-f0-9]{64}$/.test(name));
    expect(JSON.parse(kept?.[1] ?? '{}').authorization).toMatch(/^[a-f0-9]{130}$/);
    return;
  }
  if (sponsorship === 'replaced-after-board') {
    expect(screen()).toContain('보드 서명 확인');
    mockNearbyGrant = 'b'.repeat(64);
    await press('지급 요청 제출');
    await press('지급 요청 제출');
    expect(mockNearbyRequests.filter(request => request.op === 'forward')).toEqual([]);
    const kept = [...mockJournals.entries()].find(([name]) => /^customer-payment-[a-f0-9]{64}$/.test(name));
    expect(JSON.parse(kept?.[1] ?? '{}').quote.presentation.gasSponsor.grant).toBe('a'.repeat(64));
    return;
  }
  if (sponsorship === 'removed') {
    expect(mockConnect).not.toHaveBeenCalled();
    expect(mockApprove).not.toHaveBeenCalled();
    expect(screen()).not.toContain('가스 비용 · 매장 부담');
    expect(screen()).toContain('가스 부담 근거를 받지 못했습니다');
    expect(mockForwardRequests).toHaveLength(0);
    return;
  }
  expect(screen()).not.toContain('보드가 원견적을 검증했습니다');
  await Renderer.act(async () => boardReady());
  expect(screen()).toContain('보드가 원견적을 검증했습니다');
  await Renderer.act(async () => decline());
  expect(screen()).toContain('보드에서 결제를 거절했습니다');
  expect(screen()).toContain(sale);
  expect(screen()).not.toContain('보드가 원견적을 검증했습니다');
  expect(screen()).not.toContain('지급 요청 제출');
  expect(mockForwardRequests).toHaveLength(0);
});
afterEach(async () => {
  await Renderer.act(async () => {tree?.unmount();});
  mockRentalRelay?.close();
  tree = undefined;
  jest.useRealTimers();
  jest.restoreAllMocks();
});

test('W10 exposes only the reviewed original quote and rejects a changed amount before physical confirmation', async () => {
  await open();
  await press('원견적 상세');
  expect(screen()).toContain(policy.beneficiary);
  expect(screen()).toContain(orderKey);
  expect(mockApprove).not.toHaveBeenCalled();
  await press('견적 확인으로');
  await press('검증한 보드 찾기');
  mockQuote = mockQuoteFor(1n);
  mockOrder = {...order, quoteDigest: mockQuote.quoteDigest, amount: '1', revision: order.revision + 1};
  await press('시험 보드에서 결제 검토');
  expect(mockApprove).not.toHaveBeenCalled();
  expect(mockConnect).not.toHaveBeenCalled();
  expect(screen()).toContain('견적이 변경되었습니다');
  expect(mockCalls).not.toContain('quote.issue');
  await press('새 견적 확인');
  expect(mockCalls).toContain('quote.issue');
  expect(mockApprove).not.toHaveBeenCalled();
  expect(screen()).toContain('0.000001 TestUSDC');
  const scoped = [...mockJournals.entries()].find(([name]) => /^customer-payment-[a-f0-9]{64}$/.test(name));
  const next = JSON.parse(scoped?.[1] ?? '{}');
  expect(next.quote.quoteDigest).toBe(mockQuote.quoteDigest);
  expect(next.request).not.toBe(pending.request);
  expect(next.authorization).toBeUndefined();
});

test('W12 observes only normal authenticated approval progress and refreshes the same board after refusal',async()=>{
  const original=Object.getOwnPropertyDescriptor(AppState,'currentState')!;
  Object.defineProperty(AppState,'currentState',{configurable:true,value:'active'});
  let release!:()=>void;
  let observe:((value:OwnerApprovalProgress|undefined)=>void)|undefined;
  mockApprove.mockImplementation(async (_payment,_request,onBoard,progress)=>{
    onBoard();
    observe=progress;
    await new Promise<void>(resolve=>{release=resolve;});
    observe?.(undefined);
    throw new Error('PAYMENT_REFUSED');
  });
  try {
    await open();await press('검증한 보드 찾기');await press('시험 보드에서 결제 검토 및 기기 상태 확인');
    expect(mockProgressNegotiation).toHaveBeenCalledTimes(1);
    expect(mockApprove).toHaveBeenCalledTimes(1);
    await press('원승인 기기 상태');
    expect(screen()).toContain('승인 진행 또는 결과 미확인 · 업데이트 적용 보류');
    await Renderer.act(async()=>observe?.({device:'pinned',owner:'owner',generation,request:'a'.repeat(32),expires:BigInt(quote.expiresAt)}));
    expect(screen()).toContain('승인 진행 또는 결과 미확인 · 업데이트 적용 보류');
    await Renderer.act(async()=>observe?.({device:'pinned',owner:'owner',generation,request:pending.request,expires:BigInt(quote.expiresAt)}));
    expect(screen()).toContain('기기 승인 진행 · 업데이트 적용 보류');
    expect(screen()).not.toContain('이 기기 적용 증거 읽기');
    expect(mockMaintenanceRead).not.toHaveBeenCalled();
    expect(mockCampaign).not.toHaveBeenCalled();
    await press('원승인으로 돌아가기');
    expect(mockClose).not.toHaveBeenCalled();
    await press('원승인 기기 상태');
    expect(screen()).toContain('기기 승인 진행 · 업데이트 적용 보류');
    await Renderer.act(async()=>release());
    await press('인증된 기기 상태 조회 · 원승인 기기');
    expect(mockMaintenanceRead).toHaveBeenCalledTimes(1);
    expect(mockConnect.mock.calls).toEqual([['fixture'],['fixture']]);
    expect(mockApprove).toHaveBeenCalledTimes(1);
    expect(mockCalls).not.toContain('payment.forward');
    expect(screen()).not.toContain('업데이트 적용 보류');
  }finally{
    Object.defineProperty(AppState,'currentState',original);
  }
});

test('W12 loss after authenticated progress keeps maintenance on hold without resending the original',async()=>{
  let lose!:()=>void;
  mockApprove.mockImplementation(async (_payment,_request,onBoard,observe)=>{
    onBoard();
    observe?.({device:'pinned',owner:'owner',generation,request:pending.request,expires:BigInt(quote.expiresAt)});
    await new Promise<void>(resolve=>{lose=resolve;});
    observe?.(undefined);
    throw new Error('OWNER_DISCONNECTED');
  });
  await open();await press('검증한 보드 찾기');await press('시험 보드에서 결제 검토 및 기기 상태 확인');
  await press('원승인 기기 상태');
  expect(screen()).toContain('기기 승인 진행 · 업데이트 적용 보류');
  await Renderer.act(async()=>lose());
  expect(screen()).toContain('승인 진행 또는 결과 미확인 · 업데이트 적용 보류');
  expect(screen()).not.toContain('인증된 기기 상태 조회');
  expect(mockMaintenanceRead).not.toHaveBeenCalled();
  expect(mockApprove).toHaveBeenCalledTimes(1);
  await Renderer.act(async()=>tree?.unmount());
  tree=undefined;
  await open();
  expect(screen()).toContain('승인 대기가 끝났습니다');
  expect(screen()).not.toContain('원승인 기기 상태');
  expect(mockApprove).toHaveBeenCalledTimes(1);
  expect(mockCalls).not.toContain('payment.forward');
});

test('W12 hides an expired live observation before allowing any firmware request',async()=>{
  jest.useFakeTimers();
  jest.setSystemTime(now*1000);
  mockApprove.mockImplementation(async (_payment,_request,onBoard,observe)=>{
    onBoard();
    observe?.({device:'pinned',owner:'owner',generation,request:pending.request,expires:BigInt(now+1)});
    return new Promise<string>(()=>{});
  });
  try {
    await open();await press('검증한 보드 찾기');await press('시험 보드에서 결제 검토 및 기기 상태 확인');
    await press('원승인 기기 상태');
    await Renderer.act(async()=>jest.advanceTimersByTime(999));
    expect(screen()).toContain('기기 승인 진행 · 업데이트 적용 보류');
    await Renderer.act(async()=>jest.advanceTimersByTime(1));
    expect(screen()).toContain('승인 진행 또는 결과 미확인 · 업데이트 적용 보류');
    expect(screen()).not.toContain('이 기기 적용 증거 읽기');
    expect(mockClose).not.toHaveBeenCalled();
    expect(mockCampaign).not.toHaveBeenCalled();
  }finally{jest.useRealTimers();}
});

test('W12 an unsupported idle negotiation leaves the original order ready for a manual legacy request',async()=>{
  mockProgressNegotiation.mockRejectedValueOnce(new Error('OWNER_DISCONNECTED'));
  await open();await press('검증한 보드 찾기');await press('시험 보드에서 결제 검토 및 기기 상태 확인');
  expect(screen()).toContain('원승인은 요청하지 않았습니다');
  expect(mockApprove).not.toHaveBeenCalled();
  expect(mockCalls).not.toContain('payment.forward');
  mockApprove.mockRejectedValueOnce(new Error('PAYMENT_REFUSED'));
  await press('검증한 보드 찾기');await press('시험 보드에서 결제 검토');
  expect(mockProgressNegotiation).toHaveBeenCalledTimes(1);
  expect(mockApprove).toHaveBeenCalledTimes(1);
});

test.each(['cancel', 'background', 'expired'])('W10 %s discards a delayed board result and restart never approves or forwards automatically', async exit => {
  let resolveBoard!: (signature: string) => void;
  const delayed = new Promise<string>(resolveReply => {resolveBoard = resolveReply;});
  mockApprove.mockImplementation(async (_payment, _request, onBoard) => {onBoard(); return delayed;});
  await open();
  await press('검증한 보드 찾기');
  await press('시험 보드에서 결제 검토');
  expect(mockApprove).toHaveBeenCalledTimes(1);
  if (exit === 'cancel') await press('승인 종료');
  if (exit === 'background') await Renderer.act(async () => {appState?.('background');});
  if (exit === 'expired') jest.spyOn(Date, 'now').mockReturnValue(quote.expiresAt * 1000);
  await Renderer.act(async () => {resolveBoard('a'.repeat(128) + '1b');});
  await Renderer.act(async () => {tree?.unmount();});
  tree = undefined;
  await open();
  expect(screen()).toContain('승인 대기가 끝났습니다');
  expect(mockApprove).toHaveBeenCalledTimes(1);
  expect(mockCalls).not.toContain('payment.forward');
  const scoped = [...mockJournals.entries()].find(([name]) => /^customer-payment-[a-f0-9]{64}$/.test(name));
  expect(JSON.parse(scoped?.[1] ?? '{}').authorization).toBeUndefined();
});

test('W11 submits once, recovers the original hash after a lost reply and reopens the same confirmed receipt after restart', async () => {
  const verified = preparePresentedPayment(quote, order,
    {...policy, chain: 8283}, now, pending.scope);
  const authorization = bytesToHex(signDigest(hexToBytes(verified.approvalDigest), payerKey), false);
  mockJournals.set('customer-payment-v2', JSON.stringify({...pending, boardStarted: true, authorization}));
  mockForwardLost = true;
  await open();
  expect(screen()).toContain('보드 서명 확인');
  await press('지급 요청 제출');
  expect(mockForwardRequests).toHaveLength(1);
  expect(mockForwardRequests[0].idempotencyKey).toBe(pending.submitKey);
  expect(screen()).toContain('네트워크 응답을 기다립니다');
  expect(screen()).not.toContain('영수증 번호');
  const relayTxHash = '0x' + '4'.repeat(64);
  mockOrder = {...order, state: 'checking', revision: 2, orderKey: undefined, quoteDigest: undefined,
    payment: {stage: 'checking', chainId: 8283, relayTxHash}};
  await Renderer.act(async () => {tree?.unmount();});
  tree = undefined;
  await open();
  expect(mockForwardRequests).toHaveLength(1);
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(relayTxHash);
  expect(screen()).not.toContain('영수증 번호');
  expect(screen()).not.toContain('픽업 번호');
  expect(mockForwardRequests).toHaveLength(1);
  const receiptRef = '0fbc54ef-2827-4ab5-bfcd-3a27611c39ec';
  const finalizedAt = new Date((now + 1) * 1000);
  const pickupReadyAt = new Date((now + 2) * 1000);
  mockOrder = {...mockOrder, state: 'paid', revision: 3, receiptRef, pickupNo: 17,
    payment: {stage: 'finalized', chainId: 8283, relayTxHash, txHash: relayTxHash,
      includedAt: new Date(now * 1000).toISOString(), finalizedAt: finalizedAt.toISOString()}};
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(receiptRef);
  expect(screen()).toContain('픽업 번호');
  expect(screen()).not.toContain('픽업 가능 시각');
  mockOrder = {...mockOrder, payment: {...mockOrder.payment!, pickupStage: 'ready', pickupReadyAt: pickupReadyAt.toISOString()}};
  await Renderer.act(async () => {tree?.unmount();});
  tree = undefined;
  const paymentList = jest.fn();
  await open(undefined, sale, paymentList);
  await press(`매장 주문 ${sale}`);
  expect(screen()).toContain(receiptRef);
  expect(screen()).toContain(relayTxHash);
  expect(screen()).toContain('픽업 준비 완료');
  expect(screen()).toContain(finalizedAt.toLocaleString());
  expect(screen()).toContain(pickupReadyAt.toLocaleString());
  await press('결제 목록');
  expect(paymentList).toHaveBeenCalledTimes(1);
  mockOrder = {...mockOrder, payment: {...mockOrder.payment!, pickupStage: 'collected'}};
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain('픽업 준비 완료 시각');
  expect(screen()).toContain(pickupReadyAt.toLocaleString());
  expect(screen()).not.toContain('픽업 가능 시각');
  expect(screen()).not.toContain('매장 픽업대에서 같은 픽업 번호를 보여주세요.');
  expect(mockForwardRequests).toHaveLength(1);
});

test('T04 keeps the booking, original quote and relay outcome tied together on a lost submit reply', async () => {
  const verified = preparePresentedPayment(quote, order, {...policy, chain: 8283}, now, pending.scope);
  const authorization = bytesToHex(signDigest(hexToBytes(verified.approvalDigest), payerKey), false);
  mockJournals.set('customer-payment-v2', JSON.stringify({...pending, boardStarted: true, authorization}));
  mockForwardLost = true;
  await open(travelPayment);
  expect(screen()).toContain(travelPayment.booking.id);
  expect(screen()).toContain('네트워크 수수료 · 매장 릴레이 부담');
  const calls = [...mockCalls];
  const checkpoint = [...mockJournals];
  await press('주소 전체 보기');
  expect(screen()).toContain('수취 주소 확인');
  expect(screen()).toContain('서명 견적 수령 주소');
  expect(screen()).toContain(policy.beneficiary);
  expect(screen()).toContain('이번 지급');
  expect(screen()).not.toContain('지급 요청 제출');
  await press('돌아가기');
  expect(screen()).toContain(travelPayment.booking.id);
  expect(screen()).toContain(orderKey);
  expect(mockCalls).toEqual(calls);
  expect([...mockJournals]).toEqual(checkpoint);
  expect(mockConnect).not.toHaveBeenCalled();
  expect(mockApprove).not.toHaveBeenCalled();
  await press('지급 요청 제출');
  expect(screen()).toContain('네트워크 응답을 기다립니다');
  expect(mockForwardRequests).toHaveLength(1);
  const hash = '0x' + '4'.repeat(64);
  mockOrder = {...order, state: 'checking', revision: 2, orderKey: undefined, quoteDigest: undefined,
    payment: {stage: 'checking', chainId: 8283, relayTxHash: hash}};
  await Renderer.act(async () => {tree?.unmount();});
  tree = undefined;
  await open(travelPayment);
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(hash);
  expect(screen()).not.toContain('영수증 번호');
  expect(mockCalls).not.toContain('quote.issue');
  expect(mockForwardRequests).toHaveLength(1);
});

test.each(['checking', 'paid'] as const)('T05 reads an earlier deposit checkpoint in state %s before permitting balance review', async state => {
  const balanceSale = 'f0551c76-7592-4850-a414-54464d24daf0';
  mockOrderByID.set(sale, {...order, state, orderKey: undefined, quoteDigest: undefined,
    ...(state === 'paid' ? {receiptRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c39ec', receiptKind: 'booking'} : {})});
  mockOrderByID.set(balanceSale, {
    id: balanceSale, amount: travelPayment.booking.balance, asset: order.asset, expiresAt: order.expiresAt,
    state: 'preview', revision: 0,
  });
  await open({...travelPayment, kind: 'balance'}, balanceSale);
  expect(mockCalls).toEqual(['order.status', 'order.status']);
  expect(screen()).toContain(balanceSale);
  if (state === 'paid') {
    expect(screen()).toContain('이 원주문의 서명 견적 요청');
    expect(screen()).not.toContain('먼저 다른 주문의 보류된 지급을 확인하세요.');
  } else {
    expect(screen()).not.toContain('이 원주문의 서명 견적 요청');
    expect(screen()).toContain('먼저 다른 주문의 보류된 지급을 확인하세요.');
  }
});

test.each([true, false])('W11 pending reopens the original without another approval or submission (branch lookup %s)', async validBranch => {
  const verified = preparePresentedPayment(quote, order, {...policy, chain: 8283}, now, pending.scope);
  const authorization = bytesToHex(signDigest(hexToBytes(verified.approvalDigest), payerKey), false);
  mockJournals.set('customer-payment-v2', JSON.stringify({...pending, route: 'nearby', relayKey: sale,
    authorization, forwardAttempted: true}));
  if (!validBranch) mockShopBranch = customer;
  const relayTxHash = '0x' + '5'.repeat(64);
  mockOrder = {...order, state: 'checking', revision: 2, orderKey: undefined, quoteDigest: undefined,
    payment: {stage: 'checking', chainId: 8283, relayTxHash}};
  const paymentList = jest.fn();
  await open(undefined, sale, paymentList);
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(`${sale} 지급 결과 확인 중`);
  expect(screen()).toContain(relayTxHash);
  expect(screen()).toContain(validBranch ? '지점 여울 커피 성수점' : `지점 ${branch}`);
  if (!validBranch) expect(screen()).not.toContain('여울 커피 성수점');
  expect(screen()).toContain('중복 제출은 차단됩니다.');
  expect(screen()).not.toContain('같은 지급 요청 다시 제출');
  expect(screen()).not.toContain('지급 요청 제출');
  expect(screen()).not.toContain('검증한 보드 찾기');
  expect(screen()).not.toContain('픽업 번호');
  expect(screen()).not.toContain('최종 미지급');
  await press('내 결제');
  expect(paymentList).toHaveBeenCalledTimes(1);
  await Renderer.act(async () => {tree?.unmount();});
  await open(undefined, sale, paymentList);
  await press(`매장 주문 ${sale}`);
  expect(screen()).toContain(relayTxHash);
  expect(screen()).toContain(`${sale} 지급 결과 확인 중`);
  mockStatusOffline = true;
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain('원주문 ');
  expect(screen()).toContain(sale);
  expect(screen()).not.toContain(relayTxHash);
  expect(screen()).not.toContain('지급 요청 제출');
  expect(screen()).not.toContain('검증한 보드 찾기');
  mockStatusOffline = false;
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(relayTxHash);
  const originalAmount = mockOrder.amount;
  mockOrder = {...mockOrder, amount: '99', revision: 3, state: 'paid', pickupNo: 4, receiptRef: branch,
    payment: {stage: 'finalized', chainId: 8283, relayTxHash, txHash: relayTxHash,
      includedAt: new Date(now * 1000).toISOString()}};
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).not.toContain('픽업 번호');
  expect(screen()).not.toContain('확정 거래');
  expect([...mockJournals.values()].some(raw => raw.includes(authorization))).toBe(true);
  mockOrder = {...mockOrder, amount: originalAmount, revision: 3, state: 'paid', pickupNo: 4, receiptRef: branch,
    payment: {stage: 'finalized', chainId: 8283, relayTxHash, txHash: relayTxHash,
      includedAt: new Date(now * 1000).toISOString()}};
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain('확정 거래 ');
  expect(screen()).toContain(relayTxHash);
  expect(screen()).toContain('픽업 번호');
  expect(screen()).not.toContain('중복 제출은 차단됩니다.');
  expect(mockForwardRequests).toHaveLength(0);
  expect(mockNearbyRequests).toHaveLength(0);
  expect(mockApprove).not.toHaveBeenCalled();
});

test('W11 nearby unpaid needs its original owner evidence and survives a fresh payment-list lookup', async () => {
  mockJournals.set('customer-payment-v2', JSON.stringify({...pending, route: 'nearby', relayKey: sale}));
  const checking: CommerceOrder = {...order, state: 'checking', revision: 2, orderKey: undefined,
    quoteDigest: undefined, payment: {stage: 'received', chainId: 8283}};
  const expired: CommerceOrder = {...checking, state: 'closed', revision: 3, payment: undefined};
  mockOrder = checking;
  const paymentList = jest.fn();
  await open(undefined, sale, paymentList);
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain('지급 결과 확인 중');
  mockOrder = expired;
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).not.toContain('지급 0건');
  expect(screen()).not.toContain('검증한 보드 찾기');
  expect([...mockJournals.values()].some(value => value.includes(quote.quoteDigest))).toBe(true);
  const unpaid: CommerceOrder = {...expired, payment: {stage: 'unpaid', chainId: 8283, nearbyUnpaid: true}};
  mockOrder = {...unpaid, amount: '99'};
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).not.toContain('지급 0건');
  mockOrder = unpaid;
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(`${sale} 미지급 확인`);
  expect(screen()).toContain('지급 0건');
  expect(screen()).not.toContain('픽업 번호');
  await press('내 결제');
  expect(paymentList).toHaveBeenCalledTimes(1);
  await Renderer.act(async () => {tree?.unmount();});
  await open(undefined, sale, paymentList);
  await press(`매장 주문 ${sale}`);
  expect(screen()).toContain(`${sale} 미지급 확인`);
  expect(screen()).toContain('지급 0건');
  expect(mockForwardRequests).toHaveLength(0);
  expect(mockApprove).not.toHaveBeenCalled();
});

test('W11 ignores a late branch label after selecting another authenticated order', async () => {
  mockJournals.set('customer-payment-v2', JSON.stringify({...pending, route: 'nearby', relayKey: sale}));
  const other = 'f0551c76-7592-4850-a414-54464d24daff';
  mockOrder = {...order, state: 'checking', revision: 2, orderKey: undefined, quoteDigest: undefined,
      payment: {stage: 'received', chainId: 8283}};
  mockExtraOrder = {...order, id: other};
  mockOrderByID.set(other, mockExtraOrder);
  let finishLookup!: () => void;
  mockShopWait = new Promise(resolve => {finishLookup = resolve;});
  await open();
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(`지점 ${branch}`);
  await press(`매장 주문 ${other}`);
  await Renderer.act(async () => {finishLookup();});
  expect(screen()).not.toContain('여울 커피 성수점');
  expect(screen()).not.toContain('중복 제출은 차단됩니다.');
  expect(mockForwardRequests).toHaveLength(0);
  expect(mockApprove).not.toHaveBeenCalled();
});

test('W11 explicit recovery of an unreceived forward keeps the original request and approval digests', async () => {
  const verified = preparePresentedPayment(quote, order, {...policy, chain: 8283}, now, pending.scope);
  const authorization = bytesToHex(signDigest(hexToBytes(verified.approvalDigest), payerKey), false);
  mockJournals.set('customer-payment-v2', JSON.stringify({...pending, boardStarted: true, authorization}));
  mockForwardLost = true;
  await open();
  await press('지급 요청 제출');
  await press('같은 주문의 지급 결과 확인');
  expect(mockForwardRequests).toHaveLength(1);
  await press('같은 지급 요청 다시 제출');
  expect(mockForwardRequests).toHaveLength(2);
  expect(mockForwardRequests[1]).toMatchObject({
    objectRef: mockForwardRequests[0].objectRef,
    idempotencyKey: mockForwardRequests[0].idempotencyKey,
    body: mockForwardRequests[0].body,
  });
  expect(mockApprove).not.toHaveBeenCalled();
});

test.each([['매장', undefined], ['원예약', travelPayment]] as const)
('%s physical board refusal leaves the original order available for a read-only check', async (_source, bookingPayment) => {
  mockApprove.mockImplementation(async (_payment, _request, onBoard) => {
    onBoard();
    throw new Error('PAYMENT_REFUSED');
  });
  await open(bookingPayment);
  await press('검증한 보드 찾기');
  await press('시험 보드에서 결제 검토');
  expect(screen()).toContain('보드에서 결제를 거절했습니다.');
  await press('같은 주문의 지급 결과 확인');
  expect(mockApprove).toHaveBeenCalledTimes(1);
  expect(mockForwardRequests).toHaveLength(0);
});

test('W10 verified board authorization reaches signed state before any explicit W11 submission', async () => {
  const verified = preparePresentedPayment(quote, order, {...policy, chain: 8283}, now, pending.scope);
  const approval = bytesToHex(signDigest(hexToBytes(verified.approvalDigest), payerKey), false);
  mockApprove.mockImplementation(async (submitted, request, onBoard) => {
    expect(submitted).toMatchObject({digest: quote.quoteDigest, payer, generation});
    expect(bytesToHex(request, false)).toBe(pending.request);
    onBoard();
    return approval;
  });
  await open();
  await press('검증한 보드 찾기');
  await press('시험 보드에서 결제 검토');
  expect(screen()).toContain('보드 서명 확인');
  expect(mockForwardRequests).toHaveLength(0);
  const saved = [...mockJournals.entries()].find(([name]) => /^customer-payment-[a-f0-9]{64}$/.test(name));
  expect(JSON.parse(saved?.[1] ?? '{}').authorization).toBe(approval);
});

test('W10 a signed but unsubmitted quote cannot be forwarded after the original payment details change', async () => {
  const verified = preparePresentedPayment(quote, order, {...policy, chain: 8283}, now, pending.scope);
  const authorization = bytesToHex(signDigest(hexToBytes(verified.approvalDigest), payerKey), false);
  mockJournals.set('customer-payment-v2', JSON.stringify({...pending, authorization, boardStarted: true}));
  await open();
  mockQuote = mockQuoteFor(1n);
  mockOrder = {...order, amount: '1', quoteDigest: mockQuote.quoteDigest, revision: order.revision + 1};
  await press('지급 요청 제출');
  expect(mockForwardRequests).toHaveLength(0);
  expect(mockCalls).not.toContain('quote.issue');
  expect(screen()).toContain('견적이 변경되었습니다.');
  expect(screen()).toContain('새 견적 확인');
  const scoped = [...mockJournals.entries()].find(([name]) => /^customer-payment-[a-f0-9]{64}$/.test(name));
  expect(JSON.parse(scoped?.[1] ?? '{}').authorization).toBeUndefined();
});

test('W09 opens the same paid travel pickup from history before and after merchant acceptance', async () => {
  const receipt = '0fbc54ef-2827-4ab5-bfcd-3a27611c39ec';
  const payment: CommerceOrder['payment'] = {
    stage: 'finalized', chainId: 8283, txHash: '0x' + '4'.repeat(64), includedAt: order.expiresAt,
  };
  mockOrder = {...order, state: 'paid', revision: 2, orderKey: undefined, quoteDigest: undefined,
    receiptRef: receipt, receiptKind: 'pickup-awaiting-acceptance', payment};
  await open();
  await press(`매장 주문 ${sale}`);
  expect(screen()).toContain('지급 확정 · 매장 접수 대기');
  expect(screen()).toContain(receipt);
  expect(screen()).not.toContain('픽업 번호');
  expect(screen()).not.toContain('지급 요청 제출');

  await Renderer.act(async () => {tree?.unmount();});
  await open();
  await press(`매장 주문 ${sale}`);
  expect(screen()).toContain('지급 확정 · 매장 접수 대기');
  expect(screen()).not.toContain('픽업 번호');

  mockOrder = {...mockOrder, receiptKind: undefined, pickupNo: 12};
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(receipt);
  // The pickup number is shown as a ticket whose accessible name carries the number.
  expect(screen()).toContain('"accessibilityLabel":"픽업 번호 12"');
  expect(screen()).not.toContain('매장 접수 대기');
  expect(mockApprove).not.toHaveBeenCalled();
  expect(mockForwardRequests).toHaveLength(0);
  expect(mockCalls).not.toContain('quote.issue');
});

test.each([['QR', 'SW1'], ['W09', 'SW2']])('W10 direct cafe %s reviews the original quote before %s', async (entry, decision) => {
  mockJournals.clear();
  mockQuote = mockNearbyQuote(value(q.amountOffset), sale, true, 1);
  mockOrder = {...order, quoteDigest: mockQuote.quoteDigest,
    cafeReview: {branchLabel: '여울 커피 성수점', payerBound: true}};
  const verified = preparePresentedPayment(mockQuote, mockOrder, {...policy, chain: 8283}, now, pending.scope);
  const signature = bytesToHex(signDigest(hexToBytes(verified.approvalDigest), payerKey), false);
  let finish: (() => void) | undefined;
  mockApprove.mockImplementation(async (payment, _request, onBoard) => {
    expect(payment).toMatchObject({bytes: verified.bytes, digest: verified.digest, payer, generation});
    onBoard();
    await new Promise<void>(resolve => {finish = resolve;});
    if (decision === 'SW2') throw new Error('PAYMENT_REFUSED');
    return signature;
  });

  await open();
  if (entry === 'QR') {
    const input = tree?.root.findAll(node => node.props.accessibilityLabel === '매장 주문 QR 내용' && node.props.onChangeText)[0];
    expect(input).toBeDefined();
    await Renderer.act(async () => input?.props.onChangeText(encodeOrderLocator({branchRef: branch, orderRef: sale, expiresAt: now + 30}, now)));
    await press('QR 주문 연결');
  } else {
    await press(`매장 주문 ${sale}`);
  }
  await press('이 주문의 결제 내용 검토');
  expect(screen()).toContain('여울 커피 성수점');
  expect(screen()).toContain('보드의 원견적 검증 대기');
  expect(screen()).not.toContain('가스 비용 · 매장 부담');
  await press('검증한 보드 찾기');
  await press('시험 보드에서 결제 검토');
  expect(screen()).toContain('보드가 원견적을 검증했습니다');
  expect(screen()).toContain('가스 비용 · 매장 부담');
  expect(screen()).toContain('개인 지급 계정');
  expect(screen()).not.toContain('대여 지갑 예치금');
  expect(screen()).toContain(payer);
  expect(mockForwardRequests).toHaveLength(0);

  expect(finish).toBeDefined();
  await Renderer.act(async () => finish?.());
  if (decision === 'SW1') {
    expect(screen()).toContain('보드 서명 확인');
    await press('지급 요청 제출');
    expect(mockForwardRequests).toHaveLength(1);
    expect(mockForwardRequests[0]).toMatchObject({objectRef: sale, body: {
      quoteDigest: verified.digest, approvalDigest: verified.approvalDigest,
    }});
    mockOrder = {...mockOrder, state: 'checking', revision: 2, payment: {stage: 'received', chainId: 8283}};
  } else {
    expect(screen()).toContain('보드에서 결제를 거절했습니다');
    expect(mockForwardRequests).toHaveLength(0);
  }
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(sale);
  expect(mockApprove).toHaveBeenCalledTimes(1);
  if (decision === 'SW1') expect(screen()).toContain('매장이 지급 요청을 받았습니다.');
});

test('W10 rental escrow proof keeps the owner board path closed', async () => {
  mockJournals.clear();
  mockQuote = mockNearbyQuote(value(q.amountOffset), sale, true, 2);
  const escrow = {expiresAt: now + 5, signature: ''};
  mockQuote.presentation!.rentalEscrow = escrow;
  escrow.signature = bytesToHex(ed25519.sign(rentalEscrowMessage(mockQuote, generation), presentationSeed), false);
  mockOrder = {...order, quoteDigest: mockQuote.quoteDigest,
    cafeReview: {branchLabel: '여울 커피 성수점', payerBound: true}};
  await open();
  await press(`매장 주문 ${sale}`);
  await press('이 주문의 결제 내용 검토');
  expect(screen()).toContain('이 원견적을 확인한 대여 보드의 증거가 필요합니다');
  expect(screen()).not.toContain('검증한 보드 찾기');
  expect(screen()).not.toContain('개인 지급 계정');
  expect(screen()).not.toContain('보드가 원견적을 검증했습니다');
  expect(mockApprove).not.toHaveBeenCalled();
  expect(mockForwardRequests).toHaveLength(0);
});

test.each([['QR', 'SW1'], ['W09', 'SW2']])('W10 rental %s verifies %s on the original sale', async (entry, decision) => {
  mockJournals.clear();
  mockNearbyEnabled = true;
  mockQuote = mockNearbyQuote(value(q.amountOffset), sale, true, 2);
  const escrow = {expiresAt: now + 5, signature: ''};
  mockQuote.presentation!.rentalEscrow = escrow;
  escrow.signature = bytesToHex(ed25519.sign(rentalEscrowMessage(mockQuote, generation), presentationSeed), false);
  mockOrder = {...order, quoteDigest: mockQuote.quoteDigest,
    cafeReview: {branchLabel: '여울 커피 성수점', payerBound: true}};
  let reviewComplete: (() => void) | undefined;
  let decide: ((wire: string) => void) | undefined;
  let receipt = '';
  let signature = '';
  const readBoard = jest.fn(async (payment: ReturnType<typeof preparePresentedPayment>, authority: RentalReviewAuthority,
    _signal: AbortSignal, decided: (wire: string) => void, begin: () => void) => {
    begin();
    decide = decided;
    await new Promise<void>(resolve => {reviewComplete = resolve;});
    receipt = mockReviewReceipt(authority);
    signature = bytesToHex(signDigest(hexToBytes(payment.approvalDigest), payerKey), false);
    return receipt;
  });
  const terminalAPI = {profile: {branchRef: branch, subjectRef: sale}, status: async (ref: string) => {
    if (ref !== sale) throw new Error('FORBIDDEN');
    return mockOrder;
  }} as CommerceClient;
  const terminal = new NearbyRelay(terminalAPI, {} as MerchantInbox, async () => new Uint8Array(16),
    () => undefined, () => {}, undefined, {policy: {...policy, chain: 8283}, read: readBoard});
  terminal.activate(mockPhonePeer.link);
  mockRentalRelay = terminal;
  await open();
  await press('근처 단말 찾기');
  await press('근거리 단말 후보 1 인증');
  await press('이 매장 메뉴 열기');
  if (entry === 'QR') {
    const input = tree?.root.findAll(node => node.props.accessibilityLabel === '매장 주문 QR 내용' && node.props.onChangeText)[0];
    await Renderer.act(async () => input?.props.onChangeText(encodeOrderLocator({branchRef: branch, orderRef: sale, expiresAt: now + 30}, now)));
    await press('QR 주문 연결');
  } else {
    await press(`매장 주문 ${sale}`);
  }
  await press('이 주문의 결제 내용 검토');
  await press('대여 보드 원견적 검토 확인');
  expect(screen()).not.toContain('보드가 원견적을 검증했습니다');
  expect(screen()).not.toContain('대여 지갑 예치금');
  expect(mockApprove).not.toHaveBeenCalled();
  expect({route: mockNearbyRequests.map(request => request.op), calls: readBoard.mock.calls.length}).toEqual({
    route: expect.arrayContaining(['rental-review-source', 'rental-review']), calls: 1,
  });
  expect(reviewComplete).toBeDefined();
  await Renderer.act(async () => reviewComplete?.());
  expect(screen()).toContain('보드가 원견적을 검증했습니다');
  expect(screen()).toContain('대여 지갑 예치금');
  expect(screen()).toContain('가스 비용 · 매장 부담');
  expect(screen()).toContain('보드 SW1로 승인하거나 SW2로 거절하세요');
  expect(mockForwardRequests).toHaveLength(0);
  const originalSources = mockNearbyRequests.filter(request => request.op === 'rental-review-source');
  expect(originalSources).toHaveLength(1);
  expect(originalSources[0].order).toBe(sale);
  const reviewRequests = mockNearbyRequests.filter(request => request.op === 'rental-review');
  expect(reviewRequests).toHaveLength(1);
  expect(reviewRequests[0]).toMatchObject({order: sale, quoteDigest: mockQuote.quoteDigest, key: originalSources[0].key});
  const otherCustomer = {...mockPhonePeer, link: 'another-customer', principal: sale};
  terminal.activate(otherCustomer.link);
  expect((await terminal.handle(originalSources[0], otherCustomer)).status).not.toBe('ok');
  expect((await terminal.handle(reviewRequests[0], otherCustomer)).status).toBe('denied');
  expect((await terminal.handle(originalSources[0], {...mockPhonePeer, branch: customer})).status).toBe('denied');
  expect(readBoard).toHaveBeenCalledTimes(1);
  expect([...mockJournals.values()].join(' ')).not.toContain(reviewRequests[0].challenge);
  await press('대여 보드 결정 확인');
  expect(screen()).toContain('SW1 또는 SW2를 누른 뒤');
  expect(mockForwardRequests).toHaveLength(0);
  const originalDecision = mockNearbyRequests.find(request => request.op === 'rental-decision');
  expect(originalDecision).toMatchObject({key: originalSources[0].key, order: sale, quoteDigest: mockQuote.quoteDigest});
  if (!originalDecision || originalDecision.op !== 'rental-decision') throw new Error('MISSING_DECISION');
  expect((await terminal.handle(originalDecision, otherCustomer)).status).toBe('denied');
  expect((await terminal.handle(originalDecision, {...mockPhonePeer, branch: customer})).status).toBe('denied');
  const decisionWire = (reviewHash: Uint8Array) => {
    const body = concat(utf8Encode('W10D'), reviewHash, new Uint8Array([decision === 'SW1' ? 1 : 2]),
      decision === 'SW1' ? hexToBytes(signature) : new Uint8Array(65));
    const digest = sha256(concat(utf8Encode('NU54 W10 rental payment decision v1\0'), body));
    return bytesToHex(concat(body, p256.sign(digest, mockReviewKey, {prehash:false,lowS:true}).toCompactRawBytes()), false);
  };
  mockRentalDecisionOverride = receipt;
  await press('대여 보드 결정 확인');
  expect(screen()).toContain('같은 원견적의 대여 보드 결정을 확인할 수 없습니다.');
  expect(screen()).not.toContain('지급 요청 제출');
  mockRentalDecisionOverride = decisionWire(new Uint8Array(32).fill(8));
  await press('대여 보드 결정 확인');
  expect(screen()).toContain('같은 원견적의 대여 보드 결정을 확인할 수 없습니다.');
  expect(mockForwardRequests).toHaveLength(0);
  mockRentalDecisionOverride = undefined;
  await Renderer.act(async () => decide?.(decisionWire(new Uint8Array(32).fill(8))));
  await press('대여 보드 결정 확인');
  expect(screen()).toContain('SW1 또는 SW2를 누른 뒤');
  await Renderer.act(async () => decide?.(decisionWire(sha256(hexToBytes(receipt)))));
  await press('대여 보드 결정 확인');
  expect(mockForwardRequests).toHaveLength(0);
  expect([...mockJournals.values()].join(' ')).not.toContain(receipt);
  if (decision === 'SW2') {
    expect(screen()).toContain('보드에서 이 원견적의 지급을 거절했습니다.');
    expect(screen()).not.toContain('지급 요청 제출');
    expect(mockApprove).not.toHaveBeenCalled();
    return;
  }
  expect(screen()).toContain('보드에서 이 원견적의 지급을 승인했습니다.');
  mockForwardLost = true;
  await press('지급 요청 제출');
  expect(mockForwardRequests).toHaveLength(1);
  expect(mockForwardRequests[0]).toMatchObject({objectRef: sale, idempotencyKey: originalSources[0].key,
    body:{quoteDigest:mockQuote.quoteDigest},approvalSignature:signature});
  expect(mockNearbyRequests.some(request => request.op === 'forward')).toBe(false);
  const authorized = {...mockOrder};
  delete authorized.quoteDigest;
  delete authorized.orderKey;
  const originalHash = '0x' + 'f'.repeat(64);
  mockOrder = {...authorized, state: 'checking', revision: 2, payment: {stage: 'submitted', chainId: 8283, relayTxHash: originalHash}};
  await Renderer.act(async () => tree?.unmount());
  await open();
  await press(`매장 주문 ${sale}`);
  await press('같은 주문의 지급 결과 확인');
  expect(screen()).toContain(originalHash);
  expect(screen()).toContain('원거래가 네트워크에 제출되었습니다.');
  expect(mockForwardRequests).toHaveLength(1);
  expect(screen()).not.toContain('보드가 원견적을 검증했습니다');
  mockOrder = {...authorized, state: 'paid', revision: 3, receiptRef: branch, pickupNo: 4,
    payment: {stage: 'finalized', chainId: 8283, txHash: originalHash, includedAt: new Date(now * 1000).toISOString()}};
  await Renderer.act(async () => tree?.unmount());
  await open();
  await press(`매장 주문 ${sale}`);
  expect(screen()).toContain(branch);
  expect(screen()).toContain('영수증 번호');
  expect(screen()).toContain(originalHash);
  expect(mockNearbyRequests.filter(request => request.op === 'rental-review-source' || request.op === 'rental-review')).toHaveLength(2);
  expect(mockApprove).not.toHaveBeenCalled();
  expect(mockForwardRequests).toHaveLength(1);
});
