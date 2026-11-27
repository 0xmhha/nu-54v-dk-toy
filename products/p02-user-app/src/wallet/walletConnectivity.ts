import {useEffect, useState} from 'react';
import {AppState} from 'react-native';
import type {Spec} from '../specs/NativeOwnerWallet.ts';

export type WalletConnectivity = {link: 'unknown' | 'online' | 'offline'; revision: number};
export type ReadProof = {scope: string; endpoint: string; time: number; revision: number};
export const unknownConnectivity: WalletConnectivity = {link:'unknown', revision:0};

export function useWalletConnectivity(native: Pick<Spec, 'getNetworkLink' | 'onNetworkLink'> | null | undefined, active: boolean) {
  const [state, setState] = useState(unknownConnectivity);
  useEffect(() => {
    if (!native?.getNetworkLink || !native.onNetworkLink || !active) {
      setState(before => before.link === 'unknown' ? before : {link:'unknown',revision:before.revision + 1});
      return;
    }
    let open = true, sequence = 0;
    let foreground = AppState.currentState !== 'inactive' && AppState.currentState !== 'background';
    const update = (value: string) => {
      if (!open) return;
      const link = value === 'online' || value === 'offline' ? value : 'unknown';
      setState(before => before.link === link ? before : {link,revision:before.revision + 1});
    };
    const listener = native.onNetworkLink(value => {
      if (!foreground) return;
      sequence++;
      update(value);
    });
    const read = () => {
      const started = ++sequence;
      native.getNetworkLink().then(value => {if (sequence === started) update(value);})
        .catch(() => {if (sequence === started) update('unknown');});
    };
    const app = AppState.addEventListener('change', status => {
      foreground = status === 'active';
      if (foreground) read();
      else {sequence++; update('unknown');}
    });
    if (foreground) read();
    return () => {
      open=false;
      listener.remove();
      app.remove();
    };
  }, [active, native]);
  return state;
}
