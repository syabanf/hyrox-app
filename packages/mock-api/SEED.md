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

Two things to know when editing the seed:

- **No open gym.** Every gate entry is a class check-in, so every ALLOWED /
  SYNCED access log is tied to a booking and the OFFLINE CONFLICT row has a
  CONFIRMED booking on the class it belongs to (approving it deducts that class).
- **`ses_live`** is a Senopati class that starts 20 minutes after boot with
  the demo member confirmed on it, so "scan my QR" is ALLOWED immediately. The
  browser bootstrap (`src/browser/in-process.ts`) re-anchors this one session
  to load time when it restores the snapshot.
