import React, {useEffect, useRef, useState} from 'react';
import {AccessibilityInfo, Animated, Easing, StyleSheet, Text, View, type StyleProp, type ViewStyle} from 'react-native';

// The five approved motions (docs: traveller-services.md 9장, canvas "모션: 상태가 바뀌는 다섯 곳").
// Each plays once when its state changes, never loops (screen readers and the e2e driver read an idle screen),
// and stands still when the system asks for less motion. Nothing waits for a motion to finish.
// The same file is in products/p02-user-app/src/merchant/Motion.tsx; change both together.

/** True when Android's "remove animations" (or iOS "reduce motion") is on. */
export function useReducedMotion() {
  const [reduced, setReduced] = useState(true); // still until known, so the first frame never jumps
  useEffect(() => {
    let live = true;
    AccessibilityInfo.isReduceMotionEnabled().then(value => live && setReduced(value), () => live && setReduced(true));
    const listener = AccessibilityInfo.addEventListener('reduceMotionChanged', value => setReduced(value));
    return () => { live = false; listener.remove(); };
  }, []);
  return reduced;
}

const outQuint = Easing.bezier(0.22, 1, 0.36, 1);
const overshoot = Easing.bezier(0.34, 1.56, 0.64, 1);

/** 1. The basket count grows past its size once and settles (280ms) whenever the count changes. */
export function Pop({value, children, style}: {value: number; children: React.ReactNode; style?: StyleProp<ViewStyle>}) {
  const reduced = useReducedMotion();
  const scale = useRef(new Animated.Value(1)).current;
  const first = useRef(true);
  useEffect(() => {
    if (first.current) { first.current = false; return; }
    if (reduced) return;
    scale.setValue(0.6);
    Animated.timing(scale, {toValue: 1, duration: 280, easing: overshoot, useNativeDriver: true}).start();
  }, [value, reduced, scale]);
  return <Animated.View style={[style, {transform: [{scale}]}]}>{children}</Animated.View>;
}

/** 1. A new basket line rises 12px into place as it appears (200ms). */
export function SlideIn({children, style}: {children: React.ReactNode; style?: StyleProp<ViewStyle>}) {
  const reduced = useReducedMotion();
  const shown = useRef(new Animated.Value(1)).current;
  const started = useRef(false);
  useEffect(() => {
    if (reduced || started.current) return;
    started.current = true;
    shown.setValue(0);
    Animated.timing(shown, {toValue: 1, duration: 200, easing: outQuint, useNativeDriver: true}).start();
  }, [reduced, shown]);
  const translateY = shown.interpolate({inputRange: [0, 1], outputRange: [12, 0]});
  return <Animated.View style={[style, {opacity: shown, transform: [{translateY}]}]}>{children}</Animated.View>;
}

/** 2. The order number rises inside its own box (420ms); its opacity never changes, so it is never half-read. */
export function MaskRise({children, height, style}: {children: React.ReactNode; height: number; style?: StyleProp<ViewStyle>}) {
  const reduced = useReducedMotion();
  const rise = useRef(new Animated.Value(0)).current;
  const started = useRef(false);
  useEffect(() => {
    if (reduced || started.current) return;
    started.current = true;
    rise.setValue(height);
    Animated.timing(rise, {toValue: 0, duration: 420, easing: outQuint, useNativeDriver: true}).start();
  }, [reduced, rise, height]);
  return <View style={[s.mask, style]}><Animated.View style={{transform: [{translateY: rise}]}}>{children}</Animated.View></View>;
}

/** 5. Paid: the mark grows in (360ms) and one ring spreads and fades (500ms). No particles. */
export function PaidMark({size = 96, label}: {size?: number; label: string}) {
  const reduced = useReducedMotion();
  const mark = useRef(new Animated.Value(1)).current;
  const ring = useRef(new Animated.Value(1)).current;
  const started = useRef(false);
  useEffect(() => {
    if (reduced || started.current) return;
    started.current = true;
    mark.setValue(0);
    ring.setValue(0);
    Animated.parallel([
      Animated.timing(mark, {toValue: 1, duration: 360, easing: overshoot, useNativeDriver: true}),
      Animated.timing(ring, {toValue: 1, duration: 500, easing: outQuint, useNativeDriver: true}),
    ]).start();
  }, [reduced, mark, ring]);
  const round = {width: size, height: size, borderRadius: size / 2};
  return (
    <View accessible accessibilityRole="image" accessibilityLabel={label} style={round}>
      {!reduced && <Animated.View pointerEvents="none" style={[StyleSheet.absoluteFill, round, s.ring, {
        opacity: ring.interpolate({inputRange: [0, 1], outputRange: [0.6, 0]}),
        transform: [{scale: ring.interpolate({inputRange: [0, 1], outputRange: [0.8, 1.35]})}],
      }]} />}
      <Animated.View style={[round, s.mark, {transform: [{scale: mark.interpolate({inputRange: [0, 1], outputRange: [0.5, 1]})}]}]}>
        <Text style={[s.check, {fontSize: size * 0.5, lineHeight: size * 0.62}]}>✓</Text>
      </Animated.View>
    </View>
  );
}

/** 4. Payment steps: when a step finishes, the line to the next one draws (300ms) and the next dot fills. */
export function PaymentSteps({steps, current, label}: {steps: string[]; current: number; label: string}) {
  const reduced = useReducedMotion();
  const drawn = useRef(new Animated.Value(1)).current;
  const last = useRef(current);
  useEffect(() => {
    if (last.current === current) return;
    last.current = current;
    if (reduced) return;
    drawn.setValue(0);
    Animated.timing(drawn, {toValue: 1, duration: 300, easing: Easing.inOut(Easing.ease), useNativeDriver: true}).start();
  }, [current, reduced, drawn]);
  return (
    <View accessibilityRole="list" accessibilityLabel={label} style={s.steps}>
      {steps.map((name, index) => {
        const done = index < current, now = index === current;
        return (
          <React.Fragment key={name}>
            {index > 0 && (
              <View style={s.bar}>
                {index <= current && <Animated.View style={[s.barFill, index === current && {transform: [{scaleX: drawn}]}]} />}
              </View>
            )}
            <View accessibilityRole="text" accessibilityLabel={`${index + 1} ${name}${done ? ' 끝' : now ? ', 지금' : ''}`} style={s.step}>
              <View style={[s.dot, (done || now) && s.dotOn]}><Text style={[s.dotText, (done || now) && s.dotTextOn]}>{index + 1}</Text></View>
              <Text style={[s.stepName, now && s.stepNameNow]}>{name}</Text>
            </View>
          </React.Fragment>
        );
      })}
    </View>
  );
}

const green = '#174b38';
const s = StyleSheet.create({
  mask: {overflow: 'hidden'},
  ring: {borderWidth: 4, borderColor: green},
  mark: {backgroundColor: green, alignItems: 'center', justifyContent: 'center'},
  check: {color: '#fff', fontWeight: '800', textAlign: 'center'},
  steps: {flexDirection: 'row', alignItems: 'flex-start', alignSelf: 'stretch'},
  step: {alignItems: 'center', gap: 6, width: 96, zIndex: 1},
  dot: {width: 40, height: 40, borderRadius: 20, borderWidth: 2, borderColor: green, alignItems: 'center', justifyContent: 'center', backgroundColor: '#fffefa'},
  dotOn: {backgroundColor: green},
  dotText: {fontSize: 16, fontWeight: '800', color: '#4f655a'},
  dotTextOn: {color: '#fff'},
  stepName: {fontSize: 14, color: '#4f655a', textAlign: 'center'},
  stepNameNow: {color: '#182d25', fontWeight: '800'},
  bar: {flex: 1, minWidth: 24, height: 6, marginHorizontal: -20, marginTop: 17, backgroundColor: '#d5dfd3', overflow: 'hidden'},
  barFill: {position: 'absolute', top: 0, bottom: 0, left: 0, right: 0, backgroundColor: green, transformOrigin: 'left'},
});
