/**
 * Locale-aware number and date formatting, built on Intl - no dependency, and
 * no second date library beside the one the pages already use.
 *
 * WHAT IS SUPPORTED: integers and decimals, a medium date, and a medium date
 * with a short time. Everything is rendered in the BROWSER's time zone.
 *
 * WHAT THAT COSTS: a date formatted while the page is being server-rendered
 * would use the server's time zone and the default locale, so these helpers
 * belong in components that already render browser-side state (which, in this
 * panel, is every page that shows a date - the data arrives over the API after
 * mount). Do not print a formatted date from a component's first server render
 * and expect it to survive hydration.
 */

import { LOCALE_TAG, type Locale } from './core';

/** Shown where a value is genuinely absent, in place of a fabricated one. */
export const EMPTY_VALUE = '—';

function toDate(value: Date | string | number): Date | null {
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

export function formatNumber(
  locale: Locale,
  value: number,
  options?: Intl.NumberFormatOptions,
): string {
  if (!Number.isFinite(value)) return EMPTY_VALUE;
  return new Intl.NumberFormat(LOCALE_TAG[locale], options).format(value);
}

/** Medium date, for example "28 Aug 2026" / "28 thg 8, 2026". */
export function formatDate(locale: Locale, value: Date | string | number): string {
  const date = toDate(value);
  if (!date) return EMPTY_VALUE;
  return new Intl.DateTimeFormat(LOCALE_TAG[locale], { dateStyle: 'medium' }).format(date);
}

/** Medium date plus short time, in the browser's time zone. */
export function formatDateTime(locale: Locale, value: Date | string | number): string {
  const date = toDate(value);
  if (!date) return EMPTY_VALUE;
  return new Intl.DateTimeFormat(LOCALE_TAG[locale], {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}
