import { useEffect, useRef, useState } from 'react';
import {
  PermissionsAndroid,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import {
  commerceAmount,
  type CommerceClient,
  type CommercePaymentProfile,
  type SubscriptionInvoice,
  type SubscriptionOffer,
  type SubscriptionRecord,
} from '@nu54/protocol';
import Http from '../../specs/NativeCommerceHttp.ts';
import Alerts from '../../specs/NativeSubscriptionAlerts.ts';

const journal = 'subscription-alerts-v2';
const day = 86_400_000;

function reminderBeforeDue(now: number, due: number): number | undefined {
  const remaining = due - now;
  if (remaining <= 1_000) return undefined;
  const minimumDelay = remaining > 60_000 ? 60_000 : 1_000;
  return Math.max(now + minimumDelay, due - day);
}

let reminderUpdates: Promise<unknown> = Promise.resolve();

function updateReminders<T>(operation: () => Promise<T>): Promise<T> {
  const updated = reminderUpdates.then(operation);
  reminderUpdates = updated.catch(() => undefined);
  return updated;
}

async function readReminderScope(branch: string, owner: string, current = () => true) {
  if (!Http || !Alerts) return undefined;
  const previous = await Http.loadJournal(journal).then(raw => {
    try { return JSON.parse(raw) as LocalSettings; } catch { return undefined; }
  }).catch(() => undefined);
  if (!current()) return undefined;
  if (previous?.branch === branch && previous.owner === owner) return previous;
  try {
    await Alerts.clear();
  } catch (error) {
    const code = error instanceof Error && 'code' in error ? error.code : undefined;
    throw new Error(typeof code === 'string' && ['CLEAR_BUSY', 'CLEAR_DEADLINE', 'CLEAR_REMAINS'].includes(code)
      ? code : 'REMINDER_CLEAR');
  }
  if (!current()) return undefined;
  try {
    await Http.saveJournal(journal, '');
  } catch {
    throw new Error('REMINDER_JOURNAL_WRITE');
  }
  return undefined;
}

export async function isolateSubscriptionReminders(branch: string, owner: string, current = () => true) {
  await updateReminders(async () => current() ? readReminderScope(branch, owner, current) : undefined);
}

type LocalSettings = {
  owner: string;
  branch: string;
  enabled: boolean;
  scheduled: string[];
};

export function openInvoices(rows: SubscriptionInvoice[]) {
  return rows.filter(row =>
    ['preview', 'issued', 'checking'].includes(row.state),
  );
}

const press = (label: string, run: () => void, disabled = false) => (
  <Pressable accessibilityLabel={label} accessibilityRole="button" onPress={run} disabled={disabled} style={styles.action}>
    <Text style={styles.text}>{label}</Text>
  </Pressable>
);

function amount(offer: SubscriptionOffer, policy?: CommercePaymentProfile) {
  return commerceAmount(offer.amount, offer.asset, policy);
}

function cadence(offer: SubscriptionOffer) {
  return offer.cadence === 'monthly'
    ? `매월 ${offer.anchorDay}일 · ${offer.timezone}`
    : `매주 · ${offer.timezone}`;
}

export function SubscriptionScreen({
  client,
  policy,
  keyFor,
  onInvoice,
  alertRef,
  backToPayments,
}: {
  client: CommerceClient;
  policy?: CommercePaymentProfile;
  keyFor: () => Promise<string>;
  onInvoice: (sale: string) => void;
  alertRef?: string;
  backToPayments?: () => void;
}) {
  const [offers, setOffers] = useState<SubscriptionOffer[]>([]);
  const [records, setRecords] = useState<SubscriptionRecord[]>([]);
  const [invoices, setInvoices] = useState<SubscriptionInvoice[]>([]);
  const [local, setLocal] = useState<LocalSettings>();
  const [loaded, setLoaded] = useState(false);
  const scheduleRevision = useRef(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const [consent, setConsent] = useState<{
    offer: SubscriptionOffer;
    replaces?: SubscriptionRecord;
  }>();
  const [cancel, setCancel] = useState<SubscriptionRecord>();
  const { branchRef: branch, subjectRef: owner } = client.profile;
  const scope = useRef({ branch, owner });
  scope.current = { branch, owner };

  async function refresh() {
    const requestedBy = scope.current;
    const [available, enrolled, due] = await Promise.all([
      client.subscriptionOffers(),
      client.subscriptions(),
      client.invoices(),
    ]);
    if (!mounted.current || requestedBy.branch !== scope.current.branch ||
        requestedBy.owner !== scope.current.owner) return;
    setOffers(available);
    setRecords(enrolled);
    setInvoices(due);
    setLoaded(true);
  }

  useEffect(() => {
    let active = true;
    void updateReminders(() => active ? readReminderScope(branch, owner, () => active) : Promise.resolve(undefined))
      .catch(() => undefined)
      .then(previous => {
        if (active) setLocal(previous ?? { branch, owner, enabled: false, scheduled: [] });
      });
    Promise.all([
      client.subscriptionOffers(),
      client.subscriptions(),
      client.invoices(),
    ])
      .then(([available, enrolled, due]) => {
        if (!active) return;
        setOffers(available);
        setRecords(enrolled);
        setInvoices(due);
        setLoaded(true);
      })
      .catch(() => {
        if (active) setNotice('구독 목록을 다시 조회하세요.');
      });
    return () => {
      active = false;
    };
  }, [branch, client, owner]);

  useEffect(() => {
    if (!local?.enabled || local.branch !== branch || local.owner !== owner || !Alerts || !Http) return;
    let active = true;
    const revision = scheduleRevision.current;
    const canSchedule = () =>
      active && revision === scheduleRevision.current &&
      scope.current.branch === branch && scope.current.owner === owner;
    async function reconcile(settings: LocalSettings) {
      if (!canSchedule()) return;
      const allowed = await Alerts!.permission(false);
      if (!canSchedule()) return;
      if (!allowed) {
        await Alerts!.clear();
        if (!canSchedule()) return;
        const next = { ...settings, enabled: false, scheduled: [] };
        await Http!.saveJournal(journal, JSON.stringify(next));
        if (canSchedule()) {
          setLocal(next);
          setNotice('알림 권한이 없어도 미결 회차를 열 수 있습니다.');
        }
        return;
      }
      const now = Date.now();
      const reminders = new Map<string, {at: number; until: number}>();
      for (const row of openInvoices(invoices)) {
        if (row.state === 'checking' || row.stage === 'reconcile' ||
            row.stage === 'awaitingClosure') continue;
        const until = Date.parse(row.dueAt);
        const at = reminderBeforeDue(now, until);
        if (at !== undefined) reminders.set(row.id, {at, until});
      }
      for (const record of records) {
        if (record.state !== 'active' || record.needsConsent || !record.nextAt) continue;
        const at = Date.parse(record.nextAt);
        // Allow the due worker to commit the new cycle before opening the list.
        if (at > now && at - now < 90 * day)
          reminders.set(record.id, {at: at + 120_000, until: at + day});
      }
      for (const ref of settings.scheduled) {
        if (!canSchedule()) return;
        if (!reminders.has(ref)) await Alerts!.remove(ref);
      }
      const scheduled: string[] = [];
      for (const [ref, {at, until}] of reminders) {
        if (!canSchedule()) return;
        if (await Alerts!.schedule(ref, at, until)) {
          if (!canSchedule()) {
            await Alerts!.remove(ref);
            return;
          }
          scheduled.push(ref);
        }
      }
      if (!canSchedule()) return;
      if (JSON.stringify(settings.scheduled) !== JSON.stringify(scheduled)) {
        const next = { ...settings, scheduled };
        await Http!.saveJournal(journal, JSON.stringify(next));
        if (canSchedule()) setLocal(next);
      }
    }
    void updateReminders(() => reconcile(local)).catch(() => {
      if (active)
        setNotice(
          '알림을 예약할 수 없습니다. 미결 목록은 계속 사용할 수 있습니다.',
        );
    });
    return () => {
      active = false;
    };
  }, [branch, invoices, local, owner, records]);

  async function work(job: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setNotice('');
    try {
      await job();
    } catch {
      setNotice('요청 상태를 확인할 수 없습니다. 같은 구독을 다시 조회하세요.');
    } finally {
      setBusy(false);
    }
  }

  async function alerts() {
    if (!local || !Alerts || !Http) {
      setNotice('이 기기에서는 미결 목록에서 회차를 확인하세요.');
      return;
    }
    scheduleRevision.current++;
    const selected = local;
    await updateReminders(async () => {
      const current = () => mounted.current &&
        scope.current.branch === selected.branch && scope.current.owner === selected.owner;
      if (!current()) return;
      let enabled = false;
      if (!selected.enabled) {
        const allowed =
          Platform.OS !== 'android' ||
          Number(Platform.Version) < 33 ||
          (await PermissionsAndroid.request(
            PermissionsAndroid.PERMISSIONS.POST_NOTIFICATIONS,
          )) === PermissionsAndroid.RESULTS.GRANTED;
        enabled = allowed && (await Alerts!.permission(true));
        if (!current()) return;
        if (!enabled)
          setNotice('알림을 허용하지 않았습니다. 미결 목록은 사용할 수 있습니다.');
      } else {
        await Alerts!.clear();
      }
      if (!current()) return;
      const next = { ...selected, enabled, scheduled: [] };
      await Http!.saveJournal(journal, JSON.stringify(next));
      if (current()) setLocal(next);
    });
  }

  const restricted = alertRef && loaded &&
    !invoices.some(row => row.id === alertRef) &&
    !records.some(row => row.id === alertRef);
  const visibleInvoices = alertRef
    ? invoices.filter(row => row.id === alertRef || row.subscriptionRef === alertRef)
    : invoices;

  if (alertRef && (!loaded || restricted)) {
    return (
      <View style={styles.list}>
        <Text accessibilityLiveRegion="polite">
          {restricted ? '이 계정에서 열 수 없는 주문입니다.' : notice || '구독 목록 확인 중'}
        </Text>
        {backToPayments && press('내 결제로', backToPayments)}
      </View>
    );
  }
  const visibleRecords = alertRef ? records.filter(record => record.id === alertRef) : records;

  return (
    <View collapsable={false} style={styles.list}>
      <Text accessibilityRole="header">구독 관리</Text>
      {notice !== '' && <Text accessibilityLiveRegion="polite">{notice}</Text>}
      {press(
        '구독 목록 새로고침',
        () => {
          void work(refresh);
        },
        busy,
      )}
      {press(
        local?.enabled ? '회차 알림 끄기' : '회차 알림 허용',
        () => {
          void work(alerts);
        },
        busy,
      )}
      <Text accessibilityRole="header">미결 회차</Text>
      {!loaded && notice === '' && <Text>구독 목록 확인 중</Text>}
      {openInvoices(visibleInvoices).map(row => (
        <View key={row.id} collapsable={false} style={styles.invoice}>
          <Text style={styles.text}>
            {row.cycle} · {commerceAmount(row.amount, row.asset, policy)}
          </Text>
          <Text style={styles.text}>납기 {new Date(row.dueAt).toLocaleString()}</Text>
          <Text style={styles.text}>{row.fulfillment === 'digital' ? '디지털 이용권' : '현장 수령'}</Text>
          {press(
            row.stage === 'reconcile' ? '기존 회차 재조회' :
              row.stage === 'awaitingClosure' ? '납기 경과 회차 조회' :
              row.state === 'checking' ? '지급 결과 조회' : '같은 회차 결제 검토',
            () => onInvoice(row.saleRef),
            busy,
          )}
          {row.state === 'preview' && row.stage !== 'awaitingClosure' && press('회차 건너뛰기', () => {
            void work(async () => {
              await client.skipInvoice(row, await keyFor());
              await refresh();
            });
          }, busy)}
        </View>
      ))}
      {visibleInvoices
        .filter(row => row.state === 'paid' || row.state === 'closed')
        .map(row => (
          <View key={row.id} collapsable={false} style={styles.invoice}>
            <Text>
              {row.cycle} · {row.state === 'paid' ? '지급 확정' :
                row.stage === 'expired' ? '견적 만료 확정' : '종료'}
            </Text>
            {press('기존 회차 보기', () => onInvoice(row.saleRef), busy)}
          </View>
        ))}
      {visibleRecords.map(record => (
        <View key={record.id} collapsable={false} style={styles.invoice}>
          <Text>
            {record.label} · {record.termsVersion} · {amount(record, policy)}
          </Text>
          <Text>
            {cadence(record)} · 발급 뒤 {record.dueDays}일
          </Text>
          <Text>
            {record.state === 'cancelled'
              ? '해지됨'
              : record.state === 'paused'
              ? '일시정지'
              : '구독 중'}
          </Text>
          {record.nextAt && (
            <Text>다음 발급 {new Date(record.nextAt).toLocaleString()}</Text>
          )}
          {record.needsConsent && (
            <Text>새 약관에 동의하기 전 미래 청구는 보류됩니다.</Text>
          )}
          {record.needsConsent &&
            offers
              .filter(offer => offer.seriesRef === record.seriesRef)
              .map(offer => (
                <View key={offer.id}>
                  {press(
                    `변경 조건 확인 · ${offer.termsVersion}`,
                    () => setConsent({ offer, replaces: record }),
                    busy,
                  )}
                </View>
              ))}
          {record.state !== 'cancelled' && (
            <>
              {!record.needsConsent &&
                press(
                  record.state === 'paused' ? '구독 재개' : '일시정지',
                  () => {
                    void work(async () => {
                      await client.changeSubscription(
                        record,
                        record.state === 'paused' ? 'resume' : 'pause',
                        await keyFor(),
                      );
                      await refresh();
                    });
                  },
                  busy,
                )}
              {press('해지 검토', () => setCancel(record), busy)}
            </>
          )}
        </View>
      ))}
      {offers
        .filter(
          offer =>
            !records.some(
              record =>
                record.seriesRef === offer.seriesRef &&
                record.state !== 'cancelled',
            ),
        )
        .map(offer => (
          <View key={offer.id}>
            {press(
              `${offer.label} 구독 조건 보기`,
              () => setConsent({ offer }),
              busy,
            )}
          </View>
        ))}
      {consent && (
        <View>
          <Text>
            {consent.offer.label} · {consent.offer.termsVersion}
          </Text>
          <Text>
            회차 상한 {amount(consent.offer, policy)} · {cadence(consent.offer)} ·
            발급 뒤 {consent.offer.dueDays}일
          </Text>
          <Text selectable>약관 확인값 {consent.offer.termsDigest}</Text>
          <Text>{consent.offer.fulfillment === 'digital' ? '디지털 이용권' : '현장 수령'}</Text>
          <Text>회차마다 보드에서 직접 승인합니다.</Text>
          {press(
            '조건에 동의',
            () => {
              void work(async () => {
                if (consent.replaces)
                  await client.reconsentSubscription(
                    consent.replaces,
                    consent.offer,
                    await keyFor(),
                  );
                else
                  await client.acceptSubscription(
                    consent.offer,
                    await keyFor(),
                  );
                setConsent(undefined);
                await refresh();
              });
            },
            busy,
          )}
          {press('조건 닫기', () => setConsent(undefined), busy)}
        </View>
      )}
      {cancel && (
        <View>
          <Text>
            미래 회차를 해지합니다. 기존 발급 견적은 계속 조회할 수 있습니다.
          </Text>
          {press(
            '미래 회차 해지',
            () => {
              void work(async () => {
                await client.changeSubscription(
                  cancel,
                  'cancel',
                  await keyFor(),
                );
                setCancel(undefined);
                await refresh();
              });
            },
            busy,
          )}
          {press('돌아가기', () => setCancel(undefined), busy)}
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  list: {gap: 8},
  invoice: {gap: 4, borderWidth: 1, borderColor: '#bbcdc3', borderRadius: 8, padding: 12},
  action: {minHeight: 44, justifyContent: 'center', paddingHorizontal: 10, backgroundColor: '#eaf1ed', borderRadius: 6},
  text: {fontSize: 16, color: '#213a31'},
});
