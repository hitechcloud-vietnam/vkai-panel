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
import { Unavailable } from '@/components/Unavailable';
import { Skeleton, StateMessage } from './StatCard';

/**
 * Khoi "Luu luong": tab Luu luong mang / Doc ghi o dia,
 * hang so lieu tong hop va bieu do duong theo thoi gian.
 *
 * Bieu do dung recharts (da co san trong package.json - khong them thu vien moi).
 * Chuoi 1 dung mau navy thuong hieu, chuoi 2 dung mau cyan.
 */
export interface TrafficPoint {
  /** Nhan truc thoi gian, da dinh dang san (khong goi new Date() khi render). */
  time: string;
  /** `null` khi thieu diem do. */
  up: number | null;
  down: number | null;
}

export interface TrafficMetric {
  label: string;
  /** Formatted string; `null` when the API has not reported this field. */
  value: string | null;
  /** Why the figure is missing. Shown in a tooltip in place of the value. */
  reason?: string;
}

export interface TrafficSeries {
  points: TrafficPoint[];
  upLabel: string;
  downLabel: string;
  metrics: TrafficMetric[];
}

export interface TrafficPanelProps {
  network: TrafficSeries;
  disk: TrafficSeries;
  loading?: boolean;
  error?: string | null;
  /** Chu thich hien khi chua co du lieu bieu do. */
  emptyHint?: string;
}

const TABS = [
  { id: 'network', label: 'Lưu lượng mạng' },
  { id: 'disk', label: 'Đọc ghi ổ đĩa' },
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
  const [tab, setTab] = useState<TabId>('network');

  const active: TrafficSeries = tab === 'disk' ? disk : network;
  const points = Array.isArray(active?.points) ? active.points : [];
  const metrics = Array.isArray(active?.metrics) ? active.metrics : [];

  return (
    <div className="flex min-w-0 flex-col gap-4">
      {/* Tab */}
      <div role="tablist" aria-label="Loại lưu lượng" className="flex flex-wrap gap-2">
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
              {item.label}
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
        {/* Hang so lieu */}
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
                    <Unavailable
                      reason={metric?.reason || 'Chưa có dữ liệu: API chưa trả về số liệu này.'}
                    />
                  )}
                </p>
              </div>
            ))}
          </div>
        )}

        {/* Bieu do */}
        {loading ? (
          <Skeleton className="h-56 w-full" />
        ) : error ? (
          <StateMessage tone="error" title="Không tải được dữ liệu lưu lượng" hint={error} />
        ) : points.length === 0 ? (
          <StateMessage
            icon={<LineChartIcon size={32} aria-hidden="true" />}
            title="Chưa có dữ liệu lưu lượng"
            hint={emptyHint || 'API hiện chưa trả về chuỗi số liệu theo thời gian.'}
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
