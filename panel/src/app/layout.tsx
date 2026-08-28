import type { Metadata, Viewport } from 'next';
import { Inter } from 'next/font/google';
import { brand, fullName, description as brandDescription } from '@/lib/brand';
import { I18nProvider } from '@/i18n';
import '@/styles/globals.css';

const inter = Inter({
  subsets: ['latin', 'vietnamese'],
  display: 'swap',
  variable: '--font-inter',
  fallback: ['system-ui', 'Segoe UI', 'Arial', 'sans-serif'],
});

export const metadata: Metadata = {
  title: {
    default: fullName,
    template: `%s · ${brand.productName}`,
  },
  description: brandDescription,
  applicationName: brand.productName,
  generator: brand.productName,
  authors: [{ name: brand.company, url: brand.companyUrl }],
  publisher: brand.company,
  icons: {
    icon: [{ url: '/favicon.svg', type: 'image/svg+xml' }],
    shortcut: [{ url: '/favicon.svg', type: 'image/svg+xml' }],
    apple: [{ url: '/favicon.svg', type: 'image/svg+xml' }],
  },
  robots: {
    index: false,
    follow: false,
  },
};

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  themeColor: brand.colors.navy,
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    // lang is the source locale here because a server component cannot know
    // the operator's choice; I18nProvider rewrites it on the client as soon as
    // the stored choice and the browser preference have been read.
    <html lang="en" className={inter.variable}>
      <body className="min-h-screen bg-[#F7F8FA] text-gray-900 antialiased">
        <I18nProvider>{children}</I18nProvider>
      </body>
    </html>
  );
}
