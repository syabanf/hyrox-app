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

## Admin panel (Next.js) — always under /admin

The basePath is baked in: routes, assets, and `mockServiceWorker.js` all live
under `/admin`, in dev and prod alike. A plain build deploys correctly:

```bash
pnpm --filter @hyrox/admin build
pnpm --filter @hyrox/admin start       # serves http://host:3000/admin
```

Point the reverse proxy at the app for the whole prefix, WITHOUT stripping it:

```nginx
location /admin { proxy_pass http://127.0.0.1:3000; }
```

(If the proxy strips the prefix — `proxy_pass http://…/;` with a trailing
slash — routes 404 and the service worker registers at the wrong scope: the
classic "Starting mock backend…" hang. The boot screen now surfaces the real
error + a retry button instead of hanging.) To deploy at the domain root
instead, build with `NEXT_PUBLIC_BASE_PATH=""`.

Note: the two apps keep separate demo databases (per-origin localStorage).
