import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Ep gia tri ve mang mot cach an toan.
 * Backend Go serialize nil slice thanh `null`, nen moi cho .map() deu nen di qua day.
 */
export function toArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : []
}

/** Doc so an toan, tra ve gia tri thay the khi null/undefined/NaN. */
export function toNumber(value: unknown, fallback = 0): number {
  const num = typeof value === "number" ? value : Number(value)
  return Number.isFinite(num) ? num : fallback
}

/** Dinh dang dung luong theo don vi doc duoc. */
export function formatBytes(bytes: unknown, decimals = 1): string {
  const value = toNumber(bytes, 0)
  if (value <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB", "PB"]
  const i = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / Math.pow(1024, i)).toFixed(i === 0 ? 0 : decimals)} ${units[i]}`
}

/** Dinh dang phan tram an toan (khong bao gio goi .toFixed tren undefined). */
export function formatPercent(value: unknown, decimals = 1): string {
  return `${toNumber(value, 0).toFixed(decimals)}%`
}
