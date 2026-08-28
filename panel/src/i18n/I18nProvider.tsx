'use client';

/**
 * React binding for the translation layer.
 *
 *   import { useT } from '@/i18n';
 *   const t = useT();
 *   <span>{t('nav.dashboard')}</span>
 *   <span>{t('common.loadingProduct', { product: brand.productName })}</span>
 *
 * The provider wraps the whole application in app/layout.tsx, so every client
 * component below it can call the hooks. The one place that cannot is
 * app/global-error.tsx, which replaces the root layout when the application
 * fails to start; that file calls `translate` from './core' directly.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';
import {
  DEFAULT_LOCALE,
  LOCALES,
  persistLocale,
  resolveInitialLocale,
  translate,
  translatePlural,
  type Locale,
  type TranslateFn,
  type TranslatePluralFn,
} from './core';
import { formatDate, formatDateTime, formatNumber } from './format';

interface I18nValue {
  /** The locale currently in effect. */
  locale: Locale;
  /** Every locale the panel ships, in display order. */
  locales: readonly Locale[];
  /** Switches locale immediately and remembers the choice. No page reload. */
  setLocale: (locale: Locale) => void;
  /**
   * False on the server and on the first client render, true once the stored
   * choice and the browser preference have been read. Anything whose text
   * would otherwise differ between those two renders should wait for it.
   */
  ready: boolean;
  t: TranslateFn;
  tn: TranslatePluralFn;
  formatNumber: (value: number, options?: Intl.NumberFormatOptions) => string;
  formatDate: (value: Date | string | number) => string;
  formatDateTime: (value: Date | string | number) => string;
}

const I18nContext = createContext<I18nValue | null>(null);

export function I18nProvider({ children }: { children: React.ReactNode }) {
  // Starts at the default so the server's HTML and the first client render
  // agree. The real locale arrives in the effect below, one render later.
  const [locale, setLocaleState] = useState<Locale>(DEFAULT_LOCALE);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    setLocaleState(resolveInitialLocale());
    setReady(true);
  }, []);

  // Keeps <html lang> honest for screen readers and for the browser's own
  // translate prompt. app/layout.tsx renders lang="en" statically because a
  // server component cannot know the operator's choice.
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    persistLocale(next);
  }, []);

  const value = useMemo<I18nValue>(
    () => ({
      locale,
      locales: LOCALES,
      setLocale,
      ready,
      t: (key, params) => translate(locale, key, params),
      tn: (key, count, params) => translatePlural(locale, key, count, params),
      formatNumber: (v, options) => formatNumber(locale, v, options),
      formatDate: (v) => formatDate(locale, v),
      formatDateTime: (v) => formatDateTime(locale, v),
    }),
    [locale, ready, setLocale],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

function useI18nContext(): I18nValue {
  const value = useContext(I18nContext);
  if (!value) {
    // A wiring bug, not a runtime condition: the component is rendering
    // somewhere I18nProvider does not cover.
    throw new Error('useT / useI18n must be used inside <I18nProvider> (see app/layout.tsx)');
  }
  return value;
}

/** Everything the layer offers, for the rare component that needs more than t. */
export function useI18n(): I18nValue {
  return useI18nContext();
}

/** `t(key, params?)` - the hook nearly every component wants. */
export function useT(): TranslateFn {
  return useI18nContext().t;
}

/** `tn(key, count, params?)` - see translatePlural in ./core for the rule. */
export function useTn(): TranslatePluralFn {
  return useI18nContext().tn;
}

/** The current locale plus the switch, for a language control. */
export function useLocale(): Pick<I18nValue, 'locale' | 'locales' | 'setLocale' | 'ready'> {
  const { locale, locales, setLocale, ready } = useI18nContext();
  return { locale, locales, setLocale, ready };
}

/** Locale-aware Intl formatters. Read ./format before using them. */
export function useFormatters(): Pick<
  I18nValue,
  'formatNumber' | 'formatDate' | 'formatDateTime'
> {
  const { formatNumber: n, formatDate: d, formatDateTime: dt } = useI18nContext();
  return { formatNumber: n, formatDate: d, formatDateTime: dt };
}
