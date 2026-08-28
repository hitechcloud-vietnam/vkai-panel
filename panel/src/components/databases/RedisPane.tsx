'use client';

/**
 * Redis, described the way Redis actually works.
 *
 * Redis has no databases in the SQL sense. It has one keyspace split into
 * sixteen numbered slots, a memory ceiling, an eviction policy and a
 * persistence mode, and an operator's first three questions are always "how
 * much memory", "which policy" and "is it saving to disk". A panel that renders
 * this as a table of "databases" with an owner and a size column is a panel
 * built by someone who has never run one, so this pane does not.
 *
 * What it also does not do is invent the figures. There is no Redis endpoint in
 * the API at all - not INFO, not DBSIZE, not CONFIG GET, not FLUSHDB. The
 * runtime card below therefore lays out the fields an operator looks for and
 * marks every one of them unavailable with the reason, which is the same
 * convention the dashboard uses for a metric no agent has reported. The layout
 * is real; the numbers are honestly absent.
 */

import { useMemo } from 'react';

import { Unavailable } from '@/components/Unavailable';
import type { ManagedServer } from '@/types/server';
import type { CapabilityGap, DBServer, DatabaseEngine } from '@/types/databases';

import { CapabilityGaps } from './CapabilityNotice';
import InstancesPanel, { type InstancesPanelProps } from './InstancesPanel';
import { EmptyState, Panel, SectionHeader } from './PaneChrome';
import { DASH, instanceHost, instancePort } from './helpers';

/** One reason, reused by every figure on this pane. */
const NO_REDIS_API =
  'The panel has no Redis endpoint. Nothing in core/internal/handler/router.go runs INFO, DBSIZE or CONFIG GET against a Redis instance.';

/** The numbered slots Redis ships with. `databases 16` is the default in redis.conf. */
const KEYSPACE_SLOTS = Array.from({ length: 16 }, (_, i) => i);

export const REDIS_GAPS: CapabilityGap[] = [
  {
    label: 'Memory use and peak',
    missing:
      'No endpoint runs INFO memory. used_memory, used_memory_peak and maxmemory are never collected.',
  },
  {
    label: 'Keyspace per numbered database',
    missing:
      'No endpoint runs INFO keyspace or DBSIZE, so key counts and expiring-key counts for db0 to db15 are unknown.',
  },
  {
    label: 'Persistence mode',
    missing:
      'No endpoint reads the RDB save rules or the appendonly setting, so the panel cannot say whether anything is being written to disk.',
  },
  {
    label: 'maxmemory policy',
    missing:
      'No endpoint runs CONFIG GET maxmemory-policy, so the panel cannot say whether the instance evicts or refuses writes when full.',
  },
  {
    label: 'Flush a numbered database',
    missing:
      'No endpoint runs FLUSHDB. The typed confirmation this action needs is already built (components/databases/ConfirmByName.tsx) and waits only on the route.',
  },
  {
    label: 'Keys, TTLs and values',
    missing:
      'No endpoint enumerates keys. aaPanel offers a key browser with type, length and expiry; the panel has no equivalent route.',
  },
  {
    label: 'Redis ACL users',
    missing:
      'No endpoint runs ACL LIST or ACL SETUSER. DatabaseService only knows how to create MySQL and PostgreSQL accounts.',
  },
];

/** A labelled figure the backend has never reported. */
function RuntimeFigure({ label, hint }: { label: string; hint?: string }) {
  return (
    <div className="rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5">
      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
      <p className="mt-1 text-sm font-semibold text-gray-900">
        <Unavailable reason={NO_REDIS_API} />
      </p>
      {hint && <p className="mt-0.5 text-xs text-gray-500">{hint}</p>}
    </div>
  );
}

export interface RedisPaneProps {
  engine: DatabaseEngine;
  servers: DBServer[];
  nodes: ManagedServer[];
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  onRefresh: () => void;
  search: string;
  onSearchChange: (value: string) => void;
  onRegister: InstancesPanelProps['onRegister'];
  onDeleteInstance: InstancesPanelProps['onDelete'];
}

export function RedisPane({
  engine,
  servers,
  nodes,
  loading,
  refreshing,
  error,
  onRefresh,
  search,
  onSearchChange,
  onRegister,
  onDeleteInstance,
}: RedisPaneProps) {
  const primary = servers[0] || null;
  const endpoint = useMemo(() => {
    if (!primary) return DASH;
    const host = instanceHost(primary, nodes);
    if (host === DASH) return DASH;
    return `${host}:${instancePort(primary, engine)}`;
  }, [primary, nodes, engine]);

  return (
    <div className="space-y-4">
      <InstancesPanel
        engine={engine}
        servers={servers}
        nodes={nodes}
        loading={loading}
        databaseCounts={{}}
        search={search}
        onSearchChange={onSearchChange}
        onRefresh={onRefresh}
        refreshing={refreshing}
        error={error}
        onRegister={onRegister}
        onDelete={onDeleteInstance}
      />

      <Panel>
        <SectionHeader
          title="Runtime"
          description="Redis holds one keyspace split into numbered slots, not a set of databases. These are the figures an operator needs before touching it."
        />
        {servers.length === 0 ? (
          <EmptyState
            title="No Redis instance is registered"
            description="Register the instance above. Even then the panel cannot read these figures yet - the API has no Redis endpoint - but the instance will at least be recorded against its node."
          />
        ) : (
          <div className="space-y-4 px-4 py-4">
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <RuntimeFigure label="Used memory" hint="INFO memory · used_memory_human" />
              <RuntimeFigure label="Peak memory" hint="INFO memory · used_memory_peak" />
              <RuntimeFigure label="maxmemory" hint="CONFIG GET maxmemory" />
              <RuntimeFigure
                label="maxmemory policy"
                hint="noeviction, allkeys-lru, volatile-ttl, …"
              />
              <RuntimeFigure label="Persistence" hint="RDB save rules and appendonly" />
              <RuntimeFigure label="Last save" hint="INFO persistence · rdb_last_save_time" />
              <RuntimeFigure label="Connected clients" hint="INFO clients" />
              <RuntimeFigure label="Uptime" hint="INFO server · uptime_in_days" />
            </div>

            <div>
              <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                Keyspace
              </h4>
              <p className="mt-1 text-sm text-gray-500">
                Sixteen numbered slots, the redis.conf default. A slot is empty until
                something writes to it, and Redis reports only the non-empty ones.
              </p>
              <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-8">
                {KEYSPACE_SLOTS.map((slot) => (
                  <div
                    key={slot}
                    className="rounded-md border border-gray-200 bg-white px-2.5 py-2 text-center"
                  >
                    <p className="font-mono text-xs font-semibold text-gray-900">
                      db{slot}
                    </p>
                    <p className="mt-0.5 text-xs text-gray-500">
                      <Unavailable reason={NO_REDIS_API} /> keys
                    </p>
                  </div>
                ))}
              </div>
            </div>

            <div className="rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5">
              <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
                Endpoint
              </p>
              <p className="mt-1 font-mono text-sm text-gray-900">{endpoint}</p>
              <p className="mt-1 text-xs text-gray-500">
                Redis has no per-database credentials. Authentication is a single
                requirepass or an ACL user, and the panel manages neither.
              </p>
            </div>
          </div>
        )}
      </Panel>

      <CapabilityGaps
        title="What this pane cannot do yet"
        intro="Each line is a Redis capability with no route behind it. None of them is offered as a control, because a control that quietly does nothing is worse than a gap that is written down."
        gaps={REDIS_GAPS}
      />
    </div>
  );
}

export default RedisPane;
