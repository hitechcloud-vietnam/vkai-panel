'use client';

/**
 * The mail server screen moved into /email-server, which now holds the whole
 * mail system rather than half of it.
 *
 * This page stays behind as a redirect so an existing bookmark, a link in a
 * runbook or a support ticket still arrives somewhere useful. It lands on Mail
 * Domain: that is where somebody following an old "mail server" link almost
 * always needs to be, and it is the screen the old page opened on.
 */

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

const TARGET = '/email-server?tab=mail-domain';

export default function MailServerRedirectPage() {
  const router = useRouter();

  useEffect(() => {
    router.replace(TARGET);
  }, [router]);

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
      <h1 className="text-sm font-semibold text-gray-900">This page has moved</h1>
      <p className="mt-1 text-sm text-gray-600">
        The mail server now lives in Email Server, together with mailboxes, the queue and email
        marketing. Taking you there.
      </p>
      <a
        href={TARGET}
        className="mt-3 inline-flex items-center rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
      >
        Go to Email Server
      </a>
    </div>
  );
}
