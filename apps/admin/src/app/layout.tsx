import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import { Providers } from '../lib/providers';
import './globals.css';

export const metadata: Metadata = {
  title: 'HYROX Studio Admin',
  description: 'Operations dashboard for HYROX Studio',
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
          href="https://fonts.googleapis.com/css2?family=Archivo:wdth,wght@62..125,100..900&family=Manrope:wght@400..800&display=swap"
        />
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
