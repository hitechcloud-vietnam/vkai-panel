'use client';

/**
 * "Which installation are we talking about" - the first question three of the
 * six screens have to ask.
 */

import { Field, SelectInput } from './ui';
import type { WordPressSite } from '@/types/wordpress';

export function siteLabel(site: WordPressSite): string {
  const domain = site.domain || '';
  const name = site.name || '';
  if (domain && name && domain !== name) return `${domain} (${name})`;
  return domain || name || site.id;
}

export function SitePicker({
  sites,
  value,
  onChange,
  label = 'Installation',
  hint,
  disabled,
}: {
  sites: WordPressSite[];
  value: string;
  onChange: (id: string) => void;
  label?: string;
  hint?: string;
  disabled?: boolean;
}) {
  return (
    <Field label={label} hint={hint} htmlFor="wp-site-picker">
      <SelectInput
        id="wp-site-picker"
        value={value}
        disabled={disabled || sites.length === 0}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">
          {sites.length === 0 ? 'No WordPress installations are registered' : 'Select an installation'}
        </option>
        {sites.map((site) => (
          <option key={site.id} value={site.id}>
            {siteLabel(site)}
          </option>
        ))}
      </SelectInput>
    </Field>
  );
}

export default SitePicker;
