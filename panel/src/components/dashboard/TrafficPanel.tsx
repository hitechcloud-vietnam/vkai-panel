'use client';

import { useState } from 'react';
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { LineChart as LineChartIcon } from 'lucide-react';
import { colors } from '@/lib/brand';
import { useT } from '@/i18n';
import { Unavailable } from '@/components/Unavailable';
import { Skeleton, StateMessage } from './StatCard';

/**
 * The "Traffic" block: a network / disk tab pair, a row of summary figures and a
 * line chart over time.
 *
 * The chart uses recharts, which package.json already carries - no new library.
 * The first series is drawn in the brand navy, the second in cyan.
 */
export interface TrafficPoint {
  /** Time-axis label, already formatted (never call new Date() during render). */
  time: string;
  /** `null` when the sample is missing. */
  up: number | null;
  down: number | null;
}

export interface TrafficMetric {
  /** Already translated by the caller. */
  label: string;
  /** Formatted string; `null` when the API has not reported this field. */
  value: string | null;
  /** Why the figure is missing. Shown in a tooltip in place of the value. */
  reason?: string;
}

export interface TrafficSeries {
  points: TrafficPoint[];
  /** Series names, already translated by the caller. */
  upLabel: string;
  downLabel: string;
  metrics: TrafficMetric[];
}

export interface TrafficPanelProps {
  network: TrafficSeries;
  disk: TrafficSeries;
  loading?: boolean;
  error?: string | null;
  /** Note shown when the chart has no data yet, already translated. */
  emptyHint?: string;
}

/** Tab ids are machine values; the labels come from the dictionary. */
const TABS = [
  { id: 'network', labelKey: 'dashboard.traffic.tab.network' },
  { id: 'disk', labelKey: 'dashboard.traffic.tab.disk' },
] as const;

type TabId = (typeof TABS)[number]['id'];

const GRID_COLOR = '#E5E7EB';
const AXIS_COLOR = '#6B7280';

export default function TrafficPanel({
  network,
  disk,
  loading = false,
  error = null,
  emptyHint,
}: TrafficPanelProps) {
  const t = useT();
  const [tab, setTab] = useState<TabId>('network');

  const active: TrafficSeries = tab === 'disk' ? disk : network;
  const points = Array.isArray(active?.points) ? active.points : [];
  const metrics = Array.isArray(active?.metrics) ? active.metrics : [];

  return (
    <div className="flex min-w-0 flex-col gap-4">
      {/* Tabs */}
      <div
        role="tablist"
        aria-label={t('dashboard.traffic.tablistLabel')}
        className="flex flex-wrap gap-2"
      >
        {TABS.map((item) => {
          const selected = tab === item.id;
          return (
            <button
              key={item.id}
              type="button"
              role="tab"
              id={`traffic-tab-${item.id}`}
              aria-selected={selected}
              aria-controls={`traffic-panel-${item.id}`}
              onClick={() => setTab(item.id)}
              className={
                selected
                  ? 'rounded-md border border-brand-200 bg-brand-50 px-3 py-1.5 text-sm font-medium text-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500'
                  : 'rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500'
              }
            >
              {t(item.labelKey)}
            </button>
          );
        })}
      </div>

      <div
        role="tabpanel"
        id={`traffic-panel-${tab}`}
        aria-labelledby={`traffic-tab-${tab}`}
        className="flex min-w-0 flex-col gap-4"
      >
        {/* Summary figures */}
        {loading ? (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i}>
                <Skeleton className="h-3 w-16" />
                <Skeleton className="mt-2 h-5 w-20" />
              </div>
            ))}
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {metrics.map((metric, index) => (
              <div key={`${metric?.label || 'metric'}-${index}`} className="min-w-0">
                <p className="truncate text-xs text-gray-500">{metric?.label}</p>
                <p className="mt-1 truncate text-base font-semibold text-gray-900">
                  {metric?.value ? (
                    metric.value
                  ) : (
                    <Unavailable reason={metric?.reason || t('common.reason.noFigure')} />
                  )}
                </p>
              </div>
            ))}
          </div>
        )}

        {/* Chart */}
        {loading ? (
          <Skeleton className="h-56 w-full" />
        ) : error ? (
          <StateMessage
            tone="error"
            title={t('dashboard.traffic.loadFailed')}
            hint={error}
          />
        ) : points.length === 0 ? (
          <StateMessage
            icon={<LineChartIcon size={32} aria-hidden="true" />}
            title={t('dashboard.traffic.emptyTitle')}
            hint={emptyHint || t('dashboard.traffic.emptyHint')}
          />
        ) : (
          <>
            <div className="h-56 w-full">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={points} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
                  <CartesianGrid stroke={GRID_COLOR} strokeDasharray="3 3" vertical={false} />
                  <XAxis
                    dataKey="time"
                    stroke={AXIS_COLOR}
                    tick={{ fill: AXIS_COLOR, fontSize: 12 }}
                    tickLine={false}
                    axisLine={{ stroke: GRID_COLOR }}
                  />
                  <YAxis
                    stroke={AXIS_COLOR}
                    tick={{ fill: AXIS_COLOR, fontSize: 12 }}
                    tickLine={false}
                    axisLine={{ stroke: GRID_COLOR }}
                    width={48}
                  />
                  <Tooltip
                    contentStyle={{
                      background: '#FFFFFF',
                      border: `1px solid ${GRID_COLOR}`,
                      borderRadius: 6,
                      fontSize: 12,
                      color: '#111827',
                    }}
                    labelStyle={{ color: AXIS_COLOR }}
                  />
                  <Line
                    type="monotone"
                    dataKey="up"
                    name={active?.upLabel}
                    stroke={colors.navy}
                    strokeWidth={2}
                    dot={false}
                    isAnimationActive={false}
                  />
                  <Line
                    type="monotone"
                    dataKey="down"
                    name={active?.downLabel}
                    stroke={colors.cyan}
                    strokeWidth={2}
                    dot={false}
                    isAnimationActive={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
            <div className="flex flex-wrap items-center gap-4 text-xs text-gray-600">
              <span className="inline-flex items-center gap-1.5">
                <span
                  className="h-0.5 w-4 rounded"
                  style={{ background: colors.navy }}
                  aria-hidden="true"
                />
                {active?.upLabel}
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span
                  className="h-0.5 w-4 rounded"
                  style={{ background: colors.cyan }}
                  aria-hidden="true"
                />
                {active?.downLabel}
              </span>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
