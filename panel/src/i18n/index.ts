/**
 * The public surface of the translation layer. Import from '@/i18n' only -
 * the files behind it are free to move.
 */

export {
  DEFAULT_LOCALE,
  LOCALES,
  LOCALE_STORAGE_KEY,
  LOCALE_TAG,
  SOURCE_LOCALE,
  browserLocale,
  dictionaries,
  isLocale,
  persistLocale,
  resolveInitialLocale,
  sourceText,
  storedLocale,
  translate,
  translatePlural,
  type Locale,
  type TranslateFn,
  type TranslatePluralFn,
  type TranslationParams,
} from './core';

export { EMPTY_VALUE, formatDate, formatDateTime, formatNumber } from './format';

export {
  I18nProvider,
  useFormatters,
  useI18n,
  useLocale,
  useT,
  useTn,
} from './I18nProvider';

export { default as LanguageSwitcher } from './LanguageSwitcher';
