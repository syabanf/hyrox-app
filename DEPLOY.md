# Deploying the demo

Both apps run their entire backend in the visitor's browser as plain
in-process code — the API client dispatches straight into the bundled mock
handlers and seed snapshot. No database, no API server, no service worker,
and therefore no HTTPS requirement and nothing that can fail to register.

## Docker (recommended)

One command brings up the whole stack — member at `/`, admin at `/admin`,
fronted by nginx:

```bash
docker compose up -d --build
# http://<host>:8088         → member app
# http://<host>:8088/admin   → admin panel
```

Put Cloudflare / your TLS proxy in front of port 8088. Pieces:

- `deploy/member.Dockerfile` — Vite build served by nginx (SPA fallback,
  immutable asset caching, no-cache PWA worker).
- `deploy/admin.Dockerfile` — Next.js standalone build (basePath `/admin`
  baked in).
- `deploy/nginx/proxy.conf` — front door; forwards the `/admin` prefix
  WITHOUT stripping it.

After pulling new commits: `docker compose up -d --build` again.

## Without Docker

```bash
pnpm --filter @hyrox/member build   # apps/member/dist → serve at /
pnpm --filter @hyrox/admin build && pnpm --filter @hyrox/admin start
```

Reverse proxy: `location /admin { proxy_pass http://127.0.0.1:3000; }` —
no trailing slash, so the prefix reaches Next intact. The admin's basePath
defaults to `/admin` (override with `NEXT_PUBLIC_BASE_PATH`, empty string
for a domain-root deploy).

## Notes

- Demo state persists in localStorage under `hyrox.mockdb.v<SEED_VERSION>`;
  under one domain the member and admin apps share that origin, so a demo
  reset in one also resets the other after reload.
- Seed data ships as `packages/mock-api/seed-snapshot.json` (committed).
  After changing the seed code, regenerate it with
  `pnpm --filter @hyrox/mock-api dump-seed` and bump `SEED_VERSION`.
