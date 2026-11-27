import {useEffect, useState} from 'react';
import type {Spec} from '../specs/NativeOwnerWallet.ts';

/** Keeps the window out of screenshots and the recent-apps view while `needed` (OwnerWallet.setHomeProtection). */
export function useScreenProtection(needed: boolean, native: Pick<Spec, 'setHomeProtection'> | null | undefined) {
  const [on, setOn] = useState(false);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (!needed) { setOn(false); return; }
    if (!native?.setHomeProtection) { setOn(false); setFailed(true); return; }
    let mounted = true;
    setOn(false);
    setFailed(false);
    native.setHomeProtection(true).then(() => { if (mounted) setOn(true); }).catch(() => { if (mounted) { setOn(false); setFailed(true); } });
    return () => { mounted = false; setOn(false); native.setHomeProtection(false).catch(() => undefined); };
  }, [needed, native, attempt]);
  return {on, failed, retry: () => setAttempt(value => value + 1)};
}
