'use client';

/**
 * Email marketing moved into /email-server, alongside the mail server it sends
 * through. Kept as a redirect so old links still work; see the note on
 * /mail-server for the reasoning.
 */

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

const TARGET = '/email-server?tab=mail-marketing&sub=overview';

export default function EmailMarketingRedirectPage() {
  const router = useRouter();

  useEffect(() => {
    router.replace(TARGET);
  }, [router]);

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
      <h1 className="text-sm font-semibold text-gray-900">This page has moved</h1>
      <p className="mt-1 text-sm text-gray-600">
        Email marketing is now the first tab of Email Server, next to the mail domains and mailboxes
        it depends on. Taking you there.
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
