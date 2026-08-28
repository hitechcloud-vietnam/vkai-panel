'use client';

/**
 * A progress ring (donut) drawn as plain SVG - no external library.
 *
 * Colour by threshold:  < 70%  -> emerald-600 (healthy)
 *                      70-89%  -> amber-600   (warning)
 *                      >= 90%  -> red-600     (critical)
 * When the API has not reported a figure, pass value = null: the ring stays
 * grey and the centre reads "—".
 */

import { useFormatters, useT } from '@/i18n';

/** Ring track (gray-200 / #E5E7EB, the interface convention). */
const TRACK_COLOR = '#E5E7EB';
/** emerald-600 */
const GOOD_COLOR = '#059669';
/** amber-600 */
const WARN_COLOR = '#D97706';
/** red-600 */
const DANGER_COLOR = '#DC2626';
/** gray-400 - used when there is no reading */
const EMPTY_COLOR = '#9CA3AF';

export interface DonutGaugeProps {
  /** Percentage 0-100. Pass `null` when the API has not reported this field. */
  value: number | null;
  /** Label under the ring, already translated by the caller, for example "CPU". */
  label: string;
  /** Detail line under the label, for example "8 cores" or "5.2 GB / 15.6 GB". */
  detail?: string;
  /** Ring diameter, 120px by default. */
  size?: number;
  /**
   * Why there is no reading. Used only when `value` is null: it becomes the
   * dial's accessible name, its tooltip and the line under the label, so an
   * empty dial never reads as an idle machine.
   */
  unavailableReason?: string;
}

/** Clamp to 0-100, or null when the input is not a usable number. */
export function clampPercent(value: number | null | undefined): number | null {
  if (value === null || value === undefined) return null;
  const num = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(num)) return null;
  return Math.min(100, Math.max(0, num));
}

/** Ring colour for the warning thresholds. */
export function gaugeColor(value: number | null | undefined): string {
  const percent = clampPercent(value);
  if (percent === null) return EMPTY_COLOR;
  if (percent >= 90) return DANGER_COLOR;
  if (percent >= 70) return WARN_COLOR;
  return GOOD_COLOR;
}

export default function DonutGauge({
  value,
  label,
  detail,
  size = 120,
  unavailableReason,
}: DonutGaugeProps) {
  const t = useT();
  const { formatNumber } = useFormatters();
  const percent = clampPercent(value);
  const stroke = Math.max(8, Math.round(size / 12));
  const radius = Math.max(1, (size - stroke) / 2);
  const circumference = 2 * Math.PI * radius;
  const dashOffset = percent === null ? circumference : circumference * (1 - percent / 100);
  const center = size / 2;
  const color = gaugeColor(percent);
  // The reading is an integer; Intl still owns it, so one rule covers every
  // number the panel prints.
  const reading = percent === null ? '' : formatNumber(Math.round(percent));
  const centerText = percent === null ? '—' : t('common.percent', { n: reading });
  const ariaLabel =
    percent === null
      ? t('dashboard.gauge.ariaUnavailable', {
          label,
          reason: unavailableReason || t('common.noData'),
        })
      : detail
        ? t('dashboard.gauge.ariaValueDetail', { label, n: reading, detail })
        : t('dashboard.gauge.ariaValue', { label, n: reading });
  const detailText =
    percent === null ? detail || (unavailableReason ? t('common.noDataShort') : '') : detail;

  return (
    <div className="flex min-w-0 flex-col items-center text-center">
      <svg
        width={size}
        height={size}
        viewBox={`0 0 ${size} ${size}`}
        role="img"
        aria-label={ariaLabel}
        className="shrink-0"
      >
        {percent === null && unavailableReason && <title>{unavailableReason}</title>}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke={TRACK_COLOR}
          strokeWidth={stroke}
        />
        {percent !== null && percent > 0 && (
          <circle
            cx={center}
            cy={center}
            r={radius}
            fill="none"
            stroke={color}
            strokeWidth={stroke}
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={dashOffset}
            transform={`rotate(-90 ${center} ${center})`}
            className="transition-[stroke-dashoffset] duration-500"
          />
        )}
        <text
          x={center}
          y={center}
          textAnchor="middle"
          dominantBaseline="central"
          fill="#111827"
          fontWeight={600}
          fontSize={Math.round(size * 0.22)}
        >
          {centerText}
        </text>
      </svg>
      <p className="mt-3 truncate text-sm font-medium text-gray-900">{label}</p>
      <p
        className={
          percent === null && unavailableReason
            ? 'mt-0.5 cursor-help text-xs text-gray-500 underline decoration-dotted underline-offset-2'
            : 'mt-0.5 text-xs text-gray-500'
        }
        title={percent === null ? unavailableReason : undefined}
      >
        {detailText || '—'}
      </p>
    </div>
  );
}

/** Pale frame shown while the figures are loading. */
export function DonutGaugeSkeleton({ size = 120 }: { size?: number }) {
  return (
    <div className="flex flex-col items-center" aria-hidden="true">
      <div
        className="rounded-full border-[10px] border-gray-100 bg-white"
        style={{ width: size, height: size }}
      />
      <div className="mt-3 h-4 w-20 rounded bg-gray-100" />
      <div className="mt-2 h-3 w-24 rounded bg-gray-100" />
    </div>
  );
}
