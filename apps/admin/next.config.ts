import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  // Deploying under a subpath (e.g. hyrox.example.com/admin):
  //   NEXT_PUBLIC_BASE_PATH=/admin pnpm --filter @hyrox/admin build
  // Routes, assets, and public files (incl. mockServiceWorker.js) all move
  // under the prefix, and the MSW bootstrap reads the same env var.
  basePath: process.env.NEXT_PUBLIC_BASE_PATH || undefined,
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
