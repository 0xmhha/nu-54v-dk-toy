import Renderer from 'react-test-renderer';
import {TravelPickupDetail} from '../src/travel/TravelPickupDetail.tsx';

const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
const sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002';
const payment = '0fbc54ef-2827-4ab5-bfcd-3a27611c3003';
const refund = '0fbc54ef-2827-4ab5-bfcd-3a27611c3004';
const mockExchange = jest.fn();
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true, default: {exchange: (...args: unknown[]) => mockExchange(...args)},
}));

test('reads the same pickup origin through acceptance and separate refund finality', async () => {
  const original = {saleRef:sale, startsAt:'2027-01-02T09:00:00Z',timezone:'Asia/Seoul',
    amount:'3500000',asset:'0x'+'a'.repeat(40),receiptRef:payment};
  mockExchange
    .mockResolvedValueOnce(JSON.stringify({pickups:[{...original,state:'awaiting-acceptance'}]}))
    .mockResolvedValueOnce(JSON.stringify({pickups:[{...original,state:'unfulfillable',pickupNo:8,refund:{state:'pending',amount:'3500000'}}]}))
    .mockResolvedValueOnce(JSON.stringify({pickups:[{...original,state:'unfulfillable',pickupNo:8,
      refund:{state:'confirmed',amount:'3500000',receiptRef:refund}}]}));
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<TravelPickupDetail branch={branch} sale={sale} orderRevision={2} />);
  });
  const contents = () => JSON.stringify(tree.toJSON());
  expect(contents()).toContain('지급 확정 · 매장 접수 대기');
  expect(contents()).not.toContain('주문번호');
  const refresh = () => Renderer.act(async () => {
    tree.root.findAll(node => typeof node.props.onPress === 'function' &&
      node.props.accessibilityLabel === '이 주문의 픽업과 환불 상태 다시 조회')[0].props.onPress();
  });
  await refresh();
  expect(contents()).toContain('환불 지급 확인 중');
  expect(contents()).not.toContain(refund);
  await refresh();
  expect(contents()).toContain('환불 지급 확정');
  expect(contents()).toContain(refund);
  expect(mockExchange.mock.calls).toEqual(Array(3).fill(['customer', JSON.stringify({travel:{operation:'pickup-owned',branch,sale}})]));
  await Renderer.act(async () => tree.unmount());
});
