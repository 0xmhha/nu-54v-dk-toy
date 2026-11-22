import React, {useEffect, useRef, useState} from 'react';
import {AccessibilityInfo, Animated, Easing, Platform, StyleSheet, Text, View} from 'react-native';
import {boardFlows, boardPosition, boardSteps, boardStepsAhead, type BoardStepId, type BoardWait} from './boardSteps.ts';

// Drawing of the NU54DK v2 board as laid out on the silkscreen: SW4, SW3, SW2, SW1 from the top on
// the right edge and LED1-LED4 in one row beside SW2/SW1. The firmware drives all four LEDs
// together (status_led.c). Each step animates only the buttons or LEDs the owner must use or
// watch; with reduced motion the same highlight stays still.
const order = ['SW4', 'SW3', 'SW2', 'SW1'] as const;
type Name = typeof order[number];
type Art = {hold?: Name[]; tap?: Name[]; seq?: Name[]; press?: Name[]; later?: Name[]; alt?: Name[];
  led?: 'wait' | 'pin' | 'locate' | 'two'; device?: [string, string[]]; still?: boolean};
const art: Partial<Record<BoardStepId, Art>> = {
  window: {hold: ['SW4']}, recovery: {hold: ['SW3']}, hold: {hold: ['SW3']},
  approve: {tap: ['SW1'], alt: ['SW2'], led: 'wait'},
  pin: {seq: ['SW1', 'SW3'], alt: ['SW2'], led: 'pin'},
  update: {press: ['SW1', 'SW2'], later: ['SW3'], led: 'two'},
  locate: {led: 'locate'}, working: {still: true},
  display: {device: ['독립 표시 장치', ['금액', '받는 곳', '네트워크']]},
  recoveryproof: {device: ['독립 복구 장치', ['새 휴대전화 지문', '소유 세대']]},
};

/** One looping value per drawing: 0 rests, 1 is a pressed button or a lit LED at that moment. */
function useLoop(frames: [number, number][], enabled: boolean) {
  const value = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    value.setValue(0);
    if (!enabled || !frames.length) return;
    const loop = Animated.loop(Animated.sequence(frames.map(([to, ms]) =>
      Animated.timing(value, {toValue: to, duration: ms, easing: Easing.out(Easing.exp), useNativeDriver: true}))));
    loop.start();
    return () => loop.stop();
    // The frame list is static per step; restarting on its identity would reset the loop each render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, enabled, frames.length]);
  return value;
}

// Timings follow the firmware: long press 1000 ms (nu54_button.h), waiting blink 200/200,
// PIN start 300/200/300 ms, tap 80 ms and next digit 500 ms, locate 400/650 ms four times
// (status_led.c). The three PIN loops share one 4 s timeline so the presses and LEDs stay in step.
const holdFrames: [number, number][] = [[0, 200], [1, 100], [1, 1100], [0, 150], [0, 850]];
const tapFrames: [number, number][] = [[0, 700], [1, 80], [0, 120], [0, 700]];
const blinkFrames: [number, number][] = [[1, 1], [1, 199], [0, 1], [0, 199]];
const pinPress: [number, number][] = [[0, 1050], [1, 60], [0, 200], [0, 140], [1, 60], [0, 200], [0, 2290]];
const pinNext: [number, number][] = [[0, 1950], [1, 60], [0, 200], [0, 1790]];
const pinLed: [number, number][] = [[1, 1], [1, 299], [0, 1], [0, 199], [1, 1], [1, 299], [0, 1], [0, 299],
  [1, 1], [1, 79], [0, 1], [0, 319], [1, 1], [1, 79], [0, 1], [0, 419], [1, 1], [1, 499], [0, 1], [0, 1499]];
const locateFrames: [number, number][] = [[1, 1], [1, 399], [0, 1], [0, 649], [1, 1], [1, 399], [0, 1], [0, 649],
  [1, 1], [1, 399], [0, 1], [0, 649], [1, 1], [1, 399], [0, 1], [0, 2050]];

function useReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    let live = true;
    AccessibilityInfo.isReduceMotionEnabled().then(value => { if (live) setReduced(value); }).catch(() => {});
    const subscription = AccessibilityInfo.addEventListener('reduceMotionChanged', setReduced);
    return () => { live = false; subscription.remove(); };
  }, []);
  return reduced;
}

function BoardDrawing({step, t}: {step: BoardStepId; t: (ko: string) => string}) {
  const reduced = useReducedMotion();
  const a = art[step] ?? {};
  const hold = useLoop(holdFrames, !reduced && !!a.hold);
  const tap = useLoop(tapFrames, !reduced && !!a.tap);
  const first = useLoop(pinPress, !reduced && !!a.seq);
  const next = useLoop(pinNext, !reduced && !!a.seq);
  const led = useLoop(a.led === 'wait' ? blinkFrames : a.led === 'pin' ? pinLed : a.led === 'locate' ? locateFrames : [],
    !reduced && (a.led === 'wait' || a.led === 'pin' || a.led === 'locate'));

  if (a.device) {
    const [name, rows] = a.device;
    return <View style={styles.device} accessible accessibilityRole="image" accessibilityLabel={t(name) + ' · ' + t('그림')}>
      <View style={styles.deviceScreen}>{rows.map(row => <View key={row} style={styles.deviceRow}>
        <Text style={styles.deviceLabel}>{t(row)}</Text><View style={styles.deviceLine} /></View>)}</View>
      <Text style={styles.deviceName}>{t(name)}</Text>
    </View>;
  }
  const role = (name: Name) => a.hold?.includes(name) ? 'hold' : a.tap?.includes(name) ? 'tap' : a.press?.includes(name) ? 'press' :
    a.seq?.includes(name) ? 'seq' : a.later?.includes(name) ? 'later' : a.alt?.includes(name) ? 'alt' : '';
  const marked = order.filter(name => ['hold', 'tap', 'press', 'seq'].includes(role(name)));
  const label = t('보드 그림') + (marked.length ? ' · ' + t('강조한 버튼') + ' ' + marked.join('·') : a.still ? ' · ' + t('버튼을 누르지 않는 단계') : '');
  const pressScale = (value: Animated.Value) => value.interpolate({inputRange: [0, 1], outputRange: [1, 0.68]});
  const ledOn = a.led === 'two' ? [true, true, false, false] : [false, false, false, false];
  const ledAnimated = a.led === 'wait' || a.led === 'pin' || a.led === 'locate';
  return <View style={styles.board} accessible accessibilityRole="image" accessibilityLabel={label}>
    <View style={styles.module}><Text style={styles.moduleText}>NU-54V</Text></View>
    <View style={styles.leds}>{[0, 1, 2, 3].map(i => {
      const lit = ledAnimated ? (reduced ? 1 : led) : ledOn[i] ? 1 : 0;
      return <View key={i} style={styles.ledSlot}><View style={styles.ledOff} />
        <Animated.View style={[styles.ledOn, {opacity: lit}]} /></View>;
    })}<Text style={styles.ledText}>LED</Text></View>
    <View style={styles.buttons}>{order.map(name => {
      const r = role(name);
      const value = r === 'hold' ? hold : r === 'tap' ? tap : r === 'seq' ? (name === 'SW1' ? first : next) : undefined;
      const scale = r === 'press' ? 0.68 : value && !reduced ? pressScale(value) : 1;
      const strong = ['hold', 'tap', 'press', 'seq'].includes(r);
      return <View key={name} style={styles.buttonRow}>
        <View style={[styles.pad, r === 'hold' && styles.padHold]}>
          <Animated.View style={[styles.cap, strong && styles.capStrong, r === 'alt' && styles.capAlt,
            r === 'later' && styles.capLater, a.still && styles.capStill, {transform: [{scale}]}]} />
        </View>
        <Text style={[styles.buttonText, strong && styles.buttonTextStrong]}>{name}</Text>
      </View>;
    })}</View>
  </View>;
}

/** Shows which board step is running while the app waits for the screenless board. */
export function BoardStepPanel({wait, t}: {wait: BoardWait; t: (ko: string) => string}) {
  const step = boardSteps[wait.step];
  const flow = boardFlows[wait.flow];
  const position = boardPosition(wait);
  const ahead = boardStepsAhead(wait);
  // Android reads the live region below; iOS needs an explicit announcement when the step changes.
  useEffect(() => {
    if (Platform.OS === 'ios') AccessibilityInfo.announceForAccessibility(t(step.do));
    // Announce once per step; the caller recreates `t` on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wait.flow, wait.step]);
  return <View style={styles.panel} testID="board-step-panel">
    <View style={styles.head}>
      <Text style={styles.headText}>{t('하드월렛') + ' · ' + t(flow.label)}</Text>
      <Text style={styles.headCount}>{position ? `${position.index + 1} / ${position.total}` : t(step.label)}</Text>
    </View>
    <View style={styles.track} accessibilityRole="list">{flow.steps.map((id, i) => {
      const state = !position ? 'ended' : i < position.index ? 'past' : i === position.index ? 'now' : 'next';
      const said = {ended: '요청 종료', past: '완료', now: '현재', next: '다음'}[state];
      return <View key={id} style={styles.trackItem} accessible accessibilityLabel={`${i + 1}. ${t(boardSteps[id].label)}, ${t(said)}`}>
        <View style={[styles.bar, state === 'past' && styles.barPast, state === 'now' && styles.barNow,
          state === 'ended' && styles.barEnded]} />
        <Text style={[styles.trackText, state === 'now' && styles.trackTextNow]}>{`${i + 1}\n${t(boardSteps[id].label)}`}</Text>
      </View>;
    })}</View>
    <View style={styles.now}>
      <BoardDrawing step={wait.step} t={t} />
      <View style={styles.nowText}>
        <Text style={styles.caption}>{t(position ? '지금 할 일' : '결과')}</Text>
        <Text style={styles.doText} accessibilityLiveRegion="polite">{t(step.do)}</Text>
        {step.buttons.map(([button, how]) => <View key={button} style={styles.key}>
          <Text style={styles.keyName}>{button}</Text><Text style={styles.keyHow}>{t(how)}</Text></View>)}
        {!!step.led && <Text style={styles.small}>{t('표시등') + ' · ' + t(step.led)}</Text>}
      </View>
    </View>
    <Text style={styles.small}>{t(step.app)}</Text>
    {!!step.pending && <Text style={styles.pending}>{t(step.pending)}</Text>}
    {!!ahead.length && <View style={styles.ahead}>
      <Text style={styles.caption}>{t('이어서 할 일')}</Text>
      {ahead.map(id => <Text key={id} style={styles.small}>{t(boardSteps[id].label) + ' · ' +
        boardSteps[id].buttons.map(([button, how]) => button + ' ' + t(how)).join(', ')}</Text>)}
    </View>}
  </View>;
}

const ink = '#f4f8ed', mint = '#d4efa7', soft = '#cfe0cf', green = '#174b38';
const styles = StyleSheet.create({
  panel: {backgroundColor: green, borderRadius: 14, padding: 14, gap: 10},
  head: {flexDirection: 'row', justifyContent: 'space-between', gap: 10},
  headText: {color: mint, fontSize: 13, flexShrink: 1}, headCount: {color: '#fff', fontSize: 13, fontWeight: '700'},
  track: {flexDirection: 'row', gap: 4}, trackItem: {flex: 1, minWidth: 0},
  bar: {height: 3, borderRadius: 2, backgroundColor: '#8fae99', opacity: 0.55, marginBottom: 6},
  barPast: {backgroundColor: '#9fc98c', opacity: 1}, barNow: {backgroundColor: mint, opacity: 1, height: 5},
  barEnded: {backgroundColor: '#e1a3a3', opacity: 1},
  trackText: {color: '#b8cfba', fontSize: 11, lineHeight: 14}, trackTextNow: {color: '#fff', fontWeight: '700'},
  now: {flexDirection: 'row', gap: 12, alignItems: 'flex-start'}, nowText: {flex: 1, gap: 6},
  caption: {color: mint, fontSize: 12, fontWeight: '600'}, doText: {color: '#fff', fontSize: 17, lineHeight: 24},
  key: {flexDirection: 'row', alignItems: 'center', gap: 8},
  keyName: {backgroundColor: mint, color: '#123a2b', fontWeight: '700', fontSize: 13, borderRadius: 7, paddingHorizontal: 8,
    paddingVertical: 5, overflow: 'hidden'},
  keyHow: {color: ink, fontSize: 14, flexShrink: 1},
  small: {color: soft, fontSize: 13, lineHeight: 19}, pending: {color: '#5a3b05', backgroundColor: '#f6e3bf', borderRadius: 8, padding: 8, fontSize: 13},
  ahead: {borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: '#8fae99', paddingTop: 8, gap: 4},
  board: {width: 104, borderRadius: 9, backgroundColor: '#0e2a1f', borderWidth: 1.5, borderColor: '#4f7a63', padding: 8, gap: 8},
  module: {width: 46, height: 30, borderRadius: 4, backgroundColor: '#9fb0a6', alignItems: 'center', justifyContent: 'center'},
  moduleText: {color: '#0e2a1f', fontSize: 8, fontWeight: '700'},
  leds: {position: 'absolute', left: 8, top: 112, flexDirection: 'row', gap: 4, alignItems: 'center'},
  ledSlot: {width: 8, height: 5}, ledOff: {position: "absolute", top: 0, left: 0, right: 0, bottom: 0, backgroundColor: '#4d7563', borderRadius: 1.5},
  ledOn: {position: "absolute", top: 0, left: 0, right: 0, bottom: 0, backgroundColor: mint, borderRadius: 1.5}, ledText: {color: '#9fb7a6', fontSize: 8, marginLeft: 2},
  buttons: {alignSelf: 'flex-end', gap: 6},
  buttonRow: {flexDirection: 'row', alignItems: 'center', gap: 5},
  pad: {width: 22, height: 22, borderRadius: 4, backgroundColor: '#1d3d30', borderWidth: 1, borderColor: '#5f8a72', alignItems: 'center', justifyContent: 'center'},
  padHold: {borderColor: mint, borderWidth: 2},
  cap: {width: 13, height: 13, borderRadius: 7, backgroundColor: '#5f8a72'},
  capStrong: {backgroundColor: mint}, capAlt: {backgroundColor: 'transparent', borderWidth: 1.5, borderColor: '#f3d2d2'},
  capLater: {backgroundColor: 'transparent', borderWidth: 1.5, borderColor: mint, borderStyle: 'dashed'}, capStill: {backgroundColor: '#3c5c4c'},
  buttonText: {color: soft, fontSize: 10, fontWeight: '700'}, buttonTextStrong: {color: '#fff'},
  device: {width: 104, borderRadius: 9, backgroundColor: '#e9f3df', padding: 6, gap: 6, alignItems: 'center'},
  deviceScreen: {alignSelf: 'stretch', backgroundColor: '#fffefa', borderRadius: 4, borderWidth: 2, borderColor: green, padding: 6, gap: 6},
  deviceRow: {gap: 3}, deviceLabel: {color: '#4f655a', fontSize: 9, fontWeight: '600'},
  deviceLine: {height: 3, borderRadius: 2, backgroundColor: green, width: '80%'},
  deviceName: {color: '#123a2b', fontSize: 10, fontWeight: '800'},
});
