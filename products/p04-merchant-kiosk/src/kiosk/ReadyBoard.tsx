import React, {useEffect, useRef, useState} from 'react';
import {StyleSheet, Text, View} from 'react-native';
import type {CommerceReadyBoard} from '@nu54/protocol';
import {NewNumber} from './Motion';

/** How often the board asks again while it is on screen. */
export const boardPollMs = 5_000;

/**
 * Reads the ready board every few seconds while `on`. Keeps the last board when a read fails and says so,
 * and remembers which ready numbers it has already shown, so only new ones are marked.
 */
export function useReadyBoard(read: (() => Promise<CommerceReadyBoard>) | undefined, on: boolean) {
  const [board, setBoard] = useState<CommerceReadyBoard>();
  const [lost, setLost] = useState(false);
  const [fresh, setFresh] = useState<number[]>([]);
  const seen = useRef<Set<number> | undefined>(undefined);
  useEffect(() => {
    if (!on || !read) return;
    let live = true;
    const tick = () => read().then(next => {
      if (!live) return;
      const known = seen.current;
      // The first read marks nothing: those numbers were ready before the board came up.
      setFresh(known ? next.ready.filter(n => !known.has(n)) : []);
      seen.current = new Set(next.ready);
      setBoard(next);
      setLost(false);
    }, () => live && setLost(true));
    tick();
    const timer = setInterval(tick, boardPollMs);
    return () => { live = false; clearInterval(timer); };
  }, [read, on]);
  return {board, lost, fresh};
}

const clock = (iso: string) => {
  const at = new Date(iso);
  return [at.getHours(), at.getMinutes(), at.getSeconds()].map(n => String(n).padStart(2, '0')).join(':');
};

/** The band on the customer's screens: the four newest ready numbers. */
export function ReadyBand({board, ko}: {board?: CommerceReadyBoard; ko: boolean}) {
  if (!board?.ready.length) return null;
  const shown = board.ready.slice(0, 4);
  return (
    <View accessible accessibilityRole="summary" accessibilityLabel={`${ko ? '준비된 번호' : 'Ready'} ${shown.join(', ')}`} style={s.band}>
      <Text style={s.bandLabel}>{ko ? '준비됨' : 'Ready'}</Text>
      {shown.map(n => <Text key={n} style={s.bandNumber}>{n}</Text>)}
    </View>
  );
}

/** The whole board, while nobody is ordering: ready numbers large (four), the rest small; then being made and received. */
export function ReadyBoardView({board, lost, fresh, ko}: {board?: CommerceReadyBoard; lost: boolean; fresh: number[]; ko: boolean}) {
  if (!board) return lost ? <Text style={s.small}>{ko ? '준비 완료 안내판을 읽지 못했습니다. 다시 확인하고 있습니다.' : 'Could not read the ready board. Trying again.'}</Text> : null;
  const large = board.ready.slice(0, 4), rest = board.ready.slice(4);
  return (
    <View style={s.board}>
      {/* Read out once when a number becomes ready; the board itself is not re-read every few seconds. */}
      <Text accessibilityLiveRegion="polite" style={s.hidden}>{fresh.length ? `${ko ? '새로 준비된 번호' : 'Now ready'} ${fresh.join(', ')}` : ''}</Text>
      <View style={s.columns}>
      <View accessibilityRole="summary" accessibilityLabel={ko ? '준비 완료' : 'Ready'} style={s.ready}>
        <Text accessibilityRole="header" style={s.readyTitle}>{ko ? '준비되었습니다' : 'Ready'}</Text>
        {large.length ? (
          <View style={s.readyGrid}>
            {large.map(n => (
              <NewNumber key={n} fresh={fresh.includes(n)} style={s.readyTile}>
                <Text accessibilityLabel={`${ko ? '준비된 번호' : 'Ready'} ${n}`} style={s.readyNumber}>{n}</Text>
              </NewNumber>
            ))}
          </View>
        ) : <Text style={s.empty}>{ko ? '아직 준비된 번호가 없습니다' : 'Nothing ready yet'}</Text>}
        {!!rest.length && <Text style={s.restLine}>{`${ko ? '그 밖에 준비된 번호' : 'Also ready'} ${rest.join(' · ')}`}</Text>}
      </View>
      <View style={s.side}>
        <Text accessibilityRole="header" style={s.sideTitle}>{ko ? '만드는 중' : 'Being made'}</Text>
        <Text style={s.sideNumbers}>{board.making.length ? board.making.join('  ') : '–'}</Text>
        <Text accessibilityRole="header" style={s.sideTitle}>{ko ? '접수' : 'Received'}</Text>
        <Text style={s.sideNumbers}>{board.received.length ? board.received.join('  ') : '–'}</Text>
      </View>
      </View>
      <Text style={[s.small, lost && s.lost]}>
        {lost
          ? `${ko ? '연결이 끊겼습니다 · 마지막 확인' : 'Connection lost · last checked'} ${clock(board.checkedAt)}`
          : `${ko ? '번호가 보이면 카운터로 오세요 · 마지막 확인' : 'Come to the counter when you see your number · checked'} ${clock(board.checkedAt)}`}
      </Text>
    </View>
  );
}

const ink = '#182d25', deep = '#123a2b';
const s = StyleSheet.create({
  band: {flexDirection: 'row', alignItems: 'center', gap: 10, backgroundColor: '#d4efa7', borderRadius: 14, paddingVertical: 8, paddingHorizontal: 14, alignSelf: 'flex-start'},
  bandLabel: {fontSize: 16, fontWeight: '700', color: deep},
  bandNumber: {fontSize: 24, fontWeight: '800', color: deep},
  // Side by side when there is room, as in the approved board; one column on a narrow screen.
  board: {gap: 16},
  columns: {flexDirection: 'row', flexWrap: 'wrap', gap: 16},
  hidden: {position: 'absolute', width: 1, height: 1, opacity: 0},
  ready: {backgroundColor: '#d4efa7', borderRadius: 20, padding: 24, gap: 14, flexGrow: 1.4, flexBasis: 420},
  readyTitle: {fontSize: 34, fontWeight: '800', color: deep},
  readyGrid: {flexDirection: 'row', flexWrap: 'wrap', gap: 14},
  readyTile: {minWidth: 180, height: 130, alignItems: 'center', justifyContent: 'center', borderRadius: 20},
  readyNumber: {fontSize: 104, lineHeight: 120, fontWeight: '800', color: deep, letterSpacing: -4},
  restLine: {fontSize: 22, fontWeight: '700', color: deep},
  empty: {fontSize: 22, color: deep},
  side: {gap: 6, flexGrow: 1, flexBasis: 260, paddingVertical: 12},
  sideTitle: {fontSize: 22, fontWeight: '800', color: ink},
  sideNumbers: {fontSize: 40, fontWeight: '800', color: '#3d5148'},
  small: {fontSize: 16, color: '#4f655a'},
  lost: {color: '#8a3b12', fontWeight: '700'},
});
