import { Pressable, StyleSheet, Text, View } from 'react-native';
import type { CommerceCategory, CommercePrice } from '@nu54/protocol';

export function menuCategories(rows: CommercePrice[]): CommerceCategory[] {
  const categories = new Map<string, CommerceCategory>();
  for (const { category } of rows) {
    if (category) categories.set(category.key, category);
  }
  return [...categories.values()].sort((a, b) =>
    a.sortOrder - b.sortOrder || (a.key < b.key ? -1 : 1),
  );
}

export function MenuCategorySelector({categories, selected, select, locale}: {
  categories: CommerceCategory[];
  selected: string;
  select: (key: string) => void;
  locale: 'ko' | 'en';
}) {
  if (!categories.length) return null;
  const ko = locale === 'ko';
  const options = [{key: '', label: ko ? '전체 메뉴' : 'All items'},
    ...categories.map(({ key, labels }) => ({key, label: labels[locale]}))];
  return <View style={styles.categories}>
    <Text accessibilityRole="header">{ko ? '메뉴 카테고리' : 'Menu categories'}</Text>
    <View style={styles.options}>
      {options.map(({ key, label }) => <Pressable key={key} accessibilityRole="button"
        accessibilityLabel={label} accessibilityState={{ selected: key === selected }}
        style={[styles.option, key === selected && styles.selected]}
        onPress={() => select(key)}>
        <Text style={key === selected && styles.selectedLabel}>{label}</Text>
      </Pressable>)}
    </View>
  </View>;
}

const styles = StyleSheet.create({
  categories: { gap: 8 },
  options: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  option: { borderWidth: 1, borderColor: '#6b7280', borderRadius: 8,
    justifyContent: 'center', minHeight: 44, paddingHorizontal: 12 },
  selected: { backgroundColor: '#173e35', borderColor: '#173e35' },
  selectedLabel: { color: '#fff', fontWeight: '600' },
});
