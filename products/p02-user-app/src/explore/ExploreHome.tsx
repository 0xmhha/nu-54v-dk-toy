import React from 'react';
import {ScrollView, StyleSheet, Text} from 'react-native';
import {KitButton, Section} from '../ui/kit.tsx';
import {color, font} from '../ui/theme.ts';

export type ExploreEntry = {label: string; detail: string; run: () => void};

/** First page of the 둘러보기 tab (W13): services the wallet opens, grouped by what the user does there. */
export function ExploreHome({places, apps}: {places: ExploreEntry[]; apps: ExploreEntry[]}) {
  const rows = (entries: ExploreEntry[]) => entries.map(entry =>
    <KitButton key={entry.label} label={entry.label} detail={entry.detail} run={entry.run} variant="row" />);
  return <ScrollView contentContainerStyle={styles.content} testID="explore-home">
    <Text accessibilityRole="header" style={styles.title}>둘러보기</Text>
    <Section title="장소와 매장">{rows(places)}</Section>
    <Section title="앱과 서비스">{rows(apps)}</Section>
  </ScrollView>;
}

const styles = StyleSheet.create({
  content: {gap: 16, paddingBottom: 24},
  title: {fontFamily: font, fontSize: 28, fontWeight: '800', letterSpacing: -0.8, color: color.ink},
});
