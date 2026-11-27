import {useEffect, useRef} from 'react';
import {BackHandler} from 'react-native';

/** Brings the top of a page into view whenever what the page shows changes. */
export function usePageTop(page: string | undefined, top: (() => void) | undefined) {
  const first = useRef(true), latest = useRef(top);
  latest.current = top;
  useEffect(() => {
    if (first.current) { first.current = false; return; }
    latest.current?.();
  }, [page]);
}

/** System Back closes an inner page first. It is registered when the page opens, so it runs before the screen's own handler. */
export function useInnerBack(open: boolean, close: () => void) {
  const latest = useRef(close);
  latest.current = close;
  useEffect(() => {
    if (!open) return;
    const listener = BackHandler.addEventListener('hardwareBackPress', () => { latest.current(); return true; });
    return () => listener.remove();
  }, [open]);
}
