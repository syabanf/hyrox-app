import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  // The admin always lives under /admin (dev and prod alike), so a plain
  // `next build` deploys correctly behind `location /admin { proxy_pass ... }`.
  // Override with NEXT_PUBLIC_BASE_PATH ('' for domain root) if ever needed.
  basePath: process.env.NEXT_PUBLIC_BASE_PATH ?? '/admin',
  // Docker runs the standalone output (deploy/admin.Dockerfile).
  output: 'standalone',
  async redirects() {
    return [
      { source: '/', destination: '/admin', basePath: false as const, permanent: false },
    ];
  },
  // The panel calls /api on its own origin, so the browser never makes a
  // cross-origin request and CORS never enters the picture. In production
  // nginx routes /api to the Go backend before Next ever sees it; in
  // development this rewrite does the same job, so `pnpm dev` talks to a
  // local backend with nothing else to configure.
  async rewrites() {
    const target = process.env.API_PROXY_TARGET ?? 'http://localhost:8080';
    return [
      {
        source: '/api/:path*',
        destination: `${target}/api/:path*`,
        basePath: false as const,
      },
    ];
  },
  transpilePackages: [
    '@nuhabit/domain',
    '@nuhabit/application',
    '@nuhabit/contracts',
    '@nuhabit/api-client',
    '@nuhabit/mock-api',
    '@nuhabit/ui',
  ],
};

export default nextConfig;
