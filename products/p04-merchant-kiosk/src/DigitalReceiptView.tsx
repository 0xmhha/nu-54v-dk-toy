/**
 * The digital receipt after an approved payment (payment-protocol.md 6, receipt): the merchant,
 * the order and its lines, the total, how it was paid, and the settle transaction with a QR code
 * the customer scans to open it on the explorer.
 */

import { Linking, Pressable, StyleSheet, Text, View } from 'react-native';
import qrcode from 'qrcode-generator';
import { explorerTxUrl, receiptAmount, type DigitalReceipt } from '@nu54/protocol';

const short = (h: string) => `${h.slice(0, 8)}…${h.slice(-4)}`;

function timeText(unix: number): string {
  const d = new Date(unix * 1000);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

/** A QR code drawn with views: each row's runs of dark modules are one view. */
export function QrCode({ text, size = 200 }: { text: string; size?: number }) {
  const qr = qrcode(0, 'M');
  qr.addData(text);
  qr.make();
  const n = qr.getModuleCount();
  const cell = size / (n + 8); // a four-module quiet zone on each side
  const rows = [];
  for (let y = 0; y < n; y++) {
    const runs = [];
    for (let x = 0; x < n; ) {
      if (!qr.isDark(y, x)) {
        x++;
        continue;
      }
      const start = x;
      while (x < n && qr.isDark(y, x)) x++;
      runs.push(<View key={start} style={[s.dark, { left: (start + 4) * cell, width: (x - start) * cell, height: cell }]} />);
    }
    rows.push(<View key={y} style={[s.qrRow, { top: (y + 4) * cell, height: cell }]}>{runs}</View>);
  }
  return <View style={[s.qrBox, { width: size, height: size }]}>{rows}</View>;
}

export function DigitalReceiptView({ receipt: r, txHash, onNew }: { receipt: DigitalReceipt; txHash?: string; onNew: () => void }) {
  const amount = (v: string | bigint) => `${receiptAmount(v, r.token.decimals)} ${r.token.symbol}`;
  const url = txHash ? explorerTxUrl(r.chainId, txHash) : null;
  return (
    <View style={s.paper}>
      <Text style={s.store}>{r.store}</Text>
      {(r.representative || r.businessNumber) && (
        <Text style={s.small}>
          {r.representative ? `대표 ${r.representative}` : ''}
          {r.representative && r.businessNumber ? ' · ' : ''}
          {r.businessNumber ? `사업자등록번호 ${r.businessNumber}` : ''}
        </Text>
      )}
      {r.address && <Text style={s.small}>{r.address}</Text>}
      {r.phone && <Text style={s.small}>전화 {r.phone}</Text>}
      <View style={s.rule} />
      <View style={s.row}>
        <Text style={s.bold}>주문번호 {r.orderNumber}</Text>
        <Text style={s.small}>{timeText(r.time)}</Text>
      </View>
      <View style={s.rule} />
      {r.items.map((i, k) => (
        <View key={k} style={s.row}>
          <Text style={s.item}>
            {i.name} × {i.qty}
          </Text>
          <Text style={s.item}>{amount(BigInt(i.unitPrice) * BigInt(i.qty))}</Text>
        </View>
      ))}
      <View style={s.rule} />
      <View style={s.row}>
        <Text style={s.total}>합계</Text>
        <Text style={s.total}>{amount(r.total)}</Text>
      </View>
      <Text style={s.small}>결제수단 {r.token.symbol} (StableNet, chain {r.chainId})</Text>
      <Text style={s.small}>결제 계정 {short(r.payer)}</Text>
      {txHash && <Text style={s.small}>거래 {short(txHash)}</Text>}
      {url && (
        <View style={s.qr}>
          <QrCode text={url} />
          <Text style={s.small}>휴대폰 카메라로 찍으면 탐색기에서 거래를 확인할 수 있습니다</Text>
          <Pressable style={s.link} onPress={() => Linking.openURL(url)}>
            <Text style={s.linkText}>탐색기에서 보기</Text>
          </Pressable>
        </View>
      )}
      <Pressable style={s.button} onPress={onNew}>
        <Text style={s.buttonText}>새 주문</Text>
      </Pressable>
    </View>
  );
}

const s = StyleSheet.create({
  dark: { position: 'absolute', backgroundColor: '#000' },
  qrRow: { position: 'absolute', left: 0, right: 0 },
  qrBox: { backgroundColor: '#fff' },
  paper: { backgroundColor: '#fff', borderRadius: 8, padding: 16, marginTop: 8 },
  store: { fontSize: 22, fontWeight: '700', color: '#111', marginBottom: 4 },
  small: { fontSize: 13, color: '#555', marginTop: 2 },
  bold: { fontSize: 15, fontWeight: '600', color: '#111' },
  item: { fontSize: 15, color: '#111', marginVertical: 2 },
  total: { fontSize: 20, fontWeight: '700', color: '#111', marginVertical: 4 },
  row: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  rule: { borderBottomWidth: StyleSheet.hairlineWidth, borderColor: '#999', marginVertical: 8 },
  qr: { alignItems: 'center', marginTop: 12 },
  link: { marginTop: 8, paddingVertical: 10, paddingHorizontal: 16, borderRadius: 6, borderWidth: 1, borderColor: '#1b4fa0' },
  linkText: { color: '#1b4fa0', fontSize: 16, fontWeight: '600' },
  button: { backgroundColor: '#111', paddingVertical: 14, borderRadius: 8, alignItems: 'center', marginTop: 16 },
  buttonText: { color: '#fff', fontSize: 18, fontWeight: '600' },
});
