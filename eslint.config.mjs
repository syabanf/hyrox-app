import js from '@eslint/js';
import tseslint from 'typescript-eslint';

/**
 * Clean-architecture layer boundaries, enforced per directory:
 *   domain        → nothing
 *   application   → domain
 *   contracts     → zod, domain (types)
 *   api-client    → contracts, domain (types)
 *   mock-api      → domain, application, contracts, msw, faker
 *   apps          → api-client, contracts, ui, domain (types/pure helpers),
 *                   mock-api only for MSW bootstrap
 */
export default tseslint.config(
  {
    ignores: [
      '**/node_modules/**',
      '**/dist/**',
      '**/.next/**',
      '**/.turbo/**',
      '**/dev-dist/**',
      '**/next-env.d.ts',
      '**/mockServiceWorker.js',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      '@typescript-eslint/consistent-type-imports': 'error',
    },
  },
  {
    files: ['**/*.test.ts'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },
  {
    files: ['packages/domain/src/**/*.ts'],
    rules: {
      'no-restricted-imports': [
        'error',
        { patterns: [{ group: ['@hyrox/*', 'zod', 'msw', 'react*'], message: 'domain depends on nothing.' }] },
      ],
    },
  },
  {
    files: ['packages/application/src/**/*.ts'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['@hyrox/*', '!@hyrox/domain'],
              message: 'application may only import @hyrox/domain.',
            },
          ],
        },
      ],
    },
  },
  {
    files: ['packages/api-client/src/**/*.ts'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['@hyrox/*', '!@hyrox/contracts', '!@hyrox/domain'],
              message: 'api-client may only import @hyrox/contracts (and domain types).',
            },
          ],
        },
      ],
    },
  },
  {
    files: ['apps/**/src/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['@hyrox/application', '@hyrox/application/*'],
              message: 'apps must go through the API client, not use cases directly.',
            },
          ],
        },
      ],
    },
  },
);
