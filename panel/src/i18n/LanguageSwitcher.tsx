'use client';

/**
 * The operator's language control. Mounted in the application header, and
 * again on the sign-in screen, which has no header to carry it.
 *
 * A native <select> on purpose: it is keyboard and screen-reader correct
 * without a line of code, and the switch has to work on the sign-in screen and
 * inside a failed page alike. Choosing applies immediately - React re-renders
 * from context, nothing reloads.
 *
 * Each option is labelled "<CODE> · <language name>". The code is there so a
 * Vietnamese operator who lands on the English default can find the row
 * without reading the English word for their own language.
 */

import { useId } from 'react';
import { Languages } from 'lucide-react';
import { useI18n } from './I18nProvider';
import { isLocale } from './core';

export default function LanguageSwitcher({ className = '' }: { className?: string }) {
  const { locale, locales, setLocale, t } = useI18n();
  // Generated, not fixed: the switch is mounted in the header and again on the
  // sign-in screen, and a duplicate id would break the label association.
  const selectId = useId();

  return (
    <div className={`relative ${className}`}>
      <label htmlFor={selectId} className="sr-only">
        {t('common.language')}
      </label>
      <Languages
        size={15}
        className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-gray-400"
        aria-hidden
      />
      <select
        id={selectId}
        value={locale}
        title={t('common.language')}
        onChange={(event) => {
          const next = event.target.value;
          if (isLocale(next)) setLocale(next);
        }}
        className="appearance-none rounded-md border border-gray-300 bg-white py-1.5 pl-7 pr-2 text-sm text-gray-700 hover:bg-gray-50 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
      >
        {locales.map((code) => (
          <option key={code} value={code}>
            {`${code.toUpperCase()} · ${t(`common.languageName.${code}`)}`}
          </option>
        ))}
      </select>
    </div>
  );
}
