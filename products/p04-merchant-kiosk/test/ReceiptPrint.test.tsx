import Renderer from 'react-test-renderer';
import { ReceiptPrint } from '../src/kiosk/ReceiptPrint.tsx';

const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
const sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002';
const receipt = '0fbc54ef-2827-4ab5-bfcd-3a27611c3003';
const attempt = '0fbc54ef-2827-4ab5-bfcd-3a27611c3004';
const copy = '0fbc54ef-2827-4ab5-bfcd-3a27611c3005';
const mockJournal = new Map<string, string>();
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    randomBytes: async () => 'AAAAAAAAAAAAAAAAAAAAAA==',
    loadJournal: async (name: string) => mockJournal.get(name) ?? '',
    saveJournal: async (name: string, value: string) => {
      mockJournal.set(name, value);
    },
  },
}));

function button(tree: Renderer.ReactTestRenderer, label: string) {
  return tree.root.findAll(
    node =>
      node.props.accessibilityRole === 'button' &&
      node.props.accessibilityLabel === label,
  )[0];
}

test('an unknown print never resends automatically and a lost explicit copy resumes only under its original staff identity', async () => {
  const calls: string[] = [];
  let committed = false;
  const transport = jest.fn(
    async (operation: string, reference: string, wire: string) => {
      if (operation === 'queue')
        return JSON.stringify([
          { saleRef: sale, receiptRef: receipt, pickupNo: 17 },
        ]);
      if (operation === 'attempts') {
        expect(reference).toBe(receipt);
        return JSON.stringify(
          committed
            ? [
                { attemptId: copy, state: 'pending', previousId: attempt },
                { attemptId: attempt, state: 'superseded' },
              ]
            : [{ attemptId: attempt, state: 'unknown' }],
        );
      }
      expect(operation).toBe('decision');
      calls.push(wire);
      if (!committed) {
        committed = true;
        throw new Error('lost committed response');
      }
      return JSON.stringify({ attemptId: copy });
    },
  );
  async function open(journal: string) {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => {
      tree = Renderer.create(
        <ReceiptPrint
          branch={branch}
          journal={journal}
          sale={sale}
          transport={transport}
          close={() => {}}
        />,
      );
    });
    return tree;
  }
  const initial = await open('print-v1-' + '1'.repeat(64));
  expect(calls).toHaveLength(0);
  await Renderer.act(async () =>
    button(initial, '프린터에 출력되지 않음 확인').props.onPress(),
  );
  expect(calls).toHaveLength(0);
  await Renderer.act(async () =>
    button(initial, '같은 영수증 다시 출력 요청').props.onPress(),
  );
  expect(calls).toHaveLength(1);
  await Renderer.act(async () => initial.unmount());

  const substitute = await open('print-v1-' + '2'.repeat(64));
  expect(button(substitute, '같은 출력 요청 다시 확인')).toBeUndefined();
  expect(calls).toHaveLength(1);
  await Renderer.act(async () => substitute.unmount());

  const restored = await open('print-v1-' + '1'.repeat(64));
  expect(calls).toHaveLength(1);
  await Renderer.act(async () =>
    button(restored, '같은 출력 요청 다시 확인').props.onPress(),
  );
  expect(calls).toEqual([calls[0], calls[0]]);
  expect(JSON.parse(calls[0])).toMatchObject({
    branchId: branch,
    attemptId: attempt,
    action: 'reprint',
  });
  await Renderer.act(async () => restored.unmount());
});

test('a previous shift cannot display a receipt when its late read resolves after exit', async () => {
  let release!: (value: string) => void;
  const first = jest.fn(
    async () =>
      new Promise<string>(resolve => {
        release = resolve;
      }),
  );
  const second = jest.fn(async () => '[]');
  let prior!: Renderer.ReactTestRenderer;
  let current!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    prior = Renderer.create(
      <ReceiptPrint
        branch={branch}
        journal={'print-v1-' + '3'.repeat(64)}
        transport={first}
        close={() => {}}
      />,
    );
  });
  expect(first).toHaveBeenCalledWith('queue', '', '');
  await Renderer.act(async () => {
    prior.unmount();
    current = Renderer.create(
      <ReceiptPrint
        branch={branch}
        journal={'print-v1-' + '4'.repeat(64)}
        transport={second}
        close={() => {}}
      />,
    );
  });
  await Renderer.act(async () =>
    release(
      JSON.stringify([{ saleRef: sale, receiptRef: receipt, pickupNo: 17 }]),
    ),
  );
  expect(button(current, '픽업 17 영수증 열기')).toBeUndefined();
  expect(second).toHaveBeenCalledTimes(1);
  await Renderer.act(async () => current.unmount());
});
