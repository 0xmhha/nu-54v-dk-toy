import { useEffect, useSyncExternalStore } from 'react';
import { Image, StyleSheet, View, type DimensionValue } from 'react-native';
import type { CommercePhotoCache } from '@nu54/protocol';

/** A product's photo once it has arrived; a quiet box of the same size until then, so the menu does not jump. */
export function MenuPhoto({ cache, name, label, height, width = '100%' }: {
  cache: CommercePhotoCache;
  name: string;
  label: string;
  height: number;
  width?: DimensionValue;
}) {
  const data = useSyncExternalStore(listener => cache.subscribe(listener), () => cache.get(name));
  useEffect(() => cache.want(name), [cache, name]);
  return data
    ? <Image accessibilityLabel={label} source={{ uri: `data:image/jpeg;base64,${data}` }} resizeMode="cover"
        style={[styles.photo, { height, width }]} />
    : <View accessibilityElementsHidden importantForAccessibility="no-hide-descendants" style={[styles.photo, styles.waiting, { height, width }]} />;
}

const styles = StyleSheet.create({
  photo: { borderRadius: 12, overflow: 'hidden' },
  waiting: { backgroundColor: '#e3e9df' },
});
