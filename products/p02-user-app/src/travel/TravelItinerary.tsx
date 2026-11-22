import {useCallback, useEffect, useLayoutEffect, useRef, useState} from 'react';
import {Keyboard, Pressable, StyleSheet, Text, TextInput, View} from 'react-native';
import type {Place} from './catalog.ts';
import {privateTravel, travelID, TravelAuthorityError, type Consent, type TravelPolicy} from './privateClient.ts';

type Stop = {place: string; start: string; end: string};
type Detail = {name: string; category?: string; opening: {timezone: string; opens: string; closes: string};
  source: string; updatedAt: string; estimatedCost?: string};
type Plan = {id: string; consentRevision: number; catalogVersion: number; expiresAt: string;
  budgetLimit: string; budgetUnit?: string; stops: Stop[]; details?: Detail[];
  warnings?: string[]; projectedCost?: string};
type Preview = {preview: Plan; confirmation: string; provider?: string};
type Summary = {id: string; revision: number; place: string; name?: string; start: string; timezone?: string};
type Saved = {revision: number; plan: Plan};
type Reviewed = Preview & {baseRevision: number; lastComparison?: Comparison};
type Comparison = {current: Saved; previous: Preview; baseRevision: number; chosen: number[]};
const stopKey = ({place, start, end}: Stop) => JSON.stringify([place, Date.parse(start), Date.parse(end)]);

const localDate = (date: Date, timeZone: string) => new Intl.DateTimeFormat('ko-KR', {
  timeZone, month: 'long', day: 'numeric', weekday: 'short',
}).format(date);
const localTime = (date: Date, timeZone: string) => new Intl.DateTimeFormat('ko-KR', {
  timeZone, hour: '2-digit', minute: '2-digit', hour12: false,
}).format(date);
const cost = (amount: string) => new Intl.NumberFormat('ko-KR', {
  style: 'currency', currency: 'KRW', maximumFractionDigits: 0,
}).format(Number(amount));

function localParts(date: Date, timeZone: string) {
  const values = new Intl.DateTimeFormat('en-GB', {
    timeZone, year: 'numeric', month: 'numeric', day: 'numeric', hour: 'numeric',
    minute: 'numeric', hourCycle: 'h23',
  }).formatToParts(date);
  const number = (type: string) => Number(values.find(part => part.type === type)?.value);
  return [number('year'), number('month') - 1, number('day'), number('hour'), number('minute')] as const;
}

function visitTime(day: number, hour: number, timeZone: string, anchor: number) {
  const [year, month, date] = localParts(new Date(anchor), timeZone);
  const wallTime = Date.UTC(year, month, date + day, hour);
  let instant = wallTime;
  for (let i = 0; i < 4; i++) {
    const parts = localParts(new Date(instant), timeZone);
    const difference = wallTime - Date.UTC(...parts);
    if (!difference) return new Date(instant);
    instant += difference;
  }
  throw new Error('Local visit time is unavailable');
}

function Button({label, action, disabled}: {label: string; action: () => void; disabled?: boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={label} onPress={action} disabled={disabled}
    style={[styles.button, disabled && styles.disabled]}><Text>{label}</Text></Pressable>;
}

type Draft = {anchor: number; day: number; from: number; until: number; budget: string; preference: 'all'|'restaurant'|'attraction'};
type ProposalInputs = {start: string; end: string; budget: string; preference: Draft['preference']};
const defaultDraft = (): Draft => ({anchor: Date.now(), day: 1, from: 10, until: 14, budget: '15000', preference: 'all'});
type ItineraryProps = {place?: Place; places: Place[]; catalogVersion?: number; consent?: Consent;
  scopeEpoch?: number; scopeCurrent?: () => boolean; verifyConsent?: (consent: Consent) => Promise<boolean>;
  manageConsent?: () => void; policy?: TravelPolicy; approveRecommendation?: () => Promise<Consent | undefined>};

function requestInputs(draft: Draft, zone: string): ProposalInputs {
  if (!/^[1-9]\d{0,8}$/.test(draft.budget) || draft.from >= draft.until) throw new Error('Invalid itinerary draft');
  return {start: visitTime(draft.day, draft.from, zone, draft.anchor).toISOString(), end: visitTime(draft.day, draft.until, zone, draft.anchor).toISOString(),
    budget: draft.budget, preference: draft.preference};
}

function DraftInputs({draft, change, zone, disabled}: {draft: Draft; change: (update: Partial<Draft>) => void; zone: string; disabled: boolean}) {
  const {day, from, until, budget, preference} = draft;
  return <>
    <Text>{localDate(visitTime(day, 12, zone, draft.anchor), zone)} · {zone}</Text>
    <View style={styles.row}>
      <Button label="이전 날짜" disabled={disabled || day <= 0} action={() => change({day: day - 1})} />
      <Button label="다음 날짜" disabled={disabled || day >= 7} action={() => change({day: day + 1})} />
    </View>
    <Text>방문 시작 {from}:00 · 방문 종료 {until}:00</Text>
    <View style={styles.row}>
      <Button label="시작 앞당기기" disabled={disabled || from <= 0} action={() => change({from: from - 1})} />
      <Button label="시작 늦추기" disabled={disabled || from + 1 >= until} action={() => change({from: from + 1})} />
      <Button label="종료 앞당기기" disabled={disabled || until - 1 <= from} action={() => change({until: until - 1})} />
      <Button label="종료 늦추기" disabled={disabled || until >= 23} action={() => change({until: until + 1})} />
    </View>
    <View style={styles.row}>
      <TextInput accessibilityLabel="일정 예산 상한 (원)" keyboardType="number-pad" maxLength={9}
        editable={!disabled} value={budget} onChangeText={value => change({budget: value})} style={styles.input} />
      <Button label="예산 입력 완료" disabled={disabled} action={Keyboard.dismiss} />
    </View>
    <View style={styles.row}>
      <Button label="전체 선호" disabled={disabled || preference === 'all'} action={() => change({preference: 'all'})} />
      <Button label="맛집 선호" disabled={disabled || preference === 'restaurant'} action={() => change({preference: 'restaurant'})} />
      <Button label="명소 선호" disabled={disabled || preference === 'attraction'} action={() => change({preference: 'attraction'})} />
    </View>
  </>;
}

export function TravelItinerary({consent, policy, approveRecommendation, ...props}: ItineraryProps) {
  const [draft, setDraft] = useState(defaultDraft);
  const [disclosure, setDisclosure] = useState<ProposalInputs>();
  const [queued, setQueued] = useState<{receipt: string; inputs: ProposalInputs}>();
  const [authorizing, setAuthorizing] = useState(false);
  const consumeProposal = useCallback(() => setQueued(undefined), []);
  const [notice, setNotice] = useState('');
  function showDisclosure() {
    Keyboard.dismiss();
    setNotice('');
    try {
      const inputs = requestInputs(draft, zone);
      if (policy && Date.parse(inputs.end) > Date.now() + (policy.maxLifetimeSeconds - 5) * 1000) {
        setNotice('동의 기간 안의 방문 날짜와 시간을 선택해 주세요.');
        return;
      }
      setDisclosure(inputs);
    } catch {setNotice('선택한 날짜, 시간과 예산을 확인해 주세요.');}
  }
  const consentTask = useRef(false);
  const [, expire] = useState(0);
  const expiry = Date.parse(consent?.expiresAt ?? '');
  const zone = props.place?.opening?.[0]?.timezone ?? 'Asia/Seoul';
  const changeDraft = (update: Partial<Draft>) => setDraft(previous => ({...previous, ...update}));
  useEffect(() => {
    if (!consent?.accepted || !(expiry > Date.now())) return;
    const timer = setTimeout(() => expire(value => value + 1), expiry - Date.now());
    return () => clearTimeout(timer);
  }, [consent?.accepted, expiry]);

  async function consentAndGenerate() {
    if (consentTask.current || !disclosure || !approveRecommendation) return;
    consentTask.current = true;
    setAuthorizing(true);
    setNotice('');
    try {
      const accepted = await approveRecommendation();
      if (accepted) setQueued({receipt: accepted.receipt, inputs: disclosure});
      else setNotice('추천 동의를 확인하지 못했습니다. 상태를 다시 확인해 주세요.');
    } catch {
      setNotice('추천 동의를 확인하지 못했습니다. 다시 시도해 주세요.');
    } finally {
      consentTask.current = false;
      setDisclosure(undefined);
      setAuthorizing(false);
    }
  }

  if (authorizing || consent?.purpose !== 'recommendation' || !consent.accepted || !(expiry > Date.now())) {
    return <View style={styles.section}>
      <Text accessibilityRole="header" style={styles.heading}>내 일정과 추천</Text>
      {props.place && props.catalogVersion ? <DraftInputs draft={draft} change={changeDraft} zone={zone} disabled={!!disclosure} /> : null}
      {disclosure && policy ? <View style={styles.visit}>
        <Text accessibilityRole="header">추천 제공 동의를 확인하세요</Text>
        <Text>{localDate(new Date(disclosure.start), zone)} · {localTime(new Date(disclosure.start), zone)}–{localTime(new Date(disclosure.end), zone)} · {cost(disclosure.budget)}</Text>
        <Text>수신 범위 {policy.recipient} · 정책 {policy.version} · 최대 {Math.round(policy.maxLifetimeSeconds / 3600)}시간</Text>
        <Text>여행 서비스가 날짜와 예산으로 후보를 확인합니다. 기기 기록의 동의 관리에서 철회하거나 삭제할 수 있습니다.</Text>
        <Button label="동의하고 생성 요청" disabled={authorizing} action={() => {consentAndGenerate();}} />
        <Button label="동의하지 않음" disabled={authorizing} action={() => setDisclosure(undefined)} />
      </View> : <>
        <Text>일정 제안을 요청하려면 여행 추천 목적의 동의를 선택해 주세요.</Text>
        {props.place && props.catalogVersion && policy && approveRecommendation ? <Button label="추천 동의 확인"
          disabled={!/^[1-9]\d{0,8}$/.test(draft.budget)}
          action={showDisclosure} /> : null}
      </>}
      {!!notice && <Text accessibilityRole="alert">{notice}</Text>}
    </View>;
  }
  return <ItinerarySession key={JSON.stringify([props.scopeEpoch, consent.receipt, consent.revision, consent.policyVersion,
    consent.recipient, consent.expiresAt])} consent={consent} draft={draft} changeDraft={changeDraft}
    initialProposal={queued?.receipt === consent.receipt ? queued.inputs : undefined} consumeProposal={consumeProposal} {...props} />;
}

function ItinerarySession({place, places, catalogVersion, consent, scopeCurrent, verifyConsent, draft, changeDraft, initialProposal, consumeProposal, manageConsent}: ItineraryProps & {
  consent: Consent; draft: Draft; changeDraft: (update: Partial<Draft>) => void; initialProposal?: ProposalInputs; consumeProposal: () => void;
}) {
  const [list, setList] = useState<Summary[]>([]);
  const [saved, setSaved] = useState<Saved>();
  const [preview, setPreview] = useState<Reviewed>();
  const [status, setStatus] = useState('');
  const [conflictID, setConflictID] = useState('');
  const [comparison, setComparison] = useState<Comparison>();
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const generation = useRef<AbortController>(undefined);
  const consumedInitialProposal = useRef(false);
  const [generating, setGenerating] = useState(false);
  const mounted = useRef(false);
  const expiry = Date.parse(consent.expiresAt);
  const active = useCallback(() => mounted.current && Date.now() < expiry && (scopeCurrent?.() ?? true), [expiry, scopeCurrent]);
  useLayoutEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; generation.current?.abort(); };
  }, []);
  const pendingApply = useRef<{confirmation: string; key: string} | undefined>(undefined);
  const zone = place?.opening?.[0]?.timezone ?? 'Asia/Seoul';
  const title = (id: string) => places.find(item => item.id === id)?.name ?? '저장한 여행지';

  const run = useCallback(async (job: () => Promise<void>, error: string) => {
    if (lock.current || generation.current || !active()) return;
    lock.current = true;
    setBusy(true);
    setStatus('');
    try { await job(); }
    catch { if (active()) setStatus(error); }
    finally { lock.current = false; if (active()) setBusy(false); }
  }, [active]);

  const refresh = useCallback(async () => {
    const result = await privateTravel<{itineraries: Summary[]}>('/records', {projection: 'saved-itineraries'});
    if (active()) setList(result.itineraries);
  }, [active]);

  async function reload() {
    if (verifyConsent && !await verifyConsent(consent)) return;
    if (active()) await refresh();
  }

  async function open(id: string) {
    const result = await privateTravel<Saved>(`/itineraries/${id}`);
    if (!active()) return;
    setSaved(result);
    setPreview(undefined);
    pendingApply.current = undefined;
    setConflictID('');
    setComparison(undefined);
  }

  async function compareLatest() {
    if (!preview || !conflictID) throw new Error();
    const current = await privateTravel<Saved>(`/itineraries/${conflictID}`);
    if (!active()) return;
    setSaved(current);
    const previous = {preview: preview.preview, confirmation: preview.confirmation, provider: preview.provider};
    setComparison({current, previous, baseRevision: preview.baseRevision, chosen: []});
    setPreview(undefined);
    pendingApply.current = undefined;
  }

  async function reviewSelection() {
    if (!comparison || !comparison.chosen.length || !catalogVersion) throw new Error();
    const {current, previous} = comparison;
    const {id, budgetLimit, budgetUnit, stops: retained} = current.plan;
    const selected = comparison.chosen.map(index => previous.preview.stops[index]);
    const unique = new Map([...retained, ...selected].map(stop => [stopKey(stop), stop]));
    const stops = Array.from(unique.values()).sort((a, b) => Date.parse(a.start) - Date.parse(b.start));
    const result = await privateTravel<Preview>('/itineraries/preview', {
      id, consentRevision: consent.revision, catalogVersion, expiresAt: consent.expiresAt,
      budgetLimit, budgetUnit, stops, expectedRevision: current.revision,
    });
    if (!active() || result.preview.consentRevision !== consent.revision) return;
    setPreview({...result, baseRevision: current.revision, lastComparison: comparison});
    setComparison(undefined);
    setConflictID('');
  }

  const propose = useCallback(async (generate: boolean, inputs: ProposalInputs, current = active) => {
    if (!place || !catalogVersion) throw new Error('Missing itinerary source');
    const {start, end, budget, preference} = inputs;
    if (Date.parse(end) > expiry) throw new Error('Visit is outside consent period');
    const id = saved?.plan.id ?? await travelID();
    if (!current()) return;
    const scope = {id, consentRevision: consent.revision, catalogVersion,
      expectedRevision: saved?.revision ?? 0, expiresAt: consent.expiresAt, budgetLimit: budget, budgetUnit: 'KRW'};
    if (generate && !place.region) throw new Error('Missing itinerary region');
    const result = await privateTravel<Preview>(generate ? '/itineraries/generate' : '/itineraries/preview',
      generate ? {...scope, region: place.region, preference, start, end} : {...scope, stops: [{place: place.id, start, end}]});
    if (!current() || result.preview.consentRevision !== consent.revision) return;
    setPreview({...result, baseRevision: scope.expectedRevision});
  }, [active, catalogVersion, consent.expiresAt, consent.revision, expiry, place, saved]);

  const generate = useCallback(async (inputs: ProposalInputs) => {
    if (lock.current || generation.current || !active()) return;
    const request = new AbortController();
    const current = () => active() && !request.signal.aborted;
    generation.current = request;
    setGenerating(true);
    setStatus('');
    setPreview(undefined);
    try {await propose(true, inputs, current);}
    catch {if (current()) setStatus('선택한 시간과 예산의 코스를 찾지 못했습니다.');}
    finally {
      if (current() && generation.current === request) {
        generation.current = undefined;
        setGenerating(false);
      }
    }
  }, [active, propose]);

  const loaded = useRef(false);
  useEffect(() => {
    if (!loaded.current) {
      loaded.current = true;
      if (!initialProposal) run(refresh, '저장한 일정을 불러오지 못했습니다.');
      else refresh().catch(() => {});
    }
    if (initialProposal && !consumedInitialProposal.current) {
      consumedInitialProposal.current = true;
      generate(initialProposal);
      consumeProposal();
    }
  }, [consumeProposal, generate, initialProposal, refresh, run]);

  function cancelGeneration() {
    generation.current?.abort();
    generation.current = undefined;
    setGenerating(false);
    setStatus('추천 생성을 취소했어요. 같은 입력으로 다시 요청할 수 있습니다.');
  }

  async function apply() {
    if (!preview) throw new Error();
    const {id, consentRevision, catalogVersion: version, expiresAt, budgetLimit, budgetUnit, stops} = preview.preview;
    if (pendingApply.current?.confirmation !== preview.confirmation) {
      const key = await travelID();
      if (!active()) return;
      pendingApply.current = {confirmation: preview.confirmation, key};
    }
    let result;
    try {
      result = await privateTravel<{revision: number}>('/itineraries/apply', {
      id, consentRevision, catalogVersion: version, expiresAt, budgetLimit, budgetUnit, stops,
        expectedRevision: preview.baseRevision, requestKey: pendingApply.current.key, confirmedPreview: preview.confirmation,
      });
    } catch (error) {
      if (!active()) return;
      if (!(error instanceof TravelAuthorityError)) throw error;
      if (error.reason === 'stale_or_conflicting_intent') {
        setConflictID(id);
        setStatus('다른 곳에서 일정이 바뀌었습니다. 최신 일정을 열고 다시 검토해 주세요.');
        return;
      }
      if (error.reason === 'review_current_preview' || error.reason === 'itinerary_needs_comparison') {
        pendingApply.current = undefined;
        if (preview.lastComparison) setComparison(preview.lastComparison);
        setPreview(undefined);
        setStatus('제안을 다시 확인해 주세요. 변경된 영업시간이나 가격을 검토한 후 적용할 수 있습니다.');
        return;
      }
      throw error;
    }
    if (!active()) return;
    pendingApply.current = undefined;
    setSaved({revision: result.revision, plan: preview.preview});
    setPreview(undefined);
    setStatus('검토한 여행 일정을 저장했습니다.');
    try { await refresh(); }
    catch { if (active()) setStatus('일정을 저장했습니다. 목록은 다시 불러올 수 있습니다.'); }
  }

  function visitRows(plan: Plan) {
    return plan.stops.map((stop, i) => {
      const details = plan.details?.[i];
      const zoneForStop = details?.opening.timezone ?? zone;
      return <View key={`${stop.place}/${stop.start}`} style={styles.visit}>
        <Text style={styles.item}>{i + 1}. {details?.name ?? title(stop.place)}</Text>
        <Text>{details?.category === 'restaurant' ? '맛집' : details?.category === 'attraction' ? '명소' : '장소'}</Text>
        <Text>{localDate(new Date(stop.start), zoneForStop)} · {localTime(new Date(stop.start), zoneForStop)}–{localTime(new Date(stop.end), zoneForStop)}</Text>
        {details && <>
          <Text>게시된 영업시간 {details.opening.opens}–{details.opening.closes} · {zoneForStop}</Text>
          <Text>{details.estimatedCost === undefined || plan.budgetUnit !== 'KRW' ? '예상 비용 확인 필요' : `예상 비용 ${cost(details.estimatedCost)}`}</Text>
          <Text>출처 {details.source} · {localDate(new Date(details.updatedAt), zoneForStop)} 갱신</Text>
        </>}
      </View>;
    });
  }

  function updateInput(update: () => void) {
    update();
    if (!conflictID && !comparison) setPreview(undefined);
  }

  function closePreview() {
    if (preview?.lastComparison) setComparison(preview.lastComparison);
    setPreview(undefined);
    setConflictID('');
  }

  const reviewBlocked = busy || generating || !!conflictID || !!comparison;

  return <View style={styles.section}>
    <Text accessibilityRole="header" style={styles.heading}>내 일정과 추천</Text>
    {preview && <View style={styles.visit}>
      <Text accessibilityRole="header" style={styles.item}>{preview.provider ? '추천 일정 검토' : '방문 일정 검토'}</Text>
      <Text>기준 일정 버전 {preview.baseRevision} · 예산 상한 {cost(preview.preview.budgetLimit)}</Text>
      <Text>{preview.preview.projectedCost === undefined ? '전체 실제 비용 확인 필요' : `출처 기준 예상 합계 ${cost(preview.preview.projectedCost)}`}</Text>
      {visitRows(preview.preview)}
      <Button label="검토한 일정 적용" disabled={busy || !!conflictID} action={() => {run(apply, '일정을 적용하지 못했습니다. 저장한 일정을 다시 열어 비교하거나 재시도해 주세요.');}} />
      <Button label={preview.lastComparison ? "다시 비교하기" : "제안 닫기"} disabled={busy} action={closePreview} />
    </View>}
    {generating && <View style={styles.visit}>
      <Text accessibilityRole="header">추천을 생성하고 있어요</Text>
      <Button label="추천 생성 취소" action={cancelGeneration} />
      {manageConsent && <Button label="동의 관리" action={manageConsent} />}
    </View>}
    {!!status && <Text accessibilityRole="alert">{status}</Text>}
    {conflictID && !comparison ? <Button label="변경된 최신 일정 열기" disabled={busy}
      action={() => {run(compareLatest, '최신 일정을 불러오지 못했습니다.');}} /> : null}
    {comparison && <View style={styles.visit}>
      <Text accessibilityRole="header" style={styles.item}>일정 변경 사항을 비교하세요</Text>
      <Text>최신 일정 · 버전 {comparison.current.revision} · 예산 {cost(comparison.current.plan.budgetLimit)}</Text>
      {visitRows(comparison.current.plan)}
      <Text>검토했던 후보 · 기준 버전 {comparison.baseRevision} · 예산 {cost(comparison.previous.preview.budgetLimit)}</Text>
      {visitRows(comparison.previous.preview)}
      <Text>최신 일정과 예산을 유지합니다. 추가할 후보를 선택해 주세요.</Text>
      {comparison.previous.preview.stops.map((stop, index) => {
        const alreadyScheduled = comparison.current.plan.stops.some(existing => stopKey(existing) === stopKey(stop));
        const chosen = comparison.chosen.includes(index);
        const name = comparison.previous.preview.details?.[index]?.name ?? title(stop.place);
        return <Button key={stopKey(stop)} label={`후보 ${index + 1} ${name} ${alreadyScheduled ? '이미 포함' : chosen ? '선택 취소' : '선택'}`}
          disabled={busy || alreadyScheduled} action={() => setComparison(value => value && {...value,
            chosen: chosen ? value.chosen.filter(item => item !== index) : [...value.chosen, index]})} />;
      })}
      <Button label="선택한 후보 검토" disabled={busy || comparison.chosen.length === 0}
        action={() => {run(reviewSelection, '선택한 후보의 시간과 최신 예산을 함께 확인해 주세요.');}} />
      <Button label="비교 닫기" disabled={busy} action={() => {setComparison(undefined); setConflictID('');}} />
    </View>}
    <>
      <Text accessibilityRole="header" style={styles.item}>저장한 일정</Text>
      {!list.length && <Text>저장한 일정이 없습니다.</Text>}
      {list.map(entry => <Button key={entry.id} label={`${entry.name || title(entry.place)} · ${localDate(new Date(entry.start), entry.timezone ?? zone)}`}
        disabled={reviewBlocked} action={() => {run(() => open(entry.id), '일정을 다시 불러오지 못했습니다.');}} />)}
      <Button label="저장한 일정 다시 불러오기" disabled={busy || generating} action={() => {run(reload, '저장한 일정을 불러오지 못했습니다.');}} />
      {saved && !comparison && <View style={styles.visit}>
        <Text accessibilityRole="header" style={styles.item}>저장한 일정 · 버전 {saved.revision}</Text>
        {visitRows(saved.plan)}
        <Button label="새 일정으로 시작" disabled={reviewBlocked} action={() => {setSaved(undefined); setPreview(undefined); setConflictID('');}} />
      </View>}
      {place && catalogVersion ? <View style={styles.section}>
        <Text accessibilityRole="header" style={styles.item}>{place.name} 주변</Text>
        <DraftInputs draft={draft} change={update => updateInput(() => changeDraft(update))} zone={zone} disabled={busy || generating} />
        <Button label="날짜와 선호로 코스 추천" disabled={reviewBlocked || !/^[1-9]\d{0,8}$/.test(draft.budget)}
          action={() => {
            Keyboard.dismiss();
            try {generate(requestInputs(draft, zone));}
            catch {setStatus('선택한 날짜, 시간과 예산을 확인해 주세요.');}
          }} />
        <Button label="영업시간과 일정 검토" disabled={reviewBlocked}
          action={() => {run(() => propose(false, requestInputs(draft, zone)), '선택한 시간의 영업시간과 예산을 확인해 주세요.');}} />
      </View> : <Text>장소를 고르면 주변 일정을 제안받을 수 있습니다.</Text>}
    </>
  </View>;
}

const styles = StyleSheet.create({
  section: {paddingVertical: 10},
  heading: {fontSize: 21, fontWeight: '600', marginVertical: 10},
  item: {fontSize: 17, fontWeight: '600', marginVertical: 6},
  visit: {borderBottomWidth: 1, borderColor: '#cad8ce', paddingVertical: 10},
  row: {flexDirection: 'row', flexWrap: 'wrap', gap: 4},
  input: {borderBottomWidth: 1, borderColor: '#9aada0', padding: 10, marginVertical: 8},
  button: {backgroundColor: '#e5ede9', padding: 11, marginVertical: 4, alignItems: 'center', borderRadius: 5},
  disabled: {opacity: 0.5},
});
