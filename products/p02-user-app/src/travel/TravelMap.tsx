import {useMemo, useRef, useState} from 'react';
import {Image, PanResponder, Pressable, StyleSheet, Text, View} from 'react-native';
import {geographicPoint, type MapInfo, type Place} from './catalog.ts';

const tileSize = 256;
const wrap = (x: number, width: number) => ((x % width) + width) % width;

export function TravelMap({origin, info, places, select, fallback, disabled}: {
  origin: string; info: MapInfo; places: Place[]; select: (place: Place) => void;
  fallback: () => void; disabled: boolean;
}) {
  const markers = places.flatMap(place => {
    const point = geographicPoint(place);
    return point ? [{place, ...point}] : [];
  });
  const first = markers[0];
  const [center, setCenter] = useState<{x: number; y: number}>(first ?? {x: 0.5, y: 0.5});
  const [zoom, setZoom] = useState(Math.min(info.maxZoom, Math.max(info.minZoom, 12)));
  const [failed, setFailed] = useState(false);
  const [layout, setLayout] = useState({width: 0, height: 0});
  const start = useRef(center);
  const tileCount = 2 ** zoom;
  const world = tileSize * tileCount;
  const pan = useMemo(() => PanResponder.create({
    onMoveShouldSetPanResponder: (_, state) => Math.abs(state.dx) + Math.abs(state.dy) > 8,
    onPanResponderGrant: () => {start.current = center;},
    onPanResponderMove: (_, {dx, dy}) => {
      setCenter({x: wrap(start.current.x - dx / world, 1), y: Math.max(0, Math.min(1, start.current.y - dy / world))});
    },
  }), [center, world]);
  const left = center.x * world - layout.width / 2;
  const top = center.y * world - layout.height / 2;
  const tiles = [];
  if (layout.width && layout.height) {
    for (let row = Math.max(0, Math.floor(top / tileSize)); row < tileCount && row * tileSize < top + layout.height; row++) {
      for (let col = Math.floor(left / tileSize); col * tileSize < left + layout.width; col++) {
        tiles.push(<Image key={`${zoom}/${col}/${row}`}
          style={[styles.tile, {left: col * tileSize - left, top: row * tileSize - top}]}
          source={{uri: `${origin}/v1/map/${zoom}/${wrap(col, tileCount)}/${row}.png`}}
          onError={() => setFailed(true)} />);
      }
    }
  }
  const equatorialMetersPerPixel = 40075016.686 / world;
  const latitudeScale = Math.cos(Math.atan(Math.sinh(Math.PI * (1 - 2 * center.y))));
  const scale = Math.max(1, Math.round(equatorialMetersPerPixel * latitudeScale * 64));
  if (!first || failed) {
    return <View>
      <Text>{failed ? '지도 자료를 불러오지 못했습니다.' : '공개 지도 좌표가 없습니다.'}</Text>
      <Pressable accessibilityRole="button" accessibilityLabel="목록 보기" style={styles.control} onPress={fallback}><Text>목록 보기</Text></Pressable>
    </View>;
  }
  return <View>
    <View style={styles.map} {...pan.panHandlers}
      onLayout={event => setLayout(event.nativeEvent.layout)} accessibilityLabel="공개 지역 지도">
      {tiles}
      {markers.map(marker => {
        const dx = wrap(marker.x * world - center.x * world + world / 2, world) - world / 2;
        const x = layout.width / 2 + dx;
        const y = marker.y * world - top;
        if (x < 0 || x > layout.width || y < 0 || y > layout.height) return null;
        return <Pressable key={marker.place.id} style={[styles.marker, {left: x - 13, top: y - 13}]} accessibilityRole="button"
          accessibilityLabel={marker.place.name} disabled={disabled} onPress={() => select(marker.place)}><Text>●</Text></Pressable>;
      })}
      <View pointerEvents="none" style={styles.scale}><Text>{scale >= 1000 ? `${Math.round(scale / 1000)} km` : `${scale} m`}</Text></View>
    </View>
    <View style={styles.row}>
      <Pressable style={styles.control} accessibilityRole="button" accessibilityLabel="지도 축소" disabled={zoom === info.minZoom} onPress={() => setZoom(zoom - 1)}><Text>−</Text></Pressable>
      <Pressable style={styles.control} accessibilityRole="button" accessibilityLabel="지도 확대" disabled={zoom === info.maxZoom} onPress={() => setZoom(zoom + 1)}><Text>+</Text></Pressable>
      <Pressable style={styles.control} accessibilityRole="button" onPress={() => setCenter(first)}><Text>검색 위치로</Text></Pressable>
    </View>
    <Text>{info.attribution}</Text>
  </View>;
}

const styles = StyleSheet.create({
  map: {height: 230, borderColor: '#a3aba8', borderWidth: 1, overflow: 'hidden', backgroundColor: '#e6eceb'},
  tile: {position: 'absolute', width: tileSize, height: tileSize},
  marker: {position: 'absolute', width: 26, height: 26, alignItems: 'center', justifyContent: 'center', backgroundColor: '#fffdf8', borderRadius: 13},
  scale: {position: 'absolute', right: 10, bottom: 8, width: 64, borderBottomWidth: 2, alignItems: 'center', backgroundColor: '#fffdf8'},
  row: {flexDirection: 'row', gap: 8},
  control: {backgroundColor: '#e5ede9', padding: 12, marginVertical: 5, borderRadius: 5},
});
