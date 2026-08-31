# Seed data

The canonical seed lives in `src/seed.ts` — every browser generates a fresh,
deterministic database from it at boot (dates anchored to boot time,
localStorage key `hyrox.mockdb.v<SEED_VERSION>`).

`seed-snapshot.json` is a frozen export of one full run, committed so the
demo data itself is inspectable in the repo without running the apps.
Regenerate it after seed changes with:

```bash
pnpm --filter @hyrox/mock-api dump-seed
```
