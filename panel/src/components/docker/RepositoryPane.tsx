'use client';

/**
 * Repository: the registries this host can pull from and push to, and the
 * credentials for each.
 *
 * Nothing exists behind this tab - no route, no model, no storage. That is
 * fortunate rather than unfortunate, because a registry credential is a secret,
 * and a half-built version of this screen is how secrets end up readable.
 *
 * The rule for whoever builds it is written into the backlog below and is not
 * negotiable: the credential is write-only. It goes in on create and on
 * replace, and it never comes back out of the API - not to render a masked
 * field, not "just for the edit form". The panel shows the registry URL, the
 * namespace, the username and when the credential was last changed. It does not
 * show the credential, because a screen that can display a password is a screen
 * that can leak one.
 */

import { PaneUnavailable } from './CapabilityNotice';
import type { CapabilityGap } from '@/types/docker';

const GAPS: CapabilityGap[] = [
  {
    label: 'Store a registry',
    missing:
      'No routes and no model. There is no /api/v1/docker/registries group in core/internal/handler/router.go and no registry table in the schema.',
  },
  {
    label: 'Write-only credentials',
    missing:
      'When the model is added, the credential field must be accepted on write and omitted from every read response, and it must be encrypted at rest rather than stored as sent. The list response should carry only url, namespace, username and a last-updated timestamp.',
  },
  {
    label: 'Log in to a registry',
    missing:
      'Nothing performs a `docker login`, so a stored credential could not be verified before an operator relies on it for a pull.',
  },
  {
    label: 'Use a registry in a pull or push',
    missing:
      'POST /api/v1/docker/images/pull has no registry field, and there is no push route at all, so a configured registry would have nothing to be used by.',
  },
];

const WOULD_SHOW = [
  'Each configured registry with its URL, namespace and username',
  'When its credential was last changed, and by whom',
  'Whether the last login attempt against it succeeded',
  'A replace-credential action that accepts a new secret and never displays the old one',
];

export function RepositoryPane() {
  return (
    <PaneUnavailable
      title="Registries are not stored by the panel yet"
      summary={
        <>
          This tab holds the registries an operator pulls private images from, and the
          credential for each. The API has no route and no model for either. No form is
          shown, because a form that accepts a registry password and drops it on the floor
          is worse than no form: the operator believes the credential is saved and finds
          out during a deploy that it never was.
        </>
      }
      wouldShow={WOULD_SHOW}
      gaps={GAPS}
    />
  );
}

export default RepositoryPane;
