'use client';

/**
 * One fetch shape for every Docker pane.
 *
 * Each pane needs the same four things - a first load that shows a skeleton, a
 * refresh that does not blank the table underneath it, a failure that keeps the
 * message, and a result - and getting one of them subtly wrong per pane is how
 * a screen ends up with nine slightly different spinners. So it lives here
 * once.
 *
 * `refreshing` is separate from `loading` on purpose: a manual refresh must not
 * tear the rows out from under an operator who is reading them.
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import { errorMessage } from '@/lib/apiError';

export interface AsyncResource<T> {
  data: T;
  /** True only for the first load, when there is nothing to show yet. */
  loading: boolean;
  /** True for a reload that happens while data is already on screen. */
  refreshing: boolean;
  error: string | null;
  reload: () => void;
}

export function useAsyncResource<T>(
  /** Must be stable - wrap it in useCallback. */
  load: () => Promise<T>,
  initial: T,
  failureMessage: string
): AsyncResource<T> {
  const [data, setData] = useState<T>(initial);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // A pane can be unmounted by a tab change while its request is in flight.
  // Writing state after that is a React warning at best and a stale table at
  // worst, so every setState below is gated on the component still being there.
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  // Only the newest request may write. Two refreshes in quick succession can
  // otherwise land out of order and leave the older answer on screen.
  const requestId = useRef(0);

  const run = useCallback(
    async (isRefresh: boolean) => {
      const id = ++requestId.current;
      if (isRefresh) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const result = await load();
        if (!alive.current || id !== requestId.current) return;
        setData(result);
      } catch (err) {
        if (!alive.current || id !== requestId.current) return;
        setError(errorMessage(err, failureMessage));
      } finally {
        if (alive.current && id === requestId.current) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    },
    [load, failureMessage]
  );

  useEffect(() => {
    void run(false);
  }, [run]);

  const reload = useCallback(() => {
    void run(true);
  }, [run]);

  return { data, loading, refreshing, error, reload };
}
