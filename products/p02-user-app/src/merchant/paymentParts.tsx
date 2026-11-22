import React from 'react';
import {StyleSheet, Text, View} from 'react-native';
import {color, font, radius} from '../ui/theme.ts';

// Payment amount and pickup ticket of the design book's W10/W11 pages, shared by the payments
// screen and the fixture-only demo so both render the same parts.

export function AmountLine({amount, label = '금액'}: {amount: string; label?: string}) {
  return <Text style={styles.amountLine}>
    <Text style={styles.amountLabel}>{label} </Text><Text style={styles.amount}>{amount}</Text>
  </Text>;
}

export function PickupTicket({number}: {number: string | number}) {
  return <View style={styles.ticket} accessible accessibilityLabel={`픽업 번호 ${number}`}>
    <Text style={styles.ticketLabel}>픽업 번호</Text>
    <Text style={styles.ticketNumber}>{number}</Text>
  </View>;
}

const styles = StyleSheet.create({
  amountLine: {marginVertical: 4},
  amountLabel: {fontFamily: font, fontSize: 15, color: color.mute},
  amount: {fontFamily: font, fontSize: 34, fontWeight: '800', letterSpacing: -1.2, color: color.ink},
  ticket: {backgroundColor: color.mint, borderRadius: radius.l, paddingHorizontal: 20, paddingVertical: 16, gap: 2},
  ticketLabel: {fontFamily: font, fontSize: 15, fontWeight: '700', color: color.deep},
  ticketNumber: {fontFamily: font, fontSize: 56, fontWeight: '800', letterSpacing: -2, color: color.deep},
});
