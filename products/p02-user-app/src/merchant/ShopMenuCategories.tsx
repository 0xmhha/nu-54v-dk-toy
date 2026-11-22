import { Pressable, StyleSheet, Text, View } from 'react-native';
import type { CommerceCategory, CommercePrice } from '@nu54/protocol';

export function shopMenuCategories(rows: CommercePrice[]) {
  const byKey = new Map<string, CommerceCategory>();
  for (const {category} of rows) {
    if (category) byKey.set(category.key, category);
  }
  return [...byKey.values()].sort((a, b) =>
    a.sortOrder - b.sortOrder || (a.key < b.key ? -1 : 1),
  );
}

export function ShopMenuCategories({categories, selected, select, locale}: {
  categories: CommerceCategory[];
  selected: string;
  select: (key: string) => void;
  locale: 'ko' | 'en';
}) {
  if (!categories.length) return null;
  return <View style={styles.group}>
    <Text accessibilityRole="header">{locale === 'ko' ? '메뉴 카테고리' : 'Menu categories'}</Text>
    <View style={styles.options}>
      {[{key: '', label: locale === 'ko' ? '전체 메뉴' : 'All items'},
        ...categories.map(({key, labels}) => ({key, label: labels[locale]})),
      ].map(({key, label}) => <Pressable key={key} accessibilityRole="button"
        accessibilityLabel={label} accessibilityState={{selected: key === selected}}
        style={[styles.option, key === selected && styles.selected]} onPress={() => select(key)}>
        <Text style={key === selected && styles.selectedLabel}>{label}</Text>
      </Pressable>)}
    </View>
  </View>;
}

const styles = StyleSheet.create({
  group: {gap: 8},
  options: {flexDirection: 'row', flexWrap: 'wrap', gap: 8},
  option: {borderWidth: 1, borderColor: '#6b7280', borderRadius: 8,
    minHeight: 44, paddingHorizontal: 12, justifyContent: 'center'},
  selected: {backgroundColor: '#173e35', borderColor: '#173e35'},
  selectedLabel: {fontWeight: '600', color: '#fff'},
});
