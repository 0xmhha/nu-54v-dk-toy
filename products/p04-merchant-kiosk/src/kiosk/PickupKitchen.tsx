import { useEffect, useRef, useState } from 'react';
import {
  AccessibilityInfo,
  AppState,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  useWindowDimensions,
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
  checking: '결제 확인 중',
  accepted: '접수됨',
  preparing: '준비 중',
  ready: '준비 완료',
  collected: '수령 완료',
  unfulfillable: '이행 불가',
};
const actionLabel: Record<PickupTarget, string> = {
  preparing: '조리 시작',
  ready: '준비 완료 · 고객에게 알림',
  collected: '전달 완료',
  unfulfillable: '이행 불가 · 환불',
};
const confirmLabel = { collected: '전달 완료 확정', unfulfillable: '이행 불가 · 환불 확정' } as const;
// The money of these orders has already moved or is being decided, so no store action is valid.
const closedStage = { disputed: '이의 접수', refunded: '환불됨', paid: '대금 수령' } as const;
const closedOrder = Object.values(closedStage) as string[];
const updateNeeded = 'PICKUP_UPDATE';

/** Tells a stage only a newer app knows from an answer that is wrong in any other way. */
function readKitchen(raw: string) {
  try {
    return readPickupKitchen(raw);
  } catch (error) {
    const rows = (JSON.parse(raw) as { pickups?: { escrow?: unknown }[] } | null)?.pickups;
    if (!Array.isArray(rows) || !rows.some(row => row?.escrow !== undefined)) throw error;
    readPickupKitchen(JSON.stringify({ pickups: rows.map(row => ({ ...row, escrow: undefined })) }));
    throw new Error(updateNeeded);
  }
}
function closedLabel(item: PickupKitchenOrder) {
  const stage = item.escrow?.stage;
  return stage === 'refunded' || stage === 'disputed' || stage === 'paid' ? closedStage[stage] : undefined;
}
const refreshMs = 5_000;
const columns = [
  { state: 'accepted', title: '새 주문', tint: '#e9f3df' },
  { state: 'preparing', title: '준비 중', tint: '#f6e3bf' },
  { state: 'ready', title: '픽업 대기', tint: '#fffefa' },
] as const;
const clockText = (at: number) =>
  new Date(at).toLocaleTimeString('ko-KR', {hour: '2-digit', minute: '2-digit', second: '2-digit'});
function Action({
  title,
  onPress,
  disabled,
  kind = 'plain',
}: {
  title: string;
  onPress: () => void;
  disabled?: boolean;
  kind?: 'plain' | 'primary' | 'ready' | 'danger';
}) {
  const solid = kind !== 'plain';
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={title}
      accessibilityState={{ disabled: !!disabled }}
      onPress={onPress}
      disabled={disabled}
      style={({ pressed }) => [
        styles.button,
        kind === 'primary' && styles.primary,
        kind === 'ready' && styles.ready,
        kind === 'danger' && styles.danger,
        (disabled || pressed) && styles.disabled,
      ]}
    >
      <Text style={[styles.buttonText, solid && styles.solidText]}>{title}</Text>
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
  lock,
}: {
  journal: string;
  transport: (envelope: string) => Promise<string>;
  branch: string;
  actor: string;
  openReceipt?: (sale: string) => void;
  paymentProfile?: CommercePaymentProfile;
  allowCafe?: boolean;
  close: () => void;
  lock?: () => void;
}) {
  const [list, setList] = useState<PickupKitchenOrder[]>();
  const [selected, setSelected] = useState<PickupKitchenOrder>();
  const [observedAt, setObservedAt] = useState<number>();
  const [pending, setPending] = useState<PickupChange>();
  const [busy, setBusy] = useState(false);
  const [asking, setAsking] = useState<{ sale: string; revision: number; target: keyof typeof confirmLabel }>();
  const [message, setMessage] = useState('');
  const [lookup, setLookup] = useState('');
  const [refreshedAt, setRefreshedAt] = useState<number>();
  const [stale, setStale] = useState<'offline' | 'update'>();
  const [diagnostics, setDiagnostics] = useState(false);
  const [finding, setFinding] = useState(false);
  const [fresh, setFresh] = useState<string[]>([]);
  const seen = useRef<Set<string>>(undefined);
  const { width, height } = useWindowDimensions();
  const wide = width >= 840 && width > height;
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
      setStale(undefined);
      setDiagnostics(false);
      setFinding(false);
      setAsking(undefined);
      setRefreshedAt(undefined);
      setFresh([]);
      seen.current = undefined;
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

  async function run(job: (current: () => boolean) => Promise<unknown>) {
    if (activeJournals.has(journal)) {
      setMessage('앞에서 누른 처리를 확인하고 있습니다.');
      return;
    }
    activeJournals.add(journal);
    const generation = view.generation;
    const current = () => generation === view.generation;
    setBusy(true);
    setMessage('');
    try {
      await job(current);
    } catch (error) {
      if (!current()) return;
      if (error instanceof Error && error.message === updateNeeded) {
        // The last list this app could read stays; the notice says it may be old.
        setStale('update');
        return;
      }
      setList(undefined);
      setRefreshedAt(undefined);
      setSelected(undefined);
      setObservedAt(undefined);
      setAsking(undefined);
      setMessage(
        '주문을 확인하지 못했습니다. 직원 권한과 연결을 확인하고 다시 시도하세요.',
      );
    } finally {
      activeJournals.delete(journal);
      if (current()) setBusy(false);
    }
  }

  /** Every queue answer passes here, so an order that was not there before is announced once. */
  function showQueue(next: PickupKitchenOrder[]) {
    const known = seen.current;
    const arrived = known
      ? next.filter(item => item.state === 'accepted' && !closedLabel(item) && !known.has(item.saleRef)).map(item => item.saleRef)
      : [];
    seen.current = new Set(next.filter(item => item.state !== 'checking').map(item => item.saleRef));
    setFresh(old => [...old.filter(sale => next.some(item => item.saleRef === sale && item.state === 'accepted')), ...arrived]);
    if (arrived.length) AccessibilityInfo.announceForAccessibility(`새 주문 ${arrived.length}건이 들어왔습니다`);
    setList(next);
    setRefreshedAt(Date.now());
    setStale(undefined);
  }

  /** A read-only refresh: it never clears the open order and never runs beside a staff action. */
  async function refresh() {
    if (AppState.currentState === 'background' || activeJournals.has(journal)) return;
    if (!seen.current) {
      // Nothing was readable yet, so the first full load (with its journal) is still owed.
      if (stale === 'update') await run(load);
      return;
    }
    const generation = view.generation;
    try {
      const available = readKitchen(await exchange('pickup-queue'));
      if (generation === view.generation && !activeJournals.has(journal)) showQueue(available);
    } catch (error) {
      if (generation === view.generation)
        setStale(error instanceof Error && error.message === updateNeeded ? 'update' : 'offline');
    }
  }
  const latest = useRef({ refresh, open: () => run(load) });
  latest.current = { refresh, open: () => run(load) };
  useEffect(() => {
    latest.current.open();
    let timer: ReturnType<typeof setTimeout>;
    let live = true;
    const next = () => {
      timer = setTimeout(async () => {
        await latest.current.refresh();
        if (live) next();
      }, refreshMs);
    };
    next();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, []);

  async function load(current: () => boolean) {
    // Authorize the queue before attempting any v1 migration.
    const available = readKitchen(await exchange('pickup-queue'));
    if (!current()) return;
    const intent = await loadPickupIntent(journal, branch, actor);
    if (!current()) return;
    setPending(intent);
    showQueue(available);
  }

  async function open(sale: string, current: () => boolean, show = true) {
    const [item] = readKitchen(await exchange('pickup-detail', { sale }));
    if (!item || item.saleRef !== sale) throw new Error('Pickup unavailable');
    if (!current()) return undefined;
    // A card action confirms its result without opening a detail the clerk did not ask for.
    setSelected(old => show || old?.saleRef === sale ? item : old);
    setObservedAt(Date.now());
    setAsking(undefined);
    return item;
  }

  async function submit(current: () => boolean, target?: PickupTarget, order = selected) {
    let intent = await loadPickupIntent(journal, branch, actor);
    if (!current()) return;
    if (!intent) {
      if (!order || !target) return;
      intent = {
        branch,
        actor,
        sale: order.saleRef,
        requestKey: commerceRequestId(fromBase64(await Http!.randomBytes(16))),
        expectedRevision: order.revision,
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
    const confirmed = await open(intent.sale, current, !target).catch(error => {
      // The server already answered for this command; only its new stage is unreadable here.
      if (error instanceof Error && error.message === updateNeeded) return undefined;
      throw error;
    });
    if (!current()) return;
    // An explicit tombstone prevents a completed v1 copy from being adopted again.
    await Http!.saveJournal(journal, 'null');
    if (!current()) return;
    setPending(undefined);
    setAsking(undefined);
    // Should the queue be unreadable next, the kept list must not offer this step again.
    const done = { sale: intent.sale, state: confirmed?.state ?? (conflict ? undefined : intent.target),
      revision: confirmed?.revision ?? (conflict ? undefined : intent.expectedRevision + 1) };
    const settle = (row: PickupKitchenOrder): PickupKitchenOrder => row.saleRef !== done.sale ? row
      : { ...row, state: done.state ?? row.state, revision: done.revision ?? row.revision, ...(confirmed?.escrow && { escrow: confirmed.escrow }) };
    setList(old => old?.map(settle));
    setSelected(old => old && settle(old));
    if (conflict)
      setMessage('다른 단말에서 상태를 갱신했습니다. 현재 주문을 확인하세요.');
    const available = readKitchen(await exchange('pickup-queue'));
    if (current()) showQueue(available);
  }

  function badge(item: PickupKitchenOrder) {
    const label = closedLabel(item);
    return label && <Text style={styles.badge}>{label}</Text>;
  }

  function closedNote(item: PickupKitchenOrder) {
    if (item.escrow?.stage === 'disputed') return '구매자가 이의를 냈습니다. 재무 담당이 판정합니다. 매장에서 누를 것은 없습니다.';
    if (item.escrow?.stage === 'refunded') return item.state === 'collected'
      ? '결제 금액이 구매자에게 돌아갔습니다.' : '결제 금액이 구매자에게 돌아갔습니다. 물건을 건네지 마세요.';
    if (item.escrow?.stage === 'paid') return '대금을 받았습니다. 더 할 일이 없습니다.';
    return item.state === 'collected' ? '전달 완료로 기록했습니다. 더 누를 것은 없습니다.' : undefined;
  }

  /** The one set of actions for a card and for the open detail; irreversible ones ask once. */
  function actions(item: PickupKitchenOrder, detail: boolean) {
    const target = item.state === 'checking' ? undefined : nextTarget[item.state];
    const note = closedNote(selected?.saleRef === item.saleRef && closedLabel(selected) ? selected : item);
    if (note) return <Text style={styles.text}>{note}</Text>;
    if (!target) return null;
    const blocked = busy || !!pending;
    const escrow = item.escrow !== undefined;
    const asked = asking?.sale === item.saleRef && asking.revision === item.revision ? asking.target : undefined;
    const ask = (next: keyof typeof confirmLabel) => setAsking({ sale: item.saleRef, revision: item.revision, target: next });
    const send = (next: PickupTarget) => {
      setFresh(old => old.filter(sale => sale !== item.saleRef));
      run(current => submit(current, next, item));
    };
    return (
      <View style={styles.panel}>
        {!asked && (
          <Action
            kind={target === 'ready' ? 'ready' : 'primary'}
            title={actionLabel[target]}
            disabled={blocked}
            onPress={() => target === 'collected' ? ask(target) : send(target)}
          />
        )}
        {!asked && detail && (
          <Action title={actionLabel.unfulfillable} disabled={blocked} onPress={() => ask('unfulfillable')} />
        )}
        {asked && (
          <>
            <Text accessibilityRole="alert" style={styles.alert}>
              {asked === 'collected'
                ? escrow
                  ? `${item.pickupNo}번 손님에게 물건을 건넸나요? 확정하면 구매자의 확인 기간이 시작됩니다. 구매자가 수령을 확인하거나 이의 없이 기간이 지나야 대금을 받습니다. 되돌릴 수 없습니다.`
                  : `${item.pickupNo}번 손님에게 물건을 건넸나요? 확정하면 되돌릴 수 없습니다.`
                : escrow
                  ? `${item.pickupNo}번 주문을 만들 수 없나요? 확정하면 주문이 끝나고 결제 금액은 구매자에게 자동으로 돌아갑니다. 되돌릴 수 없습니다.`
                  : '이 주문을 중단합니다. 환불은 재무 담당이 따로 심사합니다. 되돌릴 수 없습니다.'}
            </Text>
            <Action
              kind={asked === 'collected' ? 'primary' : 'danger'}
              title={confirmLabel[asked]}
              disabled={blocked}
              onPress={() => send(asked)}
            />
            <Action title="확정하지 않기" disabled={busy} onPress={() => setAsking(undefined)} />
          </>
        )}
      </View>
    );
  }

  function queueItem(item: PickupKitchenOrder) {
    const checking = item.state === 'checking';
    const time = formatPickupTime(item.startsAt, item.timezone);
    return (
      <View key={item.saleRef} style={[styles.card, fresh.includes(item.saleRef) && styles.fresh]}>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={
            checking
              ? `확인 중 주문 · ${time}`
              : `픽업 ${item.pickupNo} · ${time} · ${stateLabel[item.state]}`
          }
          accessibilityState={{ disabled: busy || !!pending }}
          disabled={busy || !!pending}
          style={styles.cardHead}
          onPress={() => {
            setFresh(old => old.filter(sale => sale !== item.saleRef));
            run(current => open(item.saleRef, current));
          }}
        >
          <View style={styles.columnHead}>
            <Text style={checking ? styles.text : styles.number}>
              {checking ? '확인 중 주문' : item.pickupNo}
            </Text>
            {badge(item)}
          </View>
          <Text style={styles.muted}>{time}</Text>
          {item.settlement && <Text style={styles.muted}>
            {commerceCartAmount(item.settlement.amount, item.settlement.asset, paymentProfile)}
          </Text>}
          {!!item.items?.length && <Text style={styles.text}>
            {item.items.map(line => `${line.label}${line.option === 'standard' ? '' : ` ${line.option}`} ${line.quantity}`).join(' · ')}
          </Text>}
        </Pressable>
        {actions(item, false)}
        {item.state === 'ready' && openReceipt && (
          <Action title={`픽업 ${item.pickupNo} 영수증`} disabled={busy || !!pending} onPress={() => openReceipt(item.saleRef)} />
        )}
      </View>
    );
  }

  const closed = list?.filter(item => closedLabel(item))
    .sort((a, b) => closedOrder.indexOf(closedLabel(a)!) - closedOrder.indexOf(closedLabel(b)!));
  return (
    <View style={styles.panel}>
      <View style={styles.top}>
        <Text accessibilityRole="header" style={styles.heading}>
          픽업 접수와 준비
        </Text>
        <View style={styles.topActions}>
          <Action
            title="주문 목록 새로 고침"
            disabled={busy}
            onPress={() => {
              run(load);
            }}
          />
          <Action title="단말 화면으로 돌아가기" disabled={busy} onPress={close} />
          {lock && <Action title="고객 화면" onPress={lock} />}
        </View>
      </View>
      {stale === 'update' && (
        <Text accessibilityRole="alert" style={styles.alert}>
          {`이 앱이 모르는 주문 상태가 있어 목록을 새로 읽지 못합니다. 앱 업데이트가 필요합니다. 관리자에게 알려 주세요.${
            list && refreshedAt !== undefined ? ` 아래는 ${clockText(refreshedAt)}의 목록이며 새 주문이 빠져 있을 수 있습니다.` : ''}`}
        </Text>
      )}
      {list && refreshedAt !== undefined && stale !== 'update' && (
        <Text style={stale ? styles.alert : styles.muted}>
          {stale ? `새 주문을 확인하지 못하고 있습니다 · 마지막 갱신 ${clockText(refreshedAt)}`
            : `새 주문은 자동으로 올라옵니다 · 갱신 ${clockText(refreshedAt)}`}
        </Text>
      )}
      {fresh.length > 0 && (
        <Text accessibilityRole="alert" style={styles.alert}>{`새 주문 ${fresh.length}건`}</Text>
      )}
      {selected && (
        <View style={[styles.panel, styles.detail]}>
          {badge(selected)}
          <Text style={styles.columnTitle}>
            {selected.state === 'checking'
              ? stateLabel.checking
              : `주문번호 ${selected.pickupNo} · ${
                  stateLabel[selected.state]
                }`}
          </Text>
          {selected.settlement && (
            <>
              <Text style={styles.text}>결제 금액 {commerceCartAmount(selected.settlement.amount, selected.settlement.asset, paymentProfile)}</Text>
              <Text style={styles.text}>결제 확정 시각 {formatPickupTime(selected.settlement.finalizedAt, selected.timezone)}</Text>
              <Text style={styles.text}>픽업 가능 시각 {formatPickupTime(selected.settlement.pickupReadyAt, selected.timezone)}</Text>
            </>
          )}
          {selected.state === 'checking' && (
            <>
              {observedAt !== undefined && (
                <Text style={styles.muted}>
                  마지막 확인 {new Date(observedAt).toLocaleTimeString('ko-KR', {hour: '2-digit', minute: '2-digit'})}
                </Text>
              )}
              <Text style={styles.text}>결제가 확정되기 전에는 조리를 시작하지 마세요.</Text>
              <Action
                title="결제 결과 다시 확인"
                disabled={busy || !!pending}
                onPress={() => {
                  run(current => open(selected.saleRef, current));
                }}
              />
            </>
          )}
          {openReceipt && selected.state !== 'checking' && (
            <Action
              title="영수증 보기"
              disabled={busy || !!pending}
              onPress={() => openReceipt(selected.saleRef)}
            />
          )}
          {selected.items?.map((item, index) => (
            <Text key={index} style={styles.text}>
              {item.label} · {item.option} ×{item.quantity}
            </Text>
          ))}
          {actions(list?.find(item => item.saleRef === selected.saleRef && item.revision >= selected.revision) ?? selected, true)}
          <Action title={diagnostics ? '진단 정보 닫기' : '진단 정보'} onPress={() => setDiagnostics(old => !old)} />
          {diagnostics && (
            <>
              <Text selectable style={styles.muted}>주문 식별 번호 {selected.saleRef}</Text>
              {selected.eventRef && <Text selectable style={styles.muted}>전달 식별 번호 {selected.eventRef}</Text>}
            </>
          )}
          <Action title="상세 닫기" disabled={busy} onPress={() => { setSelected(undefined); setDiagnostics(false); }} />
        </View>
      )}
      {list?.length === 0 && <Text style={styles.text}>접수된 픽업이 없습니다.</Text>}
      {list && <View style={wide ? styles.columns : styles.panel}>
        {columns.map(column => {
          const rows = list.filter(item => item.state === column.state && !closedLabel(item));
          return (
            <View key={column.state} accessibilityLabel={`${column.title} ${rows.length}건`}
              style={[styles.column, wide && styles.columnWide, { backgroundColor: column.tint }]}>
              <View style={styles.columnHead}>
                <Text accessibilityRole="header" style={styles.columnTitle}>{column.title}</Text>
                <Text style={styles.columnTitle}>{rows.length}</Text>
              </View>
              {rows.map(item => queueItem(item))}
            </View>
          );
        })}
      </View>}
      {list?.some(item => item.state === 'checking') && (
        <View style={styles.panel}>
          <Text style={styles.muted}>결제가 확정된 주문만 위에 올라옵니다. 아래 주문은 확인 중입니다.</Text>
          {list.filter(item => item.state === 'checking').map(item => queueItem(item))}
        </View>
      )}
      {!!closed?.length && (
        <View style={[styles.column, styles.closedArea]}>
          <View style={styles.columnHead}>
            <Text accessibilityRole="header" style={styles.columnTitle}>처리 끝남·확인 필요</Text>
            <Text style={styles.columnTitle}>{closed.length}</Text>
          </View>
          {closed.map(item => queueItem(item))}
        </View>
      )}
      {list?.some(item => item.state === 'collected' && !closedLabel(item)) && (
        <View style={styles.panel}>
          <Text accessibilityRole="header" style={styles.columnTitle}>전달한 주문</Text>
          {list.filter(item => item.state === 'collected' && !closedLabel(item)).map(item => queueItem(item))}
        </View>
      )}
      {pending && (
        <Action
          title="방금 누른 처리 결과 확인"
          disabled={busy}
          onPress={() => {
            run(submit);
          }}
        />
      )}
      {!!message && <Text accessibilityRole="alert" style={styles.alert}>{message}</Text>}
      {allowCafe && (
        <Action title={finding ? '주문 직접 찾기 닫기' : '주문 직접 찾기'} onPress={() => setFinding(old => !old)} />
      )}
      {allowCafe && finding && (
        <>
          <TextInput
            accessibilityLabel="주문 식별 번호"
            value={lookup}
            onChangeText={setLookup}
            maxLength={36}
            autoCapitalize="none"
            editable={!busy && !pending}
            style={styles.input}
          />
          <Action
            title="주문 식별 번호로 찾기"
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
    </View>
  );
}

const styles = StyleSheet.create({
  panel: { gap: 10 },
  input: { borderWidth: 1, borderColor: '#71897d', borderRadius: 12, minHeight: 48, padding: 12, color: '#182d25', backgroundColor: '#fffefa' },
  heading: { fontSize: 26, fontWeight: '800', color: '#182d25' },
  text: { fontSize: 16, lineHeight: 23, color: '#182d25' },
  muted: { fontSize: 14, lineHeight: 20, color: '#4f655a' },
  alert: { fontSize: 16, lineHeight: 23, fontWeight: '700', color: '#5a3b05', backgroundColor: '#f6e3bf', borderRadius: 12, padding: 12 },
  columns: { flexDirection: 'row', gap: 16, alignItems: 'stretch' },
  column: { gap: 10, padding: 16, borderRadius: 20, borderWidth: 1, borderColor: '#d5dfd3' },
  columnWide: { flex: 1, minHeight: 240 },
  closedArea: { backgroundColor: '#f1f4ec' },
  columnHead: { flexDirection: 'row', justifyContent: 'space-between' },
  columnTitle: { fontSize: 18, fontWeight: '800', color: '#182d25' },
  detail: { padding: 20, borderRadius: 20, borderWidth: 1, borderColor: '#d5dfd3', backgroundColor: '#fffefa' },
  button: {
    minHeight: 52,
    justifyContent: 'center',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: '#d5dfd3',
    backgroundColor: '#fffefa',
  },
  buttonText: { fontSize: 16, fontWeight: '700', color: '#182d25', textAlign: 'center' },
  solidText: { color: '#fffefa', fontWeight: '800' },
  primary: { backgroundColor: '#174b38', borderColor: '#174b38' },
  ready: { backgroundColor: '#8a5a00', borderColor: '#8a5a00' },
  danger: { backgroundColor: '#9b2c2c', borderColor: '#9b2c2c' },
  top: { flexDirection: 'row', flexWrap: 'wrap', gap: 12, justifyContent: 'space-between', alignItems: 'center' },
  topActions: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  card: { borderRadius: 16, borderWidth: 1, borderColor: '#d5dfd3', backgroundColor: '#fffefa', padding: 16, gap: 10 },
  cardHead: { minHeight: 48, gap: 4 },
  badge: {
    fontSize: 14, fontWeight: '800', color: '#5c1f1f', backgroundColor: '#f8e1df', borderRadius: 999,
    paddingHorizontal: 12, paddingVertical: 4, overflow: 'hidden', alignSelf: 'flex-start',
  },
  fresh: { borderWidth: 2, borderColor: '#174b38' },
  number: { fontSize: 34, fontWeight: '800', color: '#182d25' },
  disabled: { opacity: 0.5 },
});
