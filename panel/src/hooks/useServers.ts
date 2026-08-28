'use client';

/**
 * The list of machines this panel drives, and which of them is the machine the
 * panel itself runs on.
 *
 * Every screen that needs to know "where does this action happen" reads it from
 * here, so the answer is the same everywhere: the panel host when it is the
 * only node, and the operator's choice when it is not.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';

import { serverApi, unwrapList } from '@/services/api';
import { defaultServerId, findLocalNode } from '@/lib/servers';
import type { ManagedServer } from '@/types/server';
import { errorMessage } from '@/lib/apiError';

export interface UseServersResult {
  servers: ManagedServer[];
  /** The machine the panel is installed on, when the API reports one. */
  localNode: ManagedServer | null;
  /** Which node an action applies to unless the operator says otherwise. */
  defaultId: string;
  /** True while the panel drives one machine only - no picker is worth showing. */
  singleNode: boolean;
  loading: boolean;
  error: string | null;
  reload: () => Promise<void>;
}

/** A page is enough of a scope for one node list; nothing here is cached across pages. */
export function useServers(): UseServersResult {
  const [servers, setServers] = useState<ManagedServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      // per_page is generous on purpose: a picker that silently drops nodes
      // past page one would send an operator's website to the wrong machine.
      const res = await serverApi.list({ page: 1, per_page: 200 });
      setServers(unwrapList<ManagedServer>(res));
    } catch (err: any) {
      setServers([]);
      setError(
        errorMessage(err, 'Failed to load servers')
      );
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const localNode = useMemo(() => findLocalNode(servers), [servers]);
  const defaultId = useMemo(() => defaultServerId(servers), [servers]);

  return {
    servers,
    localNode,
    defaultId,
    singleNode: servers.length <= 1,
    loading,
    error,
    reload,
  };
}

export default useServers;
