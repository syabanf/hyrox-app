import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  // The admin always lives under /admin (dev and prod alike), so a plain
  // `next build` deploys correctly behind `location /admin { proxy_pass ... }`.
  // Override with NEXT_PUBLIC_BASE_PATH ('' for domain root) if ever needed.
  basePath: process.env.NEXT_PUBLIC_BASE_PATH ?? '/admin',
  async redirects() {
    return [
      { source: '/', destination: '/admin', basePath: false as const, permanent: false },
    ];
  },
  transpilePackages: [
    '@hyrox/domain',
    '@hyrox/application',
    '@hyrox/contracts',
    '@hyrox/api-client',
    '@hyrox/mock-api',
    '@hyrox/ui',
  ],
};

export default nextConfig;
