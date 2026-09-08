import { http } from 'msw';
import { jsonError } from './helpers';

/**
 * The four ERP modules are not mocked.
 *
 * Stock ledgers, approval chains, transactional sales and loyalty arithmetic
 * are several thousand lines of rules that the Go backend already implements
 * and tests against a real database. A second implementation here would be the
 * most likely place for the demo and reality to quietly disagree — and a demo
 * that disagrees with the system is worse than one that says it cannot help.
 *
 * So demo mode says exactly that, rather than returning a bare 404 from the
 * dev server. Point NEXT_PUBLIC_API_BASE_URL at the backend and these pages
 * work in full.
 */
export function createErpStubHandlers() {
  const unavailable = (module: string) =>
    jsonError(
      501,
      'NEEDS_BACKEND',
      `${module} runs on the Go backend and is not part of the offline demo. ` +
        'Start the API and set NEXT_PUBLIC_API_BASE_URL to use it.',
    );

  return [
    http.all('*/api/admin/inventory/*', () => unavailable('Stock')),
    http.all('*/api/admin/purchasing/*', () => unavailable('Purchasing')),
    http.all('*/api/admin/pos/*', () => unavailable('The till')),
    http.all('*/api/admin/crm/*', () => unavailable('Loyalty')),
    http.all('*/api/me/loyalty', () => unavailable('Loyalty')),
  ];
}
