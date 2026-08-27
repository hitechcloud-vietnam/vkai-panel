'use client';

/**
 * Vong tien trinh (donut) ve bang SVG thuan - khong dung thu vien ngoai.
 *
 * Mau theo nguong:  < 70%  -> emerald-600 (tot)
 *                  70-89%  -> amber-600  (canh bao)
 *                  >= 90%  -> red-600    (nguy hiem)
 * Khi API chua tra du lieu, truyen value = null: vong hien nen xam va chu "—".
 */

/** Nen vong (gray-200 / #E5E7EB theo quy uoc giao dien). */
const TRACK_COLOR = '#E5E7EB';
/** emerald-600 */
const GOOD_COLOR = '#059669';
/** amber-600 */
const WARN_COLOR = '#D97706';
/** red-600 */
const DANGER_COLOR = '#DC2626';
/** gray-400 - dung khi chua co du lieu */
const EMPTY_COLOR = '#9CA3AF';

export interface DonutGaugeProps {
  /** Phan tram 0-100. Truyen `null` khi API chua tra truong nay. */
  value: number | null;
  /** Nhan hien duoi vong, vi du "CPU". */
  label: string;
  /** Dong chi tiet duoi nhan, vi du "8 nhân" hoac "5.2 GB / 15.6 GB". */
  detail?: string;
  /** Duong kinh vong, mac dinh 120px. */
  size?: number;
}

/** Ep gia tri ve khoang 0-100, tra null khi khong phai so hop le. */
export function clampPercent(value: number | null | undefined): number | null {
  if (value === null || value === undefined) return null;
  const num = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(num)) return null;
  return Math.min(100, Math.max(0, num));
}

/** Mau vong theo nguong canh bao. */
export function gaugeColor(value: number | null | undefined): string {
  const percent = clampPercent(value);
  if (percent === null) return EMPTY_COLOR;
  if (percent >= 90) return DANGER_COLOR;
  if (percent >= 70) return WARN_COLOR;
  return GOOD_COLOR;
}

export default function DonutGauge({ value, label, detail, size = 120 }: DonutGaugeProps) {
  const percent = clampPercent(value);
  const stroke = Math.max(8, Math.round(size / 12));
  const radius = Math.max(1, (size - stroke) / 2);
  const circumference = 2 * Math.PI * radius;
  const dashOffset = percent === null ? circumference : circumference * (1 - percent / 100);
  const center = size / 2;
  const color = gaugeColor(percent);
  const centerText = percent === null ? '—' : `${Math.round(percent)}%`;
  const ariaLabel =
    percent === null
      ? `${label}: chưa có dữ liệu`
      : `${label}: ${Math.round(percent)} phần trăm${detail ? `, ${detail}` : ''}`;

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
      <p className="mt-0.5 text-xs text-gray-500">{detail || '—'}</p>
    </div>
  );
}

/** Khung xam nhat khi dang tai du lieu. */
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
