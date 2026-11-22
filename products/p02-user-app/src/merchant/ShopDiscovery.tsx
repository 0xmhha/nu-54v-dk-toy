import {inputPlaceholder} from '../ui/theme.ts';
import React, { useEffect, useRef, useState } from 'react';
import { Button, StyleSheet, Text, TextInput, View } from 'react-native';
import { readEntranceQr, type ShopIdentity } from '@nu54/protocol';

export { readEntranceQr, readShopIdentity } from '@nu54/protocol';

export type ShopMatch = {
  identity: ShopIdentity;
  route: 'relay' | 'direct';
};

type Props = {
  initialQr?: string;
  offline?: boolean;
  permissionDenied?: boolean;
  find: () => Promise<{ handle: string }[] | undefined>;
  select: (handle: string) => Promise<ShopMatch>;
  online: (branch?: string) => Promise<ShopMatch>;
  readQr: () => Promise<string>;
  open: (value: ShopMatch) => Promise<void>;
  reset: () => void;
  recover?: () => Promise<void>;
  wallet: () => void;
};

const compare = (value: string) => value.trim().normalize('NFC');

export function ShopDiscovery({
  initialQr,
  offline,
  permissionDenied,
  find,
  select,
  online,
  readQr,
  open,
  reset,
  recover,
  wallet,
}: Props) {
  const [state, setState] = useState<
    'search' | 'fallback' | 'offline' | 'confirm'
  >(offline ? 'offline' : 'search');
  const [devices, setDevices] = useState<{ handle: string; number: number }[]>(
    [],
  );
  const [match, setMatch] = useState<ShopMatch>();
  const [sign, setSign] = useState('');
  const [rawQr, setRawQr] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const running = useRef(false);
  const mounted = useRef(true);
  const lastQr = useRef<string | undefined>(undefined);

  function show(value: ShopMatch) {
    if (!mounted.current) return;
    setMatch(value);
    setState('confirm');
  }

  function search() {
    reset();
    setDevices([]);
    setSign('');
    setMatch(undefined);
    setMessage('');
    lastQr.current = undefined;
    setState('search');
  }

  async function perform(
    action: () => Promise<void>,
    failure: (error: unknown) => void,
  ) {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setMessage('');
    try {
      await action();
    } catch (error) {
      if (mounted.current) failure(error);
    } finally {
      running.current = false;
      if (mounted.current) setBusy(false);
    }
  }

  async function scan() {
    reset();
    setMatch(undefined);
    setDevices([]);
    const rows = await find();
    if (!mounted.current) return;
    if (!rows?.length) {
      setState('fallback');
      setMessage(
        rows
          ? '근거리 단말을 찾을 수 없습니다.'
          : 'BLE 권한 또는 연결을 사용할 수 없습니다.',
      );
      return;
    }
    setDevices(
      rows.map(({ handle }, index) => ({ handle, number: index + 1 })),
    );
    setState('search');
  }

  async function openQr(raw: string) {
    const branch = readEntranceQr(raw);
    lastQr.current = raw;
    reset();
    show(await online(branch));
  }

  function qrFailure(error: unknown) {
    const reason = error instanceof Error ? error.message : '';
    if (reason === 'SHOP_ENTRANCE_QR' || reason === 'SHOP_WRONG_BRANCH') {
      setState('fallback');
      setMessage('등록 지점의 매장 입구 QR을 확인하세요.');
    } else if (reason === 'SHOP_CAMERA') {
      setState('fallback');
      setMessage(
        '카메라를 사용할 수 없습니다. 매장 입구 QR 주소를 입력하세요.',
      );
    } else setState('offline');
  }

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => {
    if (permissionDenied && !offline) {
      setState('fallback');
      setMessage('BLE 권한 또는 연결을 사용할 수 없습니다.');
    }
  }, [offline, permissionDenied]);

  useEffect(() => {
    if (initialQr) void perform(() => openQr(initialQr), qrFailure);
    // The parent remounts for each incoming link and the configured account is fixed here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialQr]);

  const requestedName = compare(sign);
  const nameMismatch =
    match &&
    requestedName !== '' &&
    requestedName !== compare(match.identity.branchLabel);
  return (
    <View style={styles.block}>
      {state === 'search' && (
        <>
          <Text accessibilityRole="header" style={styles.title}>
            매장 이름과 단말을 확인하세요
          </Text>
          <Text>
            단말 신호로 근처 후보를 찾습니다. 단말과 간판의 지점을 대조하세요.
          </Text>
          <TextInput placeholderTextColor={inputPlaceholder}
            accessibilityLabel="간판의 지점명"
            placeholder="간판의 지점명 (선택)"
            editable={!busy}
            maxLength={240}
            style={styles.input}
            value={sign}
            onChangeText={setSign}
          />
          <Button
            title="근처 단말 찾기"
            disabled={busy}
            onPress={() => perform(scan, () => setState('fallback'))}
          />
          {devices.map(({ handle, number }) => (
            <Button
              key={handle}
              title={`근거리 단말 후보 ${number} 인증`}
              disabled={busy}
              onPress={() =>
                perform(
                  async () => show(await select(handle)),
                  error => {
                    setDevices(current =>
                      current.filter(candidate => candidate.handle !== handle),
                    );
                    if (
                      error instanceof Error &&
                      error.message === 'SHOP_RELAY_OFFLINE'
                    )
                      setState('offline');
                    else
                      setMessage('지점 또는 단말 신원을 확인할 수 없습니다.');
                  },
                )
              }
            />
          ))}
          <Button
            title="온라인으로 매장 확인"
            disabled={busy}
            onPress={() =>
              perform(async () => {
                reset();
                show(await online());
              }, qrFailure)
            }
          />
          <Button
            title="매장 입구 QR 사용"
            disabled={busy}
            onPress={() => setState('fallback')}
          />
        </>
      )}
      {state === 'confirm' && match && (
        <>
          <Text
            accessibilityRole="header"
            style={styles.title}
          >{`${match.identity.branchLabel}에서 주문`}</Text>
          <Text>간판의 지점과 발견한 단말을 대조하세요.</Text>
          {match.identity.terminal && (
            <Text>{`발견된 단말 · 주문 단말 ${match.identity.terminal.slice(
              -8,
            )}`}</Text>
          )}
          <Text>
            연결 방식 ·{' '}
            {match.route === 'relay' ? '단말 BLE 중계' : '휴대전화 온라인 연결'}
          </Text>
          {nameMismatch && (
            <Text>입력한 지점명이 확인된 지점명과 다릅니다.</Text>
          )}
          <Button
            title="이 매장 메뉴 열기"
            disabled={busy || Boolean(nameMismatch)}
            onPress={() =>
              perform(
                () => open(match),
                () => setState('offline'),
              )
            }
          />
          <Button title="다른 매장 찾기" disabled={busy} onPress={search} />
        </>
      )}
      {state === 'fallback' && (
        <>
          <Text accessibilityRole="header" style={styles.title}>
            매장 입구의 QR을 스캔하세요
          </Text>
          <Text>휴대전화가 온라인이면 QR로 같은 매장을 열 수 있습니다.</Text>
          <Button
            title="카메라로 매장 QR 확인"
            disabled={busy}
            onPress={() =>
              perform(async () => {
                const raw = await readQr();
                if (raw) await openQr(raw);
              }, qrFailure)
            }
          />
          <TextInput placeholderTextColor={inputPlaceholder}
            accessibilityLabel="매장 입구 QR 주소"
            placeholder="매장 입구 QR 주소"
            autoCapitalize="none"
            style={styles.input}
            maxLength={96}
            value={rawQr}
            onChangeText={setRawQr}
          />
          <Button
            title="매장 입구 QR 확인"
            disabled={busy || !rawQr}
            onPress={() => perform(() => openQr(rawQr.trim()), qrFailure)}
          />
          <Button
            title="매장 연결 다시 찾기"
            disabled={busy}
            onPress={search}
          />
        </>
      )}
      {state === 'offline' && (
        <>
          <Text accessibilityRole="header" style={styles.title}>
            새 주문을 시작할 수 없습니다
          </Text>
          <Text>
            신규 발급 일시 중지. 연결이 돌아오면 이전 주문을 기존 ID로 확인할 수
            있습니다.
          </Text>
          <Button
            title="매장 상태 다시 확인"
            disabled={busy}
            onPress={() =>
              perform(async () => {
                if (lastQr.current) await openQr(lastQr.current);
                else if (match?.route === 'direct') show(await online());
                else await scan();
              }, qrFailure)
            }
          />
          {recover && (
            <Button
              title="원래 주문 상태 확인"
              disabled={busy}
              onPress={() =>
                perform(recover, () =>
                  setMessage('같은 주문의 연결이 돌아오면 다시 확인하세요.'),
                )
              }
            />
          )}
          <Button title="다른 매장 찾기" disabled={busy} onPress={search} />
          <Button title="내 지갑" disabled={busy} onPress={wallet} />
        </>
      )}
      {!!message && <Text accessibilityLiveRegion="polite">{message}</Text>}
    </View>
  );
}

const styles = StyleSheet.create({
  block: { gap: 10, marginVertical: 12 },
  title: { fontSize: 19, fontWeight: '600' },
  input: { borderWidth: 1, borderColor: '#999', padding: 10, color: '#151515' },
});
