/**
 * The translation layer, without React.
 *
 * English is the SOURCE language: every key exists in en.json, and en.json is
 * the file a developer edits first. vi.json is a translation of it.
 *
 * This module is deliberately framework-free so that it can also serve
 * app/global-error.tsx, which replaces the root layout and therefore renders
 * outside the React provider. Everything else should use the hooks in
 * ./I18nProvider instead of calling `translate` directly.
 *
 * WHAT IS SUPPORTED
 *   - Flat dotted keys, e.g. "auth.signIn", produced by flattening the nested
 *     JSON files at module load.
 *   - Single-brace interpolation, e.g. "Loading {product}…".
 *   - A two-form plural rule (one / other) via `translatePlural`. That rule is
 *     complete for English and Vietnamese and for nothing else - see the note
 *     on that function before adding a third locale.
 *   - Number and date formatting via Intl, in ./format.
 *
 * WHAT IS NOT SUPPORTED, on purpose
 *   - Gender, ordinals, nested/rich message syntax (ICU MessageFormat).
 *     A message that needs markup in the middle is split into two keys and
 *     assembled in JSX, as errors.contactSupportLead is.
 */

import enDictionary from './en.json';
import viDictionary from './vi.json';

/** Every locale the panel ships. English first: it is the source language. */
export const LOCALES = ['en', 'vi'] as const;

export type Locale = (typeof LOCALES)[number];

/** The language every key is authored in. */
export const SOURCE_LOCALE: Locale = 'en';

/** Used until the operator's choice and the browser's preference are known. */
export const DEFAULT_LOCALE: Locale = SOURCE_LOCALE;

/** localStorage key holding the operator's explicit choice. */
export const LOCALE_STORAGE_KEY = 'vkai_locale';

/** BCP-47 tags, for Intl number and date formatting. */
export const LOCALE_TAG: Record<Locale, string> = {
  en: 'en-US',
  vi: 'vi-VN',
};

/** Values that may be substituted into a message. */
export type TranslationParams = Record<string, string | number>;

/** The shape of `t` handed out by useT(). */
export type TranslateFn = (key: string, params?: TranslationParams) => string;

/** The shape of `tn` handed out by useTn(). */
export type TranslatePluralFn = (
  key: string,
  count: number,
  params?: TranslationParams,
) => string;

type MessageTree = { [segment: string]: string | MessageTree };

/** Turns { a: { b: "x" } } into { "a.b": "x" }. */
function flatten(tree: MessageTree, prefix = '', out: Record<string, string> = {}) {
  for (const [segment, value] of Object.entries(tree)) {
    const key = prefix ? `${prefix}.${segment}` : segment;
    if (typeof value === 'string') {
      out[key] = value;
    } else {
      flatten(value, key, out);
    }
  }
  return out;
}

export const dictionaries: Record<Locale, Record<string, string>> = {
  en: flatten(enDictionary as MessageTree),
  vi: flatten(viDictionary as MessageTree),
};

/**
 * Warnings are printed once per distinct message and only outside production.
 * A missing key is a bug to fix in review, not something to spam an operator's
 * console with on every render.
 */
const alreadyWarned = new Set<string>();

function warnOnce(message: string): void {
  if (process.env.NODE_ENV === 'production') return;
  if (alreadyWarned.has(message)) return;
  alreadyWarned.add(message);
  // eslint-disable-next-line no-console
  console.warn(`[VKAI Panel i18n] ${message}`);
}

const PLACEHOLDER = /\{([A-Za-z0-9_]+)\}/g;

/**
 * Substitutes {name} placeholders. A placeholder with no value is left in the
 * output exactly as written - visible in review, and traceable in production -
 * rather than collapsed to nothing.
 */
function interpolate(template: string, key: string, params?: TranslationParams): string {
  if (!template.includes('{')) return template;
  return template.replace(PLACEHOLDER, (placeholder, name: string) => {
    const value = params?.[name];
    if (value === undefined || value === null) {
      warnOnce(`key "${key}" has no value for {${name}}`);
      return placeholder;
    }
    return String(value);
  });
}

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (LOCALES as readonly string[]).includes(value);
}

/**
 * Looks a key up and fills in its placeholders.
 *
 * A key missing from the requested locale falls back to the English source; a
 * key missing everywhere returns the key itself. Neither case ever returns an
 * empty string: an empty label is invisible in review and obvious in
 * production, and the key at least says which string is missing.
 */
export function translate(
  locale: Locale,
  key: string,
  params?: TranslationParams,
): string {
  const table = dictionaries[locale] ?? dictionaries[SOURCE_LOCALE];

  let template: string | undefined = table[key];

  if (template === undefined && locale !== SOURCE_LOCALE) {
    template = dictionaries[SOURCE_LOCALE][key];
    if (template !== undefined) {
      warnOnce(`key "${key}" is missing from ${locale}.json; showing the English source`);
    }
  }

  if (template === undefined) {
    warnOnce(`key "${key}" is missing from every dictionary`);
    return key;
  }

  return interpolate(template, key, params);
}

/**
 * Picks between "<key>.one" and "<key>.other" on `count`, and passes `count`
 * through as a parameter so the message can print it.
 *
 * THE RULE IS: count === 1 chooses "one", everything else chooses "other".
 * That is exactly the CLDR plural rule for English, and Vietnamese has a
 * single form, so both entries in a Vietnamese message are simply the same
 * sentence. It is right for these two locales and for no others - a locale
 * with "few"/"many" (Polish, Arabic, Russian) needs Intl.PluralRules here and
 * new branches in every plural key, so add it deliberately rather than
 * assuming this function already handles it.
 */
export function translatePlural(
  locale: Locale,
  key: string,
  count: number,
  params?: TranslationParams,
): string {
  const form = count === 1 ? 'one' : 'other';
  return translate(locale, `${key}.${form}`, { count, ...params });
}

/**
 * The English source text for a key.
 *
 * For code that runs outside a React render and needs a developer-readable
 * string - the `message` of a thrown Error, a console line - while the UI
 * translates the same key for the operator.
 */
export function sourceText(key: string, params?: TranslationParams): string {
  return translate(SOURCE_LOCALE, key, params);
}

/** The operator's explicit choice, or null. Browser only. */
export function storedLocale(): Locale | null {
  if (typeof window === 'undefined') return null;
  try {
    const value = window.localStorage.getItem(LOCALE_STORAGE_KEY);
    return isLocale(value) ? value : null;
  } catch {
    // Private mode, or storage blocked by policy.
    return null;
  }
}

/** Persists the operator's choice. Silently does nothing if storage is blocked. */
export function persistLocale(locale: Locale): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  } catch {
    /* storage blocked: the choice still applies for this page load */
  }
}

/**
 * The first supported language in the browser's Accept-Language preference,
 * matched on the primary subtag so that "vi-VN" and "vi" both count.
 */
export function browserLocale(): Locale | null {
  if (typeof navigator === 'undefined') return null;
  const preferences =
    navigator.languages && navigator.languages.length > 0
      ? navigator.languages
      : [navigator.language];

  for (const preference of preferences) {
    const primary = String(preference || '').split('-')[0].toLowerCase();
    if (isLocale(primary)) return primary;
  }
  return null;
}

/**
 * Locale selection, in order: the operator's saved choice, then the browser's
 * Accept-Language, then English.
 *
 * MUST be called from an effect, never during render: it reads localStorage
 * and navigator, which the server does not have, and a render that disagreed
 * with the server's would break hydration.
 */
export function resolveInitialLocale(): Locale {
  return storedLocale() ?? browserLocale() ?? DEFAULT_LOCALE;
}
