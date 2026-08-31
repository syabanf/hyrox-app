/**
 * Writes the full seeded database to seed-snapshot.json.
 *
 * The canonical source of truth is src/seed.ts (data is generated fresh in
 * every browser); this snapshot is a frozen, inspectable export of one run —
 * dates are anchored to the moment it was generated.
 *
 * Regenerate with: pnpm --filter @hyrox/mock-api dump-seed
 */
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createSeedDb } from '../src/seed';

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, '..', 'seed-snapshot.json');

const db = createSeedDb(new Date().toISOString());
mkdirSync(dirname(out), { recursive: true });
writeFileSync(out, JSON.stringify(db, null, 1));

const counts = Object.fromEntries(
  Object.entries(db)
    .filter(([, v]) => Array.isArray(v))
    .map(([k, v]) => [k, (v as unknown[]).length]),
);
console.log('Wrote', out);
console.log(counts);
