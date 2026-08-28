'use client';

/**
 * The banner every screen in this section shows when the WP-CLI half of the API
 * is not reachable.
 *
 * It is not decoration. Half of the toolkit - installing, live plugin and theme
 * lists, core updates, staging - is implemented in Go and mounted by exactly one
 * call that nothing makes. Without this banner an operator would see a page of
 * dashes and greyed-out buttons and assume the panel was broken; with it, the
 * missing line is named and the fix is a one-line change somebody can make.
 */

import { RUNTIME_UNMOUNTED_REASON, RUNTIME_UNREACHABLE_REASON } from './api';
import { Button, Notice } from './ui';
import type { RuntimeAvailability } from '@/types/wordpress';

export function RuntimeBanner({
  availability,
  onRecheck,
}: {
  availability: RuntimeAvailability;
  onRecheck?: () => void;
}) {
  if (availability === 'available' || availability === 'checking') return null;

  if (availability === 'unreachable') {
    return (
      <Notice tone="amber" title="The panel API did not answer">
        <p>{RUNTIME_UNREACHABLE_REASON}</p>
        {onRecheck ? (
          <div className="mt-2">
            <Button tone="secondary" onClick={onRecheck}>
              Check again
            </Button>
          </div>
        ) : null}
      </Notice>
    );
  }

  return (
    <Notice tone="amber" title="Live WordPress operations are switched off on this panel">
      <p>{RUNTIME_UNMOUNTED_REASON}</p>
      <p className="mt-2">
        Until that call is added, this section can read and edit the records the panel keeps, and
        nothing more. Every control that needs WP-CLI is disabled and says so when you hover it,
        rather than failing after you press it.
      </p>
      {onRecheck ? (
        <div className="mt-2">
          <Button tone="secondary" onClick={onRecheck}>
            Check again
          </Button>
        </div>
      ) : null}
    </Notice>
  );
}

export default RuntimeBanner;
