import { useEffect, useRef, useState } from 'react';
import {
  AppState,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import {
  commerceRequestId,
  commerceCartAmount,
  formatPickupTime,
  type CommercePaymentProfile,
  fromBase64,
  readPickupKitchen,
  type PickupKitchenOrder,
  type PickupKitchenState,
  type PickupTarget,
} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';

import { loadPickupIntent, type PickupChange } from './pickupJournal.ts';

const activeJournals = new Set<string>();
const nextTarget: Partial<Record<PickupKitchenState, PickupTarget>> = {
  accepted: 'preparing',
  preparing: 'ready',
  ready: 'collected',
};
const stateLabel: Record<PickupKitchenOrder['state'], string> = {
  checking: '원주문 지급과 이행 확인 중',
  accepted: '접수됨',
  preparing: '준비 중',
  ready: '준비 완료',
  collected: '수령 완료',
  unfulfillable: '이행 불가',
};
const actionLabel: Record<PickupTarget, string> = {
  preparing: '준비 시작',
  ready: '준비 완료 확인',
  collected: '수령 완료 확인',
  unfulfillable: '이행 불가 확정',
};
function Action({
  title,
  onPress,
  disabled,
}: {
  title: string;
  onPress: () => void;
  disabled?: boolean;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={title}
      onPress={onPress}
      disabled={disabled}
      style={[styles.button, disabled && styles.disabled]}
    >
      <Text>{title}</Text>
    </Pressable>
  );
}

export function PickupKitchen({
  journal,
  transport,
  branch,
  actor,
  openReceipt,
  paymentProfile,
  allowCafe = false,
  close,
}: {
  journal: string;
  transport: (envelope: string) => Promise<string>;
  branch: string;
  actor: string;
  openReceipt?: (sale: string) => void;
  paymentProfile?: CommercePaymentProfile;
  allowCafe?: boolean;
  close: () => void;
}) {
  const [list, setList] = useState<PickupKitchenOrder[]>();
  const [selected, setSelected] = useState<PickupKitchenOrder>();
  const [observedAt, setObservedAt] = useState<number>();
  const [pending, setPending] = useState<PickupChange>();
  const [busy, setBusy] = useState(false);
  const [reject, setReject] = useState(false);
  const [message, setMessage] = useState('');
  const [lookup, setLookup] = useState('');
  const view = useRef({ generation: 0 }).current;

  useEffect(() => {
    const background = AppState.addEventListener('change', state => {
      if (state === 'active') return;
      view.generation++;
      setPending(undefined);
      setBusy(false);
      setMessage('');
      setList(undefined);
      setSelected(undefined);
      setObservedAt(undefined);
      setLookup('');
      setReject(false);
    });
    return () => {
      view.generation++;
      background.remove();
    };
  }, [view]);

  function exchange(operation: string, fields: object = {}) {
    return transport(
      JSON.stringify({
        travel: {
          branch,
          operation,
          ...fields,
          ...(allowCafe &&
          (operation === 'pickup-queue' || operation === 'pickup-detail')
            ? { includeCafe: true }
            : {}),
        },
      }),
    );
  }

  async function run(job: (current: () => boolean) => Promise<void>) {
    if (activeJournals.has(journal)) {
      setMessage('같은 직원의 픽업 요청을 확인하고 있습니다.');
      return;
    }
    activeJournals.add(journal);
    const generation = view.generation;
    const current = () => generation === view.generation;
    setBusy(true);
    setMessage('');
    try {
      await job(current);
    } catch {
      if (!current()) return;
      setList(undefined);
      setSelected(undefined);
      setObservedAt(undefined);
      setReject(false);
      setMessage(
        '조리 권한 또는 접수 결과를 확인할 수 없습니다. 같은 요청을 다시 확인하세요.',
      );
    } finally {
      activeJournals.delete(journal);
      if (current()) setBusy(false);
    }
  }

  async function load(current: () => boolean) {
    // Authorize the queue before attempting any v1 migration.
    const available = readPickupKitchen(await exchange('pickup-queue'));
    if (!current()) return;
    const intent = await loadPickupIntent(journal, branch, actor);
    if (!current()) return;
    setPending(intent);
    setList(available);
  }

  async function open(sale: string, current: () => boolean) {
    const [item] = readPickupKitchen(await exchange('pickup-detail', { sale }));
    if (!item || item.saleRef !== sale) throw new Error('Pickup unavailable');
    if (!current()) return;
    setSelected(item);
    setObservedAt(Date.now());
    setReject(false);
  }

  async function submit(current: () => boolean, target?: PickupTarget) {
    let intent = await loadPickupIntent(journal, branch, actor);
    if (!current()) return;
    if (!intent) {
      if (!selected || !target) return;
      intent = {
        branch,
        actor,
        sale: selected.saleRef,
        requestKey: commerceRequestId(fromBase64(await Http!.randomBytes(16))),
        expectedRevision: selected.revision,
        target,
      };
      if (!current()) return;
      await Http!.saveJournal(journal, JSON.stringify(intent));
    }
    if (!current()) return;
    setPending(intent);
    const result = JSON.parse(
      await exchange('pickup-change', {
        sale: intent.sale,
        requestKey: intent.requestKey,
        expectedRevision: intent.expectedRevision,
        target: intent.target,
      }),
    );
    const conflict = result?.error === 'request_conflict';
    if (!conflict && result?.revision !== intent.expectedRevision + 1)
      throw new Error('Pickup not confirmed');
    if (!current()) return;
    await open(intent.sale, current);
    if (!current()) return;
    // An explicit tombstone prevents a completed v1 copy from being adopted again.
    await Http!.saveJournal(journal, 'null');
    if (!current()) return;
    setPending(undefined);
    if (conflict)
      setMessage('다른 단말에서 상태를 갱신했습니다. 현재 주문을 확인하세요.');
    const available = readPickupKitchen(await exchange('pickup-queue'));
    if (current()) setList(available);
  }

  const next =
    selected && selected.state !== 'checking'
      ? nextTarget[selected.state]
      : undefined;
  return (
    <View style={styles.panel}>
      <Text accessibilityRole="header" style={styles.heading}>
        픽업 접수와 준비
      </Text>
      <Action
        title="지점 픽업 접수 조회"
        disabled={busy}
        onPress={() => {
          run(load);
        }}
      />
      {allowCafe && (
        <>
          <TextInput
            accessibilityLabel="원주문 식별값"
            value={lookup}
            onChangeText={setLookup}
            maxLength={36}
            autoCapitalize="none"
            editable={!busy && !pending}
            style={styles.input}
          />
          <Action
            title="같은 원주문 픽업 조회"
            disabled={
              busy ||
              !!pending ||
              !/^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/.test(
                lookup.trim(),
              )
            }
            onPress={() => {
              run(current => open(lookup.trim(), current));
            }}
          />
        </>
      )}
      {list?.length === 0 && <Text>접수된 픽업이 없습니다.</Text>}
      {list?.map(item => (
        <Action
          key={item.saleRef}
          disabled={busy || !!pending}
          title={
            item.state === 'checking'
              ? `확인 중 주문 ${item.saleRef}`
              : `픽업 ${item.pickupNo} · ${formatPickupTime(
                  item.startsAt,
                  item.timezone,
                )} · ${stateLabel[item.state]}`
          }
          onPress={() => {
            run(current => open(item.saleRef, current));
          }}
        />
      ))}
      {selected && (
        <View style={styles.panel}>
          <Text>
            {selected.state === 'checking'
              ? stateLabel.checking
              : `픽업 번호 ${selected.pickupNo} · ${
                  stateLabel[selected.state]
                }`}
          </Text>
          <Text selectable>원주문 {selected.saleRef}</Text>
          {selected.eventRef && (
            <Text selectable>이행 ID {selected.eventRef}</Text>
          )}
          {selected.settlement && (
            <>
              <Text>지급 금액 {commerceCartAmount(selected.settlement.amount, selected.settlement.asset, paymentProfile)}</Text>
              <Text>지급 확정 시각 {formatPickupTime(selected.settlement.finalizedAt, selected.timezone)}</Text>
              <Text>픽업 가능 시각 {formatPickupTime(selected.settlement.pickupReadyAt, selected.timezone)}</Text>
            </>
          )}
          {selected.state === 'checking' && (
            <>
              {observedAt !== undefined && (
                <Text>
                  마지막 조회 {new Date(observedAt).toLocaleTimeString('ko-KR', {hour: '2-digit', minute: '2-digit'})}
                </Text>
              )}
              <Text>
                확정 지급과 이행 통지를 확인하기 전에는 준비를 시작할 수
                없습니다.
              </Text>
              <Action
                title="기존 지급 결과 재조회"
                disabled={busy || !!pending}
                onPress={() => {
                  run(current => open(selected.saleRef, current));
                }}
              />
            </>
          )}
          {openReceipt && selected.state !== 'checking' && (
            <Action
              title="같은 원주문 영수증 열기"
              disabled={busy || !!pending}
              onPress={() => openReceipt(selected.saleRef)}
            />
          )}
          {selected.items?.map((item, index) => (
            <Text key={index}>
              {item.label} · {item.option} ×{item.quantity}
            </Text>
          ))}
          {next && (
            <Action
              title={actionLabel[next]}
              disabled={busy || !!pending}
              onPress={() => {
                run(current => submit(current, next));
              }}
            />
          )}
          {next && (
            <Action
              title="이행 불가 검토"
              disabled={busy || !!pending}
              onPress={() => setReject(true)}
            />
          )}
          {next && reject && (
            <>
              <Text>
                이 주문의 이행을 중단합니다. 환불은 같은 원주문을 기준으로
                재무에서 별도로 심사합니다.
              </Text>
              <Action
                title={actionLabel.unfulfillable}
                disabled={busy || !!pending}
                onPress={() => {
                  run(current => submit(current, 'unfulfillable'));
                }}
              />
              <Action
                title="이행 불가 검토 닫기"
                disabled={busy}
                onPress={() => setReject(false)}
              />
            </>
          )}
        </View>
      )}
      {pending && (
        <Action
          title="같은 픽업 상태 요청 다시 확인"
          disabled={busy}
          onPress={() => {
            run(submit);
          }}
        />
      )}
      {!!message && <Text accessibilityRole="alert">{message}</Text>}
      <Action title="단말 화면으로 돌아가기" disabled={busy} onPress={close} />
    </View>
  );
}

const styles = StyleSheet.create({
  panel: { gap: 7 },
  input: { borderWidth: 1, borderColor: '#839c91', padding: 10 },
  heading: { fontSize: 19, fontWeight: '600' },
  button: {
    backgroundColor: '#e5ede9',
    padding: 10,
    borderRadius: 5,
    alignItems: 'center',
  },
  disabled: { opacity: 0.5 },
});
