import {useCallback, useEffect, useRef, useState} from 'react';
import {AppState, Pressable, StyleSheet, Text, TextInput, View} from 'react-native';
import {consentScope, privateTravel, travelID, TravelAuthorityError, type Consent, type TravelPolicy} from './privateClient.ts';
import type {Place} from './catalog.ts';

export type VisitScreen = 'review' | 'stamps' | 'challenge';
type Visit = {
  issuer: string;
  proof: string;
  place: string;
  observedAt: string;
  placeName?: string;
  environment: 'synthetic' | 'live';
  state: 'settled' | 'refunded' | 'reorg';
  challenge: 'unsubmitted' | 'adjusted' | 'test-only' | 'permission-held' | 'verified' | 'duplicate';
  claimRef?: string;
  review?: string;
  reviewState?: 'published' | 'adjusted' | 'private' | 'test-only';
  benefit?: {credited: number; adjusted: number; uncredited: number};
};
type VisitView = {options: Visit[]; challengeCount: number; benefitAvailable?: boolean; next?: string};
const purposeLabel = {review: '후기 공개', challenge: '챌린지 방문 확인'};
const date = new Intl.DateTimeFormat('ko-KR', {dateStyle: 'medium', timeStyle: 'short'});
const keyFor = (item: Visit) => item.issuer + '/' + item.proof;

function Button({title, onPress, disabled}: {title: string; onPress: () => void; disabled?: boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={title} onPress={onPress} disabled={disabled}
    style={[styles.button, disabled && styles.disabled]}><Text>{title}</Text></Pressable>;
}

function evidenceLabel(item: Visit) {
  if (item.state === 'refunded') return '환불로 이용 근거가 조정됐어요.';
  if (item.state === 'reorg') return '체인 재구성으로 이용 근거가 조정됐어요.';
  return item.environment === 'synthetic' ? '시험 이용 · 실구매 배지 및 실제 혜택 없음' : '실제 지급과 이용 확인';
}

function benefitLabel(item: Visit, available: boolean) {
  if (item.environment === 'synthetic') return '시험 근거 · 실제 혜택 없음';
  if (!available || !item.benefit) return '혜택 원장 · 현재 조회할 수 없음';
  const {credited, adjusted, uncredited} = item.benefit;
  const adjustment = adjusted ? ` · 회수 또는 조정 ${adjusted}건` : '';
  if (credited) return item.state === 'settled' && item.challenge === 'verified'
    ? `혜택 원장 · 확정 ${credited}건${adjustment}` : `혜택 원장 · 기존 반영 ${credited}건, 정정 대기${adjustment}`;
  if (adjusted) return `혜택 원장 · 회수 또는 조정 ${adjusted}건`;
  if (uncredited) return '혜택 원장 · 검토 결과 미반영';
  return '혜택 원장 · 확인 대기';
}

export function TravelVisits({screen, place, places, policies}: {
  screen: VisitScreen;
  place?: Place;
  places: Place[];
  policies: TravelPolicy[];
}) {
  const [view, setView] = useState<VisitView>();
  const [consents, setConsents] = useState<Consent[]>([]);
  const [selected, setSelected] = useState('');
  const [draft, setDraft] = useState('');
  const [prior, setPrior] = useState('');
  const [consentOpen, setConsentOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const cursors = useRef(['']);
  const [page, setPage] = useState(1);
  const purpose = screen === 'review' ? 'review' : 'challenge';
  const available = (view?.options ?? []).filter(item => screen !== 'review' || !place || item.place === place.id);
  const item = available.find(option => keyFor(option) === selected);
  const decision = consents.find(value => value.purpose === purpose);
  const current = decision?.accepted && Date.parse(decision.expiresAt) > Date.now() ? decision : undefined;
  const policy = policies.filter(value => value.purpose === purpose && value.recipient === 'p09')
    .sort((a, b) => b.version - a.version)[0];
  const authorized = current && (!current.object || current.object === item?.proof) ? current : undefined;
  const verifiedPlaces = new Set(available.filter(option => option.challenge === 'verified').map(option => option.place));
  const placeName = (visit: Visit) => visit.placeName ?? places.find(option => option.id === visit.place)?.name ?? (place?.id === visit.place ? place.name : '이용 장소');

  const placeFilter = screen === 'review' ? place?.id : undefined;
  const includeBenefits = screen === 'stamps';
  const loadPage = useCallback(async (cursor: string) => {
    const [visits, decisions] = await Promise.all([
      privateTravel<VisitView>('/visits', {
        projection: 'options', cursor: cursor || undefined,
        place: placeFilter, includeBenefits,
      }),
      privateTravel<{consents: Consent[]}>('/consents'),
    ]);
    setView(visits);
    setConsents(decisions.consents);
    return visits;
  }, [placeFilter, includeBenefits]);

  const refresh = useCallback(() => loadPage(cursors.current[cursors.current.length - 1]), [loadPage]);

  async function movePage(direction: 'next' | 'previous' | 'first') {
    const trail = cursors.current;
    if (direction === 'next' && !view?.next) return;
    const next = direction === 'first' ? [''] : direction === 'previous' ? trail.slice(0, -1) : [...trail, view!.next!];
    if (!next.length) return;
    await loadPage(next[next.length - 1]);
    cursors.current = next;
    setPage(next.length);
    clearSelection();
  }

  const run = useCallback(async (work: () => Promise<unknown>) => {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setMessage('');
    try {
      await work();
    } catch {
      setMessage('처리 결과를 확인할 수 없습니다. 상태를 다시 확인하거나 같은 내용으로 다시 시도해 주세요.');
    } finally {
      setBusy(false);
      lock.current = false;
    }
  }, []);

  useEffect(() => {
    run(refresh);
    const subscription = AppState.addEventListener('change', state => {
      if (state === 'active') run(refresh);
      else { setDraft(''); setPrior(''); setConsentOpen(false); setDeleteOpen(false); }
    });
    return () => subscription.remove();
  }, [refresh, run]);

  function clearSelection() {
    setSelected('');
    setDraft('');
    setPrior('');
    setConsentOpen(false);
    setDeleteOpen(false);
    setMessage('');
  }

  function select(option: Visit) {
    clearSelection();
    setSelected(keyFor(option));
  }

  async function decide(accept: boolean) {
    if (!policy || (accept && !item)) return;
    await privateTravel('/consents', consentScope(policy, decision, item?.proof ?? '', await travelID(), accept));
    setConsentOpen(false);
    await refresh();
    if (!accept) setMessage(purpose === 'review' ? '후기 공개 동의를 철회했습니다.' : '챌린지 동의를 철회했습니다. 유효 방문 진척이 회수됐어요.');
  }

  const evidence = (option: Visit) => ({issuer: option.issuer, proof: option.proof, place: option.place});

  async function inspect(option: Visit) {
    const result = await privateTravel<{body: string}>('/reviews', {...evidence(option), inspect: true});
    setPrior(result.body);
  }

  async function publish() {
    if (!item || !authorized) return;
    try {
      const result = await privateTravel<{review: string; published: boolean}>('/reviews', {
        ...evidence(item), consentRevision: authorized.revision, publish: item.environment === 'live', body: draft,
      });
      setDraft('');
      setPrior('');
      setView(previous => previous && {...previous, options: previous.options.map(option => keyFor(option) === keyFor(item)
        ? {...option, review: result.review, reviewState: result.published ? 'published' : 'test-only'} : option)});
      setMessage(result.published ? '후기가 게시됐어요.' : '시험 후기를 저장했어요.');
    } catch (error) {
      if (!(error instanceof TravelAuthorityError) || error.reason !== 'stale_or_conflicting_intent') throw error;
      await inspect(item);
      setMessage('이미 저장된 후기가 있어요. 기존 내용과 지금 입력한 내용을 비교해 주세요.');
    }
  }

  async function claim() {
    if (!item || !authorized) return;
    if (item.challenge === 'verified' || item.challenge === 'test-only') {
      setMessage('이미 반영한 이용 근거예요.');
      return;
    }
    if (item.environment === 'live' && verifiedPlaces.has(item.place)) {
      setMessage('이미 반영한 장소예요. 진척은 그대로입니다.');
      return;
    }
    await privateTravel('/visits', {...evidence(item), purpose: 'challenge', consentRevision: authorized.revision});
    const currentPage = await refresh();
    if (currentPage.options.find(option => keyFor(option) === keyFor(item))?.challenge === 'duplicate') {
      setMessage('이미 반영한 장소예요. 진척은 그대로입니다.');
      return;
    }
    setMessage(item.environment === 'synthetic' ? '시험 방문을 확인했어요. 진척과 실제 혜택은 그대로입니다.' : '검증된 방문을 반영했어요.');
  }

  async function erase() {
    const object = screen === 'review' ? item?.review : item?.claimRef;
    if (!object) return;
    await privateTravel('/erasure', {object, requestKey: await travelID(), kind: screen === 'review' ? 'review' : 'visit-claim'});
    setPrior('');
    setDeleteOpen(false);
    await refresh();
    setMessage(screen === 'review'
      ? '후기에 대한 접근을 차단했고 삭제 요청을 접수했어요. 삭제 영수증은 여행 기록에서 확인할 수 있습니다.'
      : '선택한 방문의 챌린지 진척을 회수했어요. 삭제 영수증은 여행 기록에서 확인할 수 있습니다.');
  }

  return <View style={styles.panel}>
    <Text accessibilityRole="header" style={styles.heading}>
      {screen === 'review' ? '후기와 이용 자격' : screen === 'stamps' ? '스탬프와 혜택' : '따라하기 챌린지'}
    </Text>
    {screen === 'challenge' && view && <Text>검증된 방문 {view.challengeCount} / 3
      {view.challengeCount === 3 ? ' · 완료' : ''}</Text>}
    {screen === 'challenge' && <Text>이용 내역을 선택해 검증된 장소 세 곳을 반영하세요. 혜택 상태는 스탬프에서 확인할 수 있어요.</Text>}
    {screen !== 'stamps' && current && policy && <Button title={purposeLabel[purpose] + ' 동의 철회'}
      onPress={() => run(() => decide(false))} disabled={busy} />}
    <Button title="이용과 혜택 상태 다시 조회" onPress={() => run(refresh)} disabled={busy} />
    {view && available.length === 0 && <Text>발급이 확인된 이용 내역이 없습니다.</Text>}
    {available.map(option => {
      const label = placeName(option) + ' · ' + date.format(new Date(option.observedAt));
      return <View key={keyFor(option)} style={styles.item}>
        {screen === 'stamps' ? <Text>{label}</Text> :
          <Button title={keyFor(option) === selected ? label + ' · 선택됨' : label + ' 선택'} onPress={() => select(option)} disabled={busy} />}
        <Text>{evidenceLabel(option)}</Text>
        {screen === 'review' && option.review && <Text>{
          option.reviewState === 'published' ? '공개 후기 · 실구매 배지' :
          option.reviewState === 'adjusted' ? '공개 후기 · 배지 조정' :
          option.reviewState === 'test-only' ? '저장된 시험 후기' : '저장된 후기 · 비공개'
        }</Text>}
        {screen === 'stamps' && <>
          <Text>{option.challenge === 'verified' ? '스탬프 · 검증' : option.challenge === 'permission-held' ? '스탬프 · 권한 보류' : '스탬프 · 미반영'}</Text>
          <Text>{benefitLabel(option, !!view?.benefitAvailable)}</Text>
        </>}
        {screen === 'challenge' && option.challenge === 'verified' && <Text>검증된 방문 · 반영됨</Text>}
        {screen === 'challenge' && option.challenge === 'duplicate' && <Text>같은 장소 · 진척 이미 반영</Text>}
      </View>;
    })}
    {(page > 1 || view?.next) && <View>
      <Text>이용 내역 {page}쪽</Text>
      {page > 1 && <Button title="이전 내역 페이지" onPress={() => run(() => movePage('previous'))} disabled={busy} />}
      {!!view?.next && <Button title="지난 이용 내역 더 보기" onPress={() => run(() => movePage('next'))} disabled={busy} />}
      {page > 2 && <Button title="처음 내역 페이지" onPress={() => run(() => movePage('first'))} disabled={busy} />}
    </View>}
    {screen !== 'stamps' && <>
      {item && (item.state === 'settled' || item.review || item.claimRef) && <View style={styles.item}>
        <Text style={styles.heading}>{placeName(item)}</Text>
        {screen === 'review' && item.state === 'settled' && <>
          <Text>공개 목록에는 후기 본문과 배지 상태가 표시됩니다. 주소나 결제 정보를 입력하지 마세요.</Text>
          <TextInput accessibilityLabel="후기 본문" value={draft} onChangeText={setDraft} multiline maxLength={800} style={styles.input} />
        </>}
        {policy && !authorized && item.state === 'settled' && <>
          <Button title={purposeLabel[purpose] + ' 동의 내용 확인'} onPress={() => setConsentOpen(true)} disabled={busy} />
          {consentOpen && <>
            <Text>{purpose === 'review' ?
              (item.environment === 'live' ? '이용 자격을 확인하고 이 후기의 본문과 배지 상태를 공개합니다.' : '이용 자격을 확인하고 시험 후기를 계정에 보관합니다.') :
              '선택해 제출한 이용 내역을 검증해 챌린지 진행을 계산합니다.'}</Text>
            <Text>여행 서비스 · 최대 {Math.ceil(policy.maxLifetimeSeconds / 3600)}시간 보관 · 언제든 동의 철회 가능</Text>
            <Text>{purpose === 'review' ? '후기는 선택한 뒤 삭제를 요청할 수 있습니다.' : '선택한 방문을 챌린지에서 삭제할 수 있습니다.'}</Text>
            <Button title={purposeLabel[purpose] + ' 동의'} onPress={() => run(() => decide(true))} disabled={busy} />
            <Button title="동의 검토 닫기" onPress={() => setConsentOpen(false)} />
          </>}
        </>}
        {screen === 'review' && <>
          {item.state === 'settled' && <Button title={item.environment === 'live' ? '후기 공개 게시' : '시험 후기 저장'}
            onPress={() => run(publish)} disabled={busy || !authorized || !draft.trim()} />}
          {item.review && <>
            <Button title="저장된 후기 보기" onPress={() => run(() => inspect(item))} disabled={busy} />
            <Button title="후기 삭제 검토" onPress={() => setDeleteOpen(true)} disabled={busy} />
            {deleteOpen && <Button title="이 후기 삭제 요청" onPress={() => run(erase)} disabled={busy} />}
          </>}
          {!!prior && <Text>저장된 후기 · {prior}</Text>}
        </>}
        {screen === 'challenge' && item.state === 'settled' && <Button title="선택한 방문 근거 반영"
          onPress={() => run(claim)} disabled={busy || !authorized} />}
        {screen === 'challenge' && item.claimRef && <>
          <Button title="챌린지 방문 삭제 검토" onPress={() => setDeleteOpen(true)} disabled={busy} />
          {deleteOpen && <Button title="이 챌린지 방문 삭제 요청" onPress={() => run(erase)} disabled={busy} />}
        </>}
      </View>}
    </>}
    {!!message && <Text accessibilityRole="alert">{message}</Text>}
  </View>;
}

const styles = StyleSheet.create({
  panel: {marginVertical: 12, gap: 8},
  heading: {fontSize: 19, fontWeight: '600'},
  item: {paddingVertical: 8, borderBottomWidth: 1, borderColor: '#cad8ce', gap: 5},
  button: {backgroundColor: '#e5ede9', padding: 11, alignItems: 'center', borderRadius: 5},
  disabled: {opacity: 0.5},
  input: {minHeight: 72, padding: 8, borderWidth: 1, borderColor: '#9aada0', borderRadius: 4, textAlignVertical: 'top'},
});
