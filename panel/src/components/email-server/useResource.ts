'use client';

/**
 * One loader for every pane, so loading / failed / empty are never optional.
 *
 * `error` is always a string (lib/apiError turns the API's {error:{...}} object
 * into one). A pane that renders `state.error` therefore cannot repeat the
 * React #31 crash that used to blank whole pages here.
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import { errorMessage } from '@/lib/apiError';

export interface Resource<T> {
  data: T | null;
  loading: boolean;
  /** True while a reload runs over data that is already on screen. */
  refreshing: boolean;
  error: string;
  reload: () => void;
}

export function useResource<T>(
  load: () => Promise<T>,
  fallbackMessage: string,
  deps: unknown[] = []
): Resource<T> {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');

  // Kept in a ref so `run` does not change identity every render.
  const loadRef = useRef(load);
  loadRef.current = load;

  const hasData = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  const run = useCallback(async () => {
    if (hasData.current) setRefreshing(true);
    else setLoading(true);
    setError('');
    try {
      const result = await loadRef.current();
      if (!alive.current) return;
      setData(result);
      hasData.current = true;
    } catch (err) {
      if (!alive.current) return;
      setError(errorMessage(err, fallbackMessage));
    } finally {
      if (alive.current) {
        setLoading(false);
        setRefreshing(false);
      }
    }
  }, [fallbackMessage]);

  useEffect(() => {
    run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { data, loading, refreshing, error, reload: run };
}
