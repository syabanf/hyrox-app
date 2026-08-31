# Deploying the demo (frontend-only)

Both apps are static-friendly and run their entire backend in the browser
(MSW service worker), so any static host / reverse proxy works — but two
things are non-negotiable:

1. **HTTPS (or localhost).** Service workers refuse to register otherwise —
   the app will show "Mock backend failed to start".
2. **`mockServiceWorker.js` must be reachable at the app's own base path.**

## Member app (Vite PWA) — served at the domain root

```bash
pnpm --filter @hyrox/member build      # outputs apps/member/dist
```

Serve `dist/` at `/`. For a subpath instead, build with
`vite build --base=/app/` and serve under `/app/`.

## Admin panel (Next.js) — e.g. under /admin

```bash
NEXT_PUBLIC_BASE_PATH=/admin pnpm --filter @hyrox/admin build
pnpm --filter @hyrox/admin start       # or deploy the .next output to a Node host
```

`NEXT_PUBLIC_BASE_PATH` moves routes, assets, and `mockServiceWorker.js`
under the prefix AND tells the MSW bootstrap where to register the worker.
Point the reverse proxy at the app for the whole `/admin` prefix:

```nginx
location /admin { proxy_pass http://127.0.0.1:3000; }
```

Without the env var the admin registers `/mockServiceWorker.js` at the domain
root, which a subpath deploy cannot serve — the exact "Starting mock
backend…" hang. The boot screen now surfaces the real error + a retry button
instead of hanging.

Note: the two apps keep separate demo databases (per-origin localStorage).
