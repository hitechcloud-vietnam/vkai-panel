import type { Metadata } from 'next';
import TwoFactorSettings from '@/components/settings/TwoFactorSettings';

export const metadata: Metadata = {
  title: 'Two-factor authentication',
};

export default function TwoFactorPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Two-factor authentication</h1>
        <p className="mt-1 text-sm text-gray-600">
          Protect this account with a code from your phone. The panel controls every website and
          server it manages, so a password on its own is a single point of failure.
        </p>
      </div>

      <TwoFactorSettings />

      <div className="rounded-lg border border-gray-200 bg-white p-5 shadow-sm">
        <h2 className="text-sm font-semibold text-gray-900">If you lose your phone</h2>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-gray-600">
          <li>Sign in with one of your recovery codes. Each one works a single time.</li>
          <li>Then issue a fresh set of codes and enrol your new device.</li>
          <li>
            With no phone and no codes left, an administrator has to reset the second factor for
            you. That reset is verified out of band and recorded in the audit log.
          </li>
        </ul>
      </div>
    </div>
  );
}
