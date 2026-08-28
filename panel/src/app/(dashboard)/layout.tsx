'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import Sidebar from '@/components/Sidebar';
import Header from '@/components/Header';
import AppFooter from '@/components/AppFooter';
import { useAuthStore } from '@/store/auth';
import { brand } from '@/lib/brand';

/** Placeholder shown while the session is checked: same proportions as the real shell. */
function DashboardSkeleton() {
  return (
    <div className="flex h-screen overflow-hidden bg-[#F7F8FA]">
      <aside className="hidden w-[236px] shrink-0 border-r border-gray-200 bg-white lg:block">
        <div className="flex h-14 items-center border-b border-gray-200 px-4">
          <div className="h-8 w-32 rounded-md bg-gray-100" />
        </div>
        <div className="py-2">
          {Array.from({ length: 12 }).map((_, i) => (
            <div key={i} className="flex h-11 items-center border-b border-gray-100 px-4">
              <div className="h-3.5 w-32 rounded bg-gray-100" />
            </div>
          ))}
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="flex h-14 shrink-0 items-center justify-between border-b border-gray-200 bg-white px-4">
          <div className="h-4 w-56 rounded bg-gray-100" />
          <div className="h-8 w-40 rounded bg-gray-100" />
        </div>

        <div className="flex-1 space-y-5 overflow-y-auto p-5">
          <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
            <div className="h-56 rounded-lg border border-gray-200 bg-white lg:col-span-2" />
            <div className="h-56 rounded-lg border border-gray-200 bg-white" />
          </div>
          <div className="h-24 rounded-lg border border-gray-200 bg-white" />
          <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div className="h-64 rounded-lg border border-gray-200 bg-white" />
            <div className="h-64 rounded-lg border border-gray-200 bg-white" />
          </div>
        </div>

        <div className="h-10 shrink-0 border-t border-gray-200 bg-white" />
      </div>
    </div>
  );
}

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const { isAuthenticated, loadUser } = useAuthStore();
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    let token: string | null = null;
    try {
      token = window.localStorage.getItem('access_token');
    } catch {
      token = null;
    }

    if (!token) {
      router.replace('/login');
      return;
    }

    loadUser()
      .catch((err) => {
        // loadUser already signs the session out; this catch only stops the
        // rejection going unhandled. Console output stays in English: it is
        // read by whoever is debugging, not by the operator.
        console.error(`[${brand.productName}] Could not load the signed-in user:`, err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!loading && !isAuthenticated) {
      router.replace('/login');
    }
  }, [loading, isAuthenticated, router]);

  if (loading || !isAuthenticated) {
    return <DashboardSkeleton />;
  }

  return (
    <div className="flex h-screen overflow-hidden bg-[#F7F8FA]">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <Header />
        {/* Content area, scrolls on its own; #F7F8FA canvas, 20px padding */}
        <main className="min-h-0 flex-1 overflow-y-auto bg-[#F7F8FA] p-5">{children}</main>
        <AppFooter />
      </div>
    </div>
  );
}
