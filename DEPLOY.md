# Deploying the demo

Both apps run their entire backend in the visitor's browser (MSW service
worker) — no database, no API server. Two non-negotiables:

1. **HTTPS (or localhost).** Service workers refuse to register otherwise.
2. **`mockServiceWorker.js` must be reachable** (the boot screen shows the
   real error + a Retry button if it isn't).

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
  immutable asset caching, no-cache service workers).
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

- The two apps keep separate demo databases (per-origin localStorage — and
  under one domain, separate localStorage keys are still shared per origin;
  member and admin use different storage keys so they don't collide).
- The admin registers its mock worker at `/admin/mockServiceWorker.js` and
  falls back to the root copy (served by the member app) for embedded
  browsers that refuse subpath-hosted service-worker scripts.
