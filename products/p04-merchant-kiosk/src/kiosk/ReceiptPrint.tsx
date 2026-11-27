import { useEffect, useRef, useState } from 'react';
import { AppState, Pressable, StyleSheet, Text, View } from 'react-native';
import { commerceRequestId, fromBase64 } from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';

const ref = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/;
const states = {
  pending: '출력 대기',
  unknown: '출력 결과 미확인',
  printed: '출력 확인',
  superseded: '새 시도로 인계',
} as const;
type Receipt = { saleRef: string; receiptRef: string; pickupNo: number };
type Attempt = {
  attemptId: string;
  previousId?: string;
  state: keyof typeof states;
};
type Action = 'confirm_printed' | 'reprint';
type Intent = {
  receipt: Receipt;
  attempt: string;
  key: string;
  action: Action;
};
type Transport = (
  operation: 'queue' | 'attempts' | 'decision',
  reference: string,
  body: string,
) => Promise<string>;
const active = new Set<string>();

function validReceipt(row: Receipt | null): row is Receipt {
  return (
    !!row &&
    ref.test(row.saleRef) &&
    ref.test(row.receiptRef) &&
    Number.isSafeInteger(row.pickupNo) &&
    row.pickupNo > 0
  );
}
function Button({
  title,
  run,
  disabled,
}: {
  title: string;
  run: () => void;
  disabled?: boolean;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={title}
      disabled={disabled}
      onPress={run}
    >
      <Text>{title}</Text>
    </Pressable>
  );
}

export function ReceiptPrint({
  branch,
  journal,
  sale,
  transport,
  close,
}: {
  branch: string;
  journal: string;
  sale?: string;
  transport: Transport;
  close: () => void;
}) {
  const [queue, setQueue] = useState<Receipt[]>();
  const [selected, setSelected] = useState<Receipt>();
  const [attempts, setAttempts] = useState<Attempt[]>();
  const [review, setReview] = useState<Action>();
  const [pending, setPending] = useState<Intent>();
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const view = useRef({ generation: 0 }).current;

  async function run(job: (current: () => boolean) => Promise<void>) {
    if (active.has(journal)) return;
    const epoch = view.generation;
    const current = () => epoch === view.generation;
    active.add(journal);
    setBusy(true);
    setMessage('');
    try {
      await job(current);
    } catch {
      if (current()) {
        setQueue(undefined);
        setSelected(undefined);
        setAttempts(undefined);
        setReview(undefined);
        setMessage(
          '출력 상태를 확인할 수 없습니다. 기존 영수증과 요청을 다시 확인하세요.',
        );
      }
    } finally {
      active.delete(journal);
      if (current()) setBusy(false);
    }
  }

  async function open(row: Receipt, current: () => boolean) {
    const result = JSON.parse(
      await transport('attempts', row.receiptRef, ''),
    ) as Attempt[];
    if (
      !Array.isArray(result) ||
      result.length > 50 ||
      result.some(
        item =>
          !ref.test(item.attemptId) ||
          !Object.prototype.hasOwnProperty.call(states, item.state) ||
          (item.previousId !== undefined && !ref.test(item.previousId)),
      )
    )
      throw new Error('PRINT_ATTEMPTS');
    if (!current()) return;
    setSelected(row);
    setAttempts(result);
    setReview(undefined);
  }

  async function load(current: () => boolean) {
    if (!Http) throw new Error('PRINT_JOURNAL');
    const saved = JSON.parse(
      (await Http.loadJournal(journal)) || 'null',
    ) as Intent | null;
    if (
      saved &&
      (!validReceipt(saved.receipt) ||
        !ref.test(saved.attempt) ||
        !ref.test(saved.key) ||
        !['confirm_printed', 'reprint'].includes(saved.action))
    )
      throw new Error('PRINT_JOURNAL');
    const list = JSON.parse(
      await transport('queue', sale ?? '', ''),
    ) as Receipt[];
    if (
      !Array.isArray(list) ||
      list.length > 50 ||
      list.some(
        row =>
          !validReceipt(row) || (sale !== undefined && row.saleRef !== sale),
      )
    )
      throw new Error('PRINT_QUEUE');
    if (!current()) return;
    setQueue(list);
    setPending(saved ?? undefined);
    const row = saved?.receipt ?? (sale ? list[0] : undefined);
    if (row) await open(row, current);
  }

  useEffect(() => {
    run(load);
    const listener = AppState.addEventListener('change', state => {
      if (state === 'active') return;
      view.generation++;
      setQueue(undefined);
      setSelected(undefined);
      setAttempts(undefined);
      setPending(undefined);
      setReview(undefined);
      setMessage('');
      setBusy(false);
      close();
    });
    return () => {
      view.generation++;
      listener.remove();
    };
    // The parent remounts with a new journal when the staff identity changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [journal, sale]);

  async function decide(current: () => boolean) {
    if (
      !Http ||
      (!pending &&
        (!selected ||
          !review ||
          !(
            attempts?.[0]?.state === 'unknown' ||
            (review === 'reprint' && attempts?.[0]?.state === 'printed')
          )))
    )
      return;
    const intent = pending ?? {
      receipt: selected!,
      attempt: attempts![0].attemptId,
      key: commerceRequestId(fromBase64(await Http.randomBytes(16))),
      action: review!,
    };
    if (!current()) return;
    await Http.saveJournal(journal, JSON.stringify(intent));
    if (!current()) return;
    setPending(intent);
    setReview(undefined);
    const wire = JSON.stringify({
      branchId: branch,
      attemptId: intent.attempt,
      idempotencyKey: intent.key,
      action: intent.action,
    });
    const result = JSON.parse(
      await transport('decision', intent.attempt, wire),
    ) as { attemptId: string };
    if (
      !ref.test(result.attemptId) ||
      (intent.action === 'confirm_printed'
        ? result.attemptId !== intent.attempt
        : result.attemptId === intent.attempt)
    )
      throw new Error('PRINT_DECISION');
    if (!current()) return;
    await open(intent.receipt, current);
    if (!current()) return;
    await Http.saveJournal(journal, 'null');
    if (current()) setPending(undefined);
  }

  const unknown = attempts?.[0]?.state === 'unknown';
  return (
    <View style={styles.panel}>
      <Text accessibilityRole="header">직원 영수증과 출력</Text>
      <Button
        title="출력 목록 다시 조회"
        disabled={busy}
        run={() => {
          run(load);
        }}
      />
      {queue?.length === 0 && <Text>확정 인계된 영수증이 없습니다.</Text>}
      {queue?.map(row => (
        <Button
          key={row.receiptRef}
          title={`픽업 ${row.pickupNo} 영수증 열기`}
          disabled={busy || !!pending}
          run={() => {
            run(current => open(row, current));
          }}
        />
      ))}
      {selected && (
        <>
          <Text selectable>영수증 {selected.receiptRef}</Text>
          <Text selectable>원주문 {selected.saleRef}</Text>
          <Text>픽업 번호 {selected.pickupNo}</Text>
          {attempts?.map(row => (
            <Text key={row.attemptId} selectable>
              {states[row.state]} · {row.attemptId}
            </Text>
          ))}
          {unknown && (
            <Text>
              프린터와 실제 종이를 확인하세요. 미확인 출력은 자동으로 다시
              보내지 않습니다.
            </Text>
          )}
          {unknown && !pending && !review && (
            <>
              <Button
                title="종이에 이미 출력됨 확인"
                disabled={busy}
                run={() => setReview('confirm_printed')}
              />
              <Button
                title="프린터에 출력되지 않음 확인"
                disabled={busy}
                run={() => setReview('reprint')}
              />
            </>
          )}
          {attempts?.[0]?.state === 'printed' && !pending && !review && (
            <Button
              title="추가 사본 검토"
              disabled={busy}
              run={() => setReview('reprint')}
            />
          )}
          {review && (
            <>
              <Text>
                {review === 'reprint'
                  ? '같은 영수증의 새 출력 시도 한 건을 요청합니다.'
                  : '기존 출력 시도를 출력 완료로 기록합니다.'}
              </Text>
              <Button
                title={
                  review === 'reprint'
                    ? '같은 영수증 다시 출력 요청'
                    : '기존 출력 확인 기록'
                }
                disabled={busy}
                run={() => {
                  run(decide);
                }}
              />
              <Button
                title="출력 결정 취소"
                disabled={busy}
                run={() => setReview(undefined)}
              />
            </>
          )}
        </>
      )}
      {pending && (
        <Button
          title="같은 출력 요청 다시 확인"
          disabled={busy}
          run={() => {
            run(decide);
          }}
        />
      )}
      {!!message && <Text accessibilityRole="alert">{message}</Text>}
      <Button title="직원 업무로 돌아가기" run={close} />
    </View>
  );
}

const styles = StyleSheet.create({ panel: { gap: 10 } });
