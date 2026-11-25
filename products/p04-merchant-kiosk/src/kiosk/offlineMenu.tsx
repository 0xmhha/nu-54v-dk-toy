import { useState } from 'react';
import { menuCategories, MenuCategorySelector } from './menuCategories';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import {
  appendCommerceCatalogPage,
  assertCommerceCatalogPage,
  commerceAmount,
  commerceCatalogLimit,
  commerceMenuLabels,
  readShopIdentity,
  utf8Encode,
  type CommercePrice,
  type CommercePaymentProfile,
  type ShopIdentity,
} from '@nu54/protocol';

export const menuCacheJournal = 'terminal-public-menu-v1';
const byteLimit = 128_000;
const scopePattern = /^[a-f0-9]{64}$/;
export type CachedMenu = {
  scope: string;
  modeRevision: number;
  shop: ShopIdentity;
  checkedAt: number;
  rows: CommercePrice[];
};

export function networkUnavailable(error: unknown) {
  return !!error && typeof error === 'object' &&
    'code' in error && error.code === 'COMMERCE_NETWORK_OFFLINE';
}

function projectedRow(row: CommercePrice): CommercePrice {
  const { version, expiresAt, sku, optionKey, label, asset, unitPrice, available, skuAvailable, metadata, category } = row;
  return {
    version, expiresAt, sku, optionKey, label, asset, unitPrice, available, skuAvailable,
    ...(category && { category: { key: category.key, sortOrder: category.sortOrder, labels: {
      ko: category.labels.ko, en: category.labels.en,
    } } }),
    ...(metadata && {metadata: {
      labels: metadata.labels,
      optionLabels: metadata.optionLabels,
      optionGroupLabels: metadata.optionGroupLabels,
      allergens: metadata.allergens,
      crossContact: metadata.crossContact,
      crossContactAllergens: metadata.crossContactAllergens,
      crossContactSource: metadata.crossContactSource,
      optionRequired: metadata.optionRequired,
    }}),
  };
}

export function readCachedMenu(raw: string, scope: string, branch: string, now: number): CachedMenu | undefined {
  if (!raw || utf8Encode(raw).length > byteLimit || !scopePattern.test(scope)) return undefined;
  try {
    const value = JSON.parse(raw);
    if (!value || value.scope !== scope ||
        !Number.isSafeInteger(value.modeRevision) || value.modeRevision < 1 ||
        !Number.isSafeInteger(value.checkedAt) || value.checkedAt < 1 || value.checkedAt > now)
      return undefined;
    const candidate:unknown=value.rows;
    assertCommerceCatalogPage(candidate, value.checkedAt, commerceCatalogLimit);
    if (candidate.some(row=>!row.label.trim() || /[\p{Cc}\u202a-\u202e\u2066-\u2069]/u.test(row.label)))
      return undefined;
    const rows: CommercePrice[] = [];
    appendCommerceCatalogPage(rows, candidate.map(projectedRow));
    return {scope, shop: readShopIdentity(value.shop, branch), modeRevision: value.modeRevision, checkedAt: value.checkedAt, rows};
  } catch {
    return undefined;
  }
}

export function encodeCachedMenu(value: CachedMenu): string | undefined {
  const raw=JSON.stringify({...value, rows: value.rows.map(projectedRow)});
  return readCachedMenu(raw, value.scope, value.shop.branch, value.checkedAt) ? raw : undefined;
}

export function OfflineMenu({cached, busy, retry, policy}: {cached: CachedMenu; busy: boolean; retry:()=>void; policy?: CommercePaymentProfile}) {
  const [locale, setLocale]=useState<'ko'|'en'>('ko');
  const ko=locale==='ko';
  const [category, setCategory] = useState('');
  const categories = menuCategories(cached.rows);
  const selected = categories.some(({ key }) => key === category) ? category : '';
  const localTime=new Date(cached.checkedAt*1000).toLocaleTimeString(ko?'ko-KR':'en-US', {
    hour:'2-digit', minute:'2-digit',
  });
  return <View style={styles.menu}>
    <Text accessibilityRole="header" style={styles.title}>
      {ko?'최근 메뉴를 보고 있습니다':'Viewing the last synced menu'}
    </Text>
    <Text accessibilityRole="header">{cached.shop.branchLabel}</Text>
    <Text>{ko?'매장 서버 연결이 돌아오면 현재 가격과 재고를 확인할 수 있습니다.':
      'Reconnect to the store to check current prices and stock.'}</Text>
    <Text>{ko?'마지막 조회 · ':'Last checked · '}{localTime}</Text>
    <Text>{ko?'새 견적 발급 · 일시 중지':'New quotes paused'}</Text>
    <Pressable accessibilityRole="button" accessibilityLabel={ko?'메뉴 언어 English':'Menu language 한국어'}
      style={styles.button} onPress={()=>setLocale(ko?'en':'ko')}>
      <Text>{ko?'English menu':'한국어 메뉴'}</Text>
    </Pressable>
    <Pressable accessibilityRole="button" accessibilityLabel={ko?'매장 연결 다시 확인':'Check store connection'}
      accessibilityState={{disabled:busy}} style={styles.button} disabled={busy} onPress={retry}>
      <Text>{ko?'매장 연결 다시 확인':'Check store connection'}</Text>
    </Pressable>
    {!cached.rows.length && <Text>{ko?'표시할 메뉴가 없습니다.':'There are no items to show.'}</Text>}
    <MenuCategorySelector categories={categories} selected={selected} locale={locale} select={setCategory} />
    {cached.rows.map(row=>{
      if (selected && row.category?.key !== selected) return null;
      const label=commerceMenuLabels(row,locale);
      const price=commerceAmount(row.unitPrice,row.asset,policy,locale);
      return <Text key={row.sku + '/' + row.optionKey}>
        {label.product} · {label.option} · {ko ? price : price.replace('기준 단위','base units')}
        {row.available===0 ? (ko?' · 품절':' · Sold out') : ''}
      </Text>;
    })}
  </View>;
}

const styles=StyleSheet.create({
  menu:{gap:12},
  title:{fontSize:22,fontWeight:'600'},
  button:{minHeight:45,justifyContent:'center'},
});
