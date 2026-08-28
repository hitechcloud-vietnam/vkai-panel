'use client';

/**
 * Latest metrics for a set of nodes, keyed by node id.
 *
 * GET /api/v1/servers/:id/metrics answers 404 until an agent on that node has
 * reported at least once. That is not an error worth showing an operator - it
 * is a node whose figures are not available yet - so a missing sample lands in
 * the map as `null` and the caller renders it as unavailable, never as zero.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';

import { serverApi, unwrap } from '@/services/api';
import type { ServerMetrics } from '@/types/server';

export type MetricsById = Record<string, ServerMetrics | null>;

export interface UseServerMetricsResult {
  metrics: MetricsById;
  loading: boolean;
  reload: () => Promise<void>;
}

export function useServerMetrics(serverIds: string[]): UseServerMetricsResult {
  const [metrics, setMetrics] = useState<MetricsById>({});
  const [loading, setLoading] = useState(false);

  // A stable key so the effect does not refire on every parent render just
  // because the caller built a fresh array.
  const idsKey = useMemo(
    () => (Array.isArray(serverIds) ? serverIds.filter(Boolean).join(',') : ''),
    [serverIds]
  );

  const reload = useCallback(async () => {
    const ids = idsKey ? idsKey.split(',') : [];
    if (ids.length === 0) {
      setMetrics({});
      setLoading(false);
      return;
    }
    setLoading(true);
    const results = await Promise.allSettled(ids.map((id) => serverApi.metrics(id)));
    const next: MetricsById = {};
    ids.forEach((id, index) => {
      const result = results[index];
      next[id] =
        result.status === 'fulfilled'
          ? unwrap<ServerMetrics>(result.value, null)
          : null;
    });
    setMetrics(next);
    setLoading(false);
  }, [idsKey]);

  useEffect(() => {
    reload();
  }, [reload]);

  return { metrics, loading, reload };
}

export default useServerMetrics;
