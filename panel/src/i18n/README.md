# Translation layer

English is the SOURCE language. Every string the interface ships starts life in
`en.json`; `vi.json` is a translation of it. Both files must hold the same set
of keys - a key missing from `vi.json` falls back to the English source and
warns in development.

## Using it

```tsx
'use client';
import { useT } from '@/i18n';

export default function Panel() {
  const t = useT();
  return <h1>{t('servers.title')}</h1>;
}
```

Import from `@/i18n` only. Everything public is re-exported from `index.ts`.

| Export | What it gives you |
| --- | --- |
| `useT()` | `t(key, params?)` |
| `useTn()` | `tn(key, count, params?)` - see *Plurals* |
| `useLocale()` | `{ locale, locales, setLocale, ready }` |
| `useFormatters()` | `{ formatNumber, formatDate, formatDateTime }` |
| `useI18n()` | all of the above in one object |
| `LanguageSwitcher` | the language `<select>` |
| `translate(locale, key, params?)` | non-React lookup |
| `sourceText(key, params?)` | the English text, for `Error.message` and console lines |
| `EMPTY_VALUE` | `—`, for a value the API genuinely does not supply |

## Key naming

`area.thing`, lower camelCase leaves, dotted namespaces:

- `common.*` shared vocabulary: actions, `documentation`, `support`, `copyright`
- `nav.*` sidebar entries, `nav.group.*` for the group headings
- `sidebar.*`, `header.*`, `footer.*` the application shell
- `auth.*` sign-in and two-factor
- `errors.*` the error, 404 and global-error screens
- `dashboard.*`, `servers.*`, `websites.*`, ... one namespace per feature area

Reuse a `common.*` key rather than adding `servers.cancel`. Add a feature key
when the wording is specific to that screen.

## Interpolation

Single braces: `"Loading {product}…"`, called as
`t('common.loadingProduct', { product: brand.productName })`. A placeholder
with no matching parameter is left in the output verbatim and warns in
development - it is never blanked.

## Missing keys

`t('nope.at.all')` returns the string `nope.at.all` and warns once, in
development only. It never returns an empty string: an empty label is invisible
in review and obvious in production.

## Plurals - what is supported

`tn(key, count)` chooses between `key.one` and `key.other` on `count === 1`,
and passes `count` in as a parameter:

```json
"selectedCount": { "one": "{count} item selected", "other": "{count} items selected" }
```

That rule is exactly right for English, and Vietnamese has one form (both
entries carry the same sentence). It is right for nothing else. A locale with
`few`/`many` needs `Intl.PluralRules` in `core.ts` and new branches in every
plural key - do that deliberately rather than assuming it already works.

## Dates and numbers - what is supported

`Intl.NumberFormat` and `Intl.DateTimeFormat` under `en-US` / `vi-VN`: plain
numbers, a medium date, and a medium date with a short time, all in the
BROWSER's time zone. Nothing else - no relative time, no calendars. Do not
print a formatted date from a component's first server render; see the header
comment in `format.ts`.

## Locale selection

`localStorage['vkai_locale']` -> the browser's `Accept-Language` (matched on
the primary subtag, so `vi-VN` counts as `vi`) -> English.

Both of those are read in an effect, never during render, so the server's
markup and the browser's first render always agree. That costs one render at
the default locale before the operator's choice appears; `useLocale().ready`
tells you when the choice has landed.

`<html lang>` is `en` in `app/layout.tsx` because a server component cannot
know the choice; the provider rewrites it on the client.

## The one place that cannot use the hooks

`app/global-error.tsx` replaces the root layout, so it renders outside the
provider. It resolves the locale itself and calls `translate` directly.
