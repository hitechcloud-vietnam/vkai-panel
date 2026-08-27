import type { Metadata } from 'next';
import '@/styles/globals.css';

export const metadata: Metadata = {
  title: 'vKAI Panel - HiTechCloud Server Management',
  description: 'Enterprise multi-server hosting & web control panel',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-dark-950 text-dark-50">
        {children}
      </body>
    </html>
  );
}
