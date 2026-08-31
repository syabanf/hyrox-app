import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
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
