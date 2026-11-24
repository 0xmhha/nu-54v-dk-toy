import React, {useMemo, useState} from 'react';
import {Text, TextInput, View, StyleSheet} from 'react-native';
import {Card, KitButton, Row, Section} from '../ui/kit.tsx';
import {useInnerBack, usePageTop} from '../ui/innerBack.ts';
import {color, font, inputPlaceholder, radius, type} from '../ui/theme.ts';
import type {Consent} from './consent.ts';
import {licenseText, notice, notices, type License, type Notice, type NoticeId} from './notices.ts';

type Language = 'ko' | 'en';
const order: NoticeId[] = ['terms', 'privacy', 'location', 'test-assets'];
const day = (at: number, ko: boolean) => new Date(at).toLocaleString(ko ? 'ko-KR' : 'en-US');
const blank = (ko: boolean) => ko ? '아직 정해지지 않음' : 'Not decided yet';

export function NoticePage({document, language}: {document: Notice; language: Language}) {
  const ko = language === 'ko';
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{document.title}</Text>
    <Card style={document.status === 'draft' ? styles.draft : undefined}>
      <Row label={ko ? '판' : 'Edition'} value={`${document.edition}${document.status === 'draft' ? (ko ? ' · 초안' : ' · draft') : ''}`} />
      <Row label={ko ? '시행일' : 'Effective'} value={document.effective ?? blank(ko)} />
      <Row label={ko ? '책임 주체' : 'Publisher'} value={notices.publisher.name ?? blank(ko)} />
      <Row label={ko ? '문의' : 'Contact'} value={notices.publisher.contact ?? blank(ko)} />
    </Card>
    {document.sections.map(section => <View key={section.heading} style={styles.words}>
      <Text accessibilityRole="header" style={type.section}>{section.heading}</Text>
      <Text selectable style={type.body}>{section.body}</Text>
    </View>)}
  </View>;
}

const pageSize = 40;
export function LicensePage({language, licenses, title, top}: {language: Language; licenses: License[]; title: string; top?: () => void}) {
  const ko = language === 'ko';
  const [search, setSearch] = useState(''), [shown, setShown] = useState(pageSize), [open, setOpen] = useState<License>();
  useInnerBack(!!open, () => setOpen(undefined));
  usePageTop(open?.name, top);
  const found = useMemo(() => {
    const words = search.trim().toLowerCase();
    return words ? licenses.filter(item => `${item.name} ${item.license}`.toLowerCase().includes(words)) : licenses;
  }, [licenses, search]);
  if (open) return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.section}>{`${open.name} ${open.version}`}</Text>
    <Text style={type.small}>{open.license}</Text>
    <Text selectable style={styles.license}>{licenseText(open)}</Text>
    <KitButton label={ko ? '목록으로' : 'Back to the list'} run={() => setOpen(undefined)} variant="secondary" />
  </View>;
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{title}</Text>
    <Text style={type.small}>{ko ? `앱에 들어간 패키지 ${licenses.length}개` : `${licenses.length} packages in this app`}</Text>
    <TextInput accessibilityLabel={ko ? '패키지 찾기' : 'Find a package'} placeholder={ko ? '패키지 찾기' : 'Find a package'} placeholderTextColor={inputPlaceholder}
      value={search} onChangeText={text => { setSearch(text); setShown(pageSize); }} style={styles.input} autoCapitalize="none" autoCorrect={false} maxLength={80} />
    <View style={styles.group}>{found.slice(0, shown).map(item => <KitButton key={item.name + item.version} variant="row" label={item.name}
      detail={`${item.version} · ${item.license}`} run={() => setOpen(item)} />)}</View>
    {found.length > shown && <KitButton label={ko ? '더 보기' : 'Show more'} run={() => setShown(shown + pageSize)} variant="secondary" />}
  </View>;
}

/** 법적 고지: the documents, the licences and what the owner agreed to. */
export function LegalHome({language, consents = [], setAlerts, busy, back, top}: {
  language: Language; consents?: Consent[]; setAlerts: (on: boolean) => void; busy: boolean; back: () => void; top?: () => void;
}) {
  const ko = language === 'ko';
  const [page, setPage] = useState<NoticeId | 'licenses' | 'firmware'>();
  const leave = () => setPage(undefined);
  useInnerBack(!!page, leave);
  usePageTop(page, top);
  if (page) {
    const document = page === 'licenses' || page === 'firmware' ? undefined : notice(page, language);
    return <View style={styles.page}>
      {document ? <NoticePage document={document} language={language} />
        : page === 'firmware' ? <LicensePage language={language} top={top} licenses={notices.firmwareLicenses ?? []} title={ko ? '보드 펌웨어 라이선스' : 'Board firmware licences'} />
        : <LicensePage language={language} top={top} licenses={notices.licenses} title={ko ? '오픈소스 라이선스' : 'Open-source licences'} />}
      <KitButton label={ko ? '법적 고지로 돌아가기' : 'Back to legal notices'} run={leave} variant="secondary" />
    </View>;
  }
  const documents = order.map(id => notice(id, language)).filter((item): item is Notice => !!item);
  const alerts = consents.find(item => item.id === 'alerts');
  return <View style={styles.page}>
    {(documents.some(item => item.status === 'draft') || !notices.publisher.name) && <Card style={styles.draft}>
      <Text style={type.label}>{ko ? '초안입니다' : 'Draft'}</Text>
      <Text style={type.body}>{ko ? '책임 주체와 시행일이 정해지기 전이라 외부에 배포하지 않는 시험용 앱입니다.' : 'The publisher and dates are not set, so this test app is not distributed publicly.'}</Text>
    </Card>}
    <Section title={ko ? '문서' : 'Documents'}>
      {documents.map(item => <KitButton key={item.id} variant="row" label={item.title} detail={`${ko ? '판' : 'Edition'} ${item.edition}${item.status === 'draft' ? (ko ? ' · 초안' : ' · draft') : ''}`}
        run={() => setPage(item.id)} />)}
      <KitButton variant="row" label={ko ? '오픈소스 라이선스' : 'Open-source licences'} detail={ko ? '앱에 들어간 패키지' : 'Packages in this app'} run={() => setPage('licenses')} />
      {!!notices.firmwareLicenses?.length && <KitButton variant="row" label={ko ? '보드 펌웨어 라이선스' : 'Board firmware licences'} run={() => setPage('firmware')} />}
    </Section>
    <Section title={ko ? '내 동의 기록' : 'What I agreed to'}>
      {consents.filter(item => item.id !== 'alerts').map(item => <View key={item.id} style={styles.consent}>
        <Text style={type.label}>{notice(item.id as NoticeId, language)?.title ?? item.id}</Text>
        <Text style={type.small}>{`${ko ? '판' : 'Edition'} ${item.edition} · ${day(item.at, ko)}`}</Text>
      </View>)}
      <KitButton variant="row" disabled={busy} label={ko ? '주문 준비 알림 (선택)' : 'Order-ready notices (optional)'}
        detail={!alerts ? (ko ? '아직 정하지 않았습니다 · 누르면 켭니다' : 'Not chosen yet · tap to turn on')
          : alerts.declined ? (ko ? '꺼짐 · 누르면 켭니다' : 'Off · tap to turn on') : (ko ? '켜짐 · 누르면 끕니다' : 'On · tap to turn off')}
        run={() => setAlerts(!alerts || !!alerts.declined)} />
    </Section>
    <Card>
      <Row label={ko ? '책임 주체' : 'Publisher'} value={notices.publisher.name ?? blank(ko)} />
      <Row label={ko ? '문의' : 'Contact'} value={notices.publisher.contact ?? blank(ko)} />
    </Card>
    <KitButton label={ko ? '설정으로 돌아가기' : 'Return to Settings'} run={back} variant="secondary" />
  </View>;
}

const styles = StyleSheet.create({
  page: {gap: 14}, words: {gap: 6},
  draft: {backgroundColor: color.amberBg, borderColor: color.amberBg},
  group: {borderRadius: radius.m, overflow: 'hidden', borderWidth: 1, borderColor: color.line, gap: 1, backgroundColor: color.line},
  consent: {backgroundColor: color.sheet, paddingHorizontal: 16, paddingVertical: 10, gap: 2},
  input: {fontFamily: font, fontSize: 15, borderWidth: 1, borderColor: '#8fa497', borderRadius: radius.m, padding: 12, minHeight: 50, color: color.ink, backgroundColor: color.sheet},
  license: {fontFamily: 'monospace', fontSize: 12, lineHeight: 18, color: color.ink, backgroundColor: color.sheet, borderRadius: 8, padding: 10},
});
