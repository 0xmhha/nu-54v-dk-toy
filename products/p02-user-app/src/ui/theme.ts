// Design tokens of the wallet app. Values come from the screen design book
// (docs/content/commercial/screens/book.css and screens.css) so the app and the book read as one
// product. Change them there first, then here.
import {Platform} from 'react-native';

/** Pretendard is bundled on Android (MainApplication registers it); iOS keeps the system font for now. */
export const font = Platform.OS === 'android' ? 'Pretendard' : undefined;

export const color = {
  ink: '#182d25',      // primary text
  green: '#174b38',    // brand surface, primary action
  deep: '#123a2b',     // text on mint
  mint: '#d4efa7',     // highlight, chips on green
  mintSoft: '#e9f3df', // soft action background
  mute: '#4f655a',     // secondary text (6.1:1 on sheet)
  line: '#d5dfd3',     // hairlines
  sheet: '#fffefa',    // cards
  ground: '#f1f4ec',   // screen background
  amber: '#8a5a00',    // waiting text
  amberBg: '#f6e3bf',
  danger: '#9b2c2c',
  dangerBg: '#f8e1df',
  onGreen: '#f4f8ed',
  onGreenSoft: '#cfe0cf',
} as const;

export const type = {
  display: {fontFamily: font, fontSize: 40, fontWeight: '800' as const, letterSpacing: -1.6, color: color.ink},
  title: {fontFamily: font, fontSize: 28, fontWeight: '800' as const, letterSpacing: -0.8, color: color.ink},
  section: {fontFamily: font, fontSize: 18, fontWeight: '700' as const, color: color.ink},
  body: {fontFamily: font, fontSize: 16, lineHeight: 24, color: color.ink},
  small: {fontFamily: font, fontSize: 14, lineHeight: 20, color: color.mute},
  label: {fontFamily: font, fontSize: 15, fontWeight: '700' as const, color: color.ink},
};

export const space = {xs: 4, s: 8, m: 12, l: 16, xl: 24};
export const radius = {s: 8, m: 12, l: 18, pill: 999};

/** Placeholder text color for every input; the system dark theme would otherwise draw a light hint on a light field. */
export const inputPlaceholder = color.mute;
