/**
 * /audit - kept alive as a redirect.
 *
 * The audit trail is now the "Logs Audit" tab of the Logs page. This route stays
 * so that a bookmark, a link in a runbook or a browser history entry pointing at
 * /audit still lands on the audit trail instead of a 404. It is a redirect
 * rather than a copy of the page: two pages rendering the same trail is two
 * places to fix the next time it changes.
 *
 * redirect() throws, so nothing below it runs and nothing is rendered here.
 */

import { redirect } from 'next/navigation';

export default function AuditRedirectPage() {
  redirect('/logs?tab=audit');
}
