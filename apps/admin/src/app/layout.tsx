import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import { Providers } from '../lib/providers';
import './globals.css';

export const metadata: Metadata = {
  title: 'NüHabit Admin',
  description: 'Operations dashboard for NüHabit',
  icons: { icon: '/admin/favicon.png' },
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        {/* React 19 hoists these into <head>. */}
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Outfit:wght@400..900&family=Manrope:wght@300..800&display=swap"
        />
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
