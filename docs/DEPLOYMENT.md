# Deploying NüHabit

Everything needed to put this on a server and keep it there: what it is made
of, what to configure, how to get the first person signed in, how to upgrade,
and how to get the data back when something goes wrong.

Read **[Before you take real money](#before-you-take-real-money)** first. There
is one thing in here that is not finished, and it is the one that costs money.

---

## What you are deploying

Five containers behind one origin:

```
                    ┌─────────────────────────────────┐
   :443/:80  ──────▶│  proxy (nginx)                  │
                    │    /        → member PWA        │
                    │    /admin   → admin panel       │
                    │    /api     → Go API            │
                    └──────┬──────────┬───────────┬───┘
                           │          │           │
                     ┌─────▼───┐ ┌────▼────┐ ┌────▼─────┐
                     │ member  │ │  admin  │ │   api    │
                     │ (nginx) │ │ (Next)  │ │  (Go)    │
                     └─────────┘ └─────────┘ └────┬─────┘
                                                  │
                                            ┌─────▼──────┐
                                            │ postgres   │
                                            └────────────┘
```

One origin is a deliberate choice, not a convenience: the browser never makes
a cross-origin request, so there is no CORS configuration to get wrong and no
preflight to debug at three in the morning.

The API is a modular monolith. One binary mounts every module by default; the
same binary mounts a subset when you set `MODULES`, which is how it splits into
services later without a rewrite. See [Splitting the API](#splitting-the-api).

**Images.** `apps/backend/Dockerfile` builds the API on `scratch` (a static Go
binary, ~30 MB). `deploy/member.Dockerfile` builds the PWA and serves it from
nginx. `deploy/admin.Dockerfile` builds the Next.js standalone output. All
three are built by `docker compose`.

---

## Prerequisites

- Docker Engine 24+ with the Compose plugin
- 2 GB RAM and 10 GB disk for a single-studio installation
- A hostname pointing at the server, and TLS in front (see [TLS](#tls))
- PostgreSQL 16 — the stack brings its own, or point `DATABASE_URL` at a
  managed instance and delete the `postgres` service

Check what Docker has room for before you start; a full disk fails a build
halfway with an error that does not mention disk:

```bash
docker system df
```

---

## Configuration

Every setting is an environment variable with a development default, so an
empty file boots. Production is stricter, and the server refuses to start
rather than run half-configured — that is the point of the checks below.

### Required in production

| Variable | Why |
|---|---|
| `APP_ENV=production` | Turns on every check in this table. Without it, none of them apply. |
| `AUTH_SECRET` | Signs every bearer token. Refused if left at `dev-secret-change-me`. Generate with `openssl rand -base64 48`. |
| `AUTH_DEMO_OTP=false` | Demo mode accepts **any** well-formed OTP and lets the login screen sign in from a role card without a password. Refused if true in production. |
| `DATABASE_URL` | With `sslmode=require` for anything not on the same host. |
| `PAYMENTS_ALLOW_MOCK=true` | An acknowledgement, not a feature. See [Before you take real money](#before-you-take-real-money). |

Changing `AUTH_SECRET` invalidates every issued token, signing everybody out.
That is the intended way to revoke sessions in a hurry.

### Worth setting

| Variable | Default | Notes |
|---|---|---|
| `HTTP_ALLOWED_ORIGINS` | `*` | Narrow to your own hostname. Behind the bundled proxy nothing is cross-origin, so this only matters if you serve the apps from somewhere else. |
| `STUDIO_TIMEZONE` | `Asia/Jakarta` | Reports, payroll periods and "today" resolve against this, never against UTC. |
| `AUTH_TOKEN_TTL` | `720h` (30 days) | Members stay signed in on their phones; shorten it if that is not acceptable. |
| `AUTH_LOGIN_ATTEMPTS` | `10` | Wrong staff passwords before the account locks. |
| `AUTH_LOCKOUT_FOR` | `15m` | How long it stays locked. |
| `LOG_FORMAT` | `json` | Leave as JSON so a log shipper can parse it. |
| `LOG_LEVEL` | `info` | |
| `DATABASE_MAX_CONNS` | `20` | Per API instance. Multiply by your replica count and keep it under Postgres's `max_connections`. |
| `HTTP_SHUTDOWN_TIMEOUT` | `15s` | How long in-flight requests get to finish on deploy. |
| `AUTO_MIGRATE` | unset (= on) | The API applies pending migrations at boot under an advisory lock, so several replicas starting together is safe. Set `false` on all but one if you would rather migrate deliberately. |

### Leave alone unless you know why

`AUTH_PASSWORD_ITERATIONS` (210 000) is the PBKDF2 cost for staff passwords. It
is configurable so a test run does not spend half a second hashing a demo
roster; production refuses anything lower. `AUTH_OTP_LENGTH`, `AUTH_OTP_TTL`
and `PAYMENTS_INVOICE_TTL` have sensible defaults.

`INSTAGRAM_VERIFY_TOKEN` and `INSTAGRAM_APP_SECRET` are empty by default, and
an unconfigured social webhook **refuses every delivery**. That is the safe
state: a publicly reachable endpoint that writes to the support inbox without
checking a signature is an open door.

### Where to put it

Create `.env` beside `docker-compose.yml`. It is gitignored.

```bash
# .env — never commit this
APP_ENV=production
AUTH_SECRET=<openssl rand -base64 48>
AUTH_DEMO_OTP=false
PAYMENTS_ALLOW_MOCK=true
HTTP_ALLOWED_ORIGINS=https://studio.example
STUDIO_TIMEZONE=Asia/Jakarta
POSTGRES_PASSWORD=<openssl rand -base64 24>
```

The bundled `docker-compose.yml` hard-codes development database credentials
and `APP_ENV=development`; it is the demo stack. For production, copy it and
replace the `environment:` blocks with `env_file: .env`, or run
`docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d`.

---

## First deployment

### 1. Get the code

```bash
git clone https://github.com/syabanf/hyrox-app.git
cd hyrox-app
```

### 2. Write the configuration

Create `.env` as above. Check it before continuing — every mistake here shows
up as a container that will not start, which is better than one that starts
wrong, but still costs you a round trip.

### 3. Build and start

```bash
docker compose up -d --build
```

The first build takes several minutes: two Node builds and a Go build. After
that, layers are cached and only what changed rebuilds.

Services start in dependency order, each waiting for the one below it to
report healthy — the API waits for the database, the front door waits for all
three things it proxies to. A cold start therefore serves its first request
rather than a 502.

```bash
docker compose ps        # every service should read "healthy"
```

### 4. Apply the schema

The API applies pending migrations at boot, so by the time it reports healthy
the schema is current. To do it deliberately instead, set `AUTO_MIGRATE=false`
and run:

```bash
docker compose run --rm --entrypoint /usr/local/bin/migrate api
docker compose run --rm --entrypoint /usr/local/bin/migrate api status
```

Migrations are forward-only and run inside an advisory lock. There is no
`down`: rolling back a schema is how you lose the data the new schema was
holding. To undo one, write the next one.

### 5. Create the first staff account

**This step is not optional and nothing else replaces it.** A correctly
configured production deployment has no way in: every route that creates a
staff account requires a caller who already has one, demo mode is off, and the
seeder refuses to touch a production database.

```bash
docker compose run --rm \
  -e BOOTSTRAP_PASSWORD='a-long-passphrase-you-chose' \
  --entrypoint /usr/local/bin/bootstrap api \
  -name "Alya Santoso" -email alya@studio.example
```

It creates one Super Admin and then refuses to run again while any staff
account exists — so it cannot be used later to quietly mint a second one. Make
the rest from the panel, where the audit trail records who created whom.

The password comes from the environment or stdin, never a flag: a flag lands
in the shell history and in `ps` output, which is the wrong place for the first
credential on a new system. Minimum ten characters — length is the part that
costs an attacker something.

### 6. Sign in

Open `https://studio.example/admin`, sign in with that email and password.
There will be no role cards: those only appear when the server reports demo
mode, which production does not.

Then, in order:

1. **Config → Users** — create accounts for everybody else. Setting somebody's
   password marks it for replacement at their first sign-in, because a
   credential two people know is not a credential.
2. **Config → Business Rules** — credit expiry, cancellation deadline, no-show
   policy, QR TTL, anti-passback window. These are data, not code; the gate
   honours a change immediately.
3. **Config → Branches & Gates** — your real sites and door readers.
4. **Operations → Class Types**, then **Coaches**, then **Coach Incentives →
   Schemes** for what each coach is paid.
5. **Commercial → Credit Packages** — what members can buy.

---

## Seeding

There are two completely different things called seeding here. Do not confuse
them.

### The demo studio — never in production

`cmd/seed` loads a fictional studio: two branches, nine members, a fortnight of
classes, stock, suppliers, a POS catalogue, GPS activities, and six staff
accounts that all share the password `nuhabit-demo-2026`.

```bash
docker compose --profile seed run --rm seed      # or: pnpm stack:up
```

It refuses to run when `APP_ENV=production` unless you set
`SEED_ALLOW_PRODUCTION=true`. **Do not set it.** Those six accounts have a
published password and full permissions. The guard is the last thing standing
between a demo and a production database with a public login.

Use it on a staging deployment, where it is genuinely useful: it produces a
studio complete enough to click through every screen.

### Your real opening data

There is no bulk importer for members. Real data goes in through the panel and
the CSV imports the ERP screens already have:

| What | Where |
|---|---|
| Suppliers | Purchasing → Suppliers → Import |
| Stock items | Stock → Items → Import |

Each is a two-step: **Check it** dry-runs the file and reports what would be
created, updated and rejected, by line number; **Import it** applies it.
Nothing is written by the dry run.

Supplier price lists have an importer on the API
(`POST /api/admin/purchasing/suppliers/{id}/prices/import?apply=`) but no
button in the panel yet, so that one is a `curl` for now.

Everything else — branches, gates, class types, coaches, packages, vouchers,
business rules — is a form in the admin panel, and there are not enough of any
of them to want a script.

Members register themselves through the app, or the front desk creates them at
**Members → New member**.

---

## Before you take real money

**No payment gateway is implemented.** Only a mock exists, and it settles every
invoice it issues without any money moving.

The server will not start in production until you acknowledge this:

```
PAYMENTS_ALLOW_MOCK=true
```

Setting it is a statement that **top-ups are taken at the desk and recorded by
staff** — cash, transfer, a card terminal that is not this system — and that no
member is ever shown a payment link that appears to charge them.

If you put the mock in front of members, every one of them can top up their
wallet for free, and the credit ledger will agree with them. There is no
recovery from that beyond reversing entries by hand.

Wiring a real gateway means implementing `wallet.Gateway` (three methods:
`Name`, `CreateInvoice`, `VerifyCallback`) in `apps/backend/internal/modules/
wallet/gateway.go` and selecting it in `internal/app/app.go`. The callback
verification is the part that matters: an unauthenticated webhook that credits
a wallet is a way to print credits.

`PAYMENTS_PROVIDER` is refused for any value but `mock`, so a half-finished
integration cannot silently fall back to settling its own payments.

---

## TLS

The bundled proxy speaks HTTP on port 80 and is meant to sit behind something
that terminates TLS. Two ways:

**A reverse proxy in front** (Caddy, Traefik, or nginx on the host) holding the
certificate and forwarding to `127.0.0.1:8088`. Simplest, and certificate
renewal stays outside the stack.

**Terminate in the bundled proxy** by mounting your certificate and adding a
TLS server block to `deploy/nginx/proxy.conf`.

Either way, pass the scheme through: the proxy already sets
`X-Forwarded-Proto`, and the apps build absolute URLs from it.

Serve the member app over HTTPS or it will not work properly — the PWA's
service worker and the QR screen both require a secure context.

---

## Upgrading

```bash
git pull
docker compose up -d --build
```

Compose replaces containers whose image changed and leaves the rest. The API
applies pending migrations as it starts.

Two things to know:

**Migrations run before the new code serves traffic**, but during a rolling
replacement the old container may briefly see the new schema. Every migration
here is additive for that reason — new columns are nullable or defaulted, and
nothing is dropped in the same release that stops using it.

**The frontends bake their configuration in at build time.** `docker compose
up --build` rebuilds them, so changing `NEXT_PUBLIC_*` or `VITE_*` requires a
rebuild, not a restart.

To roll back, check out the previous tag and rebuild. If that release predates
a migration, the older code runs against the newer schema — which is why
migrations are additive.

---

## Backups

The database is the only stateful thing in the stack. Everything else is built
from the repository.

```bash
# Nightly, and before every upgrade.
docker compose exec -T postgres \
  pg_dump -U nuhabit -Fc nuhabit > backup-$(date +%F).dump
```

Restore into an empty database:

```bash
docker compose exec -T postgres \
  pg_restore -U nuhabit -d nuhabit --clean --if-exists < backup-2026-09-09.dump
```

Test the restore on a staging copy. A backup nobody has restored is a file, not
a backup.

Keep them off this machine, and keep at least one that predates whatever went
wrong: the most common data loss is a mistake made a week ago and noticed
today.

Two things a `pg_dump` will not tell you and you should record separately:
`AUTH_SECRET` (a lost one signs everybody out) and which release the dump came
from (a newer dump does not restore into older code).

---

## Health and monitoring

| Endpoint | Question |
|---|---|
| `GET /api/health` | Is the process alive? |
| `GET /api/ready` | Can it serve? (checks the database) |

Point a load balancer at `/api/ready`, not `/api/health`: alive is not the same
as able to serve, and a container that has lost its database should be taken
out of rotation rather than sent traffic.

The API image is `scratch` — no shell, no curl — so its own container health
check is the binary asking itself: `api -health` reads `/ready` and exits with
the answer.

Logs are structured JSON on stdout, one line per request, each carrying a
request id that also comes back in the `X-Request-Id` header. When somebody
reports a failure, that id finds it.

```bash
docker compose logs -f api
docker compose logs -f api | grep '"level":"ERROR"'
```

Worth alerting on: any `ERROR`, `/api/ready` failing, `pg_isready` failing,
and disk on the database volume.

---

## Splitting the API

The same image becomes a single service when you set `MODULES`:

```yaml
environment:
  MODULES: catalog,identity,scheduling,access,reporting
  AUTO_MIGRATE: 'false'    # one owner applies migrations, not every replica
```

Available: `catalog`, `identity`, `wallet`, `scheduling`, `access`,
`incentives`, `reporting`, `engagement`, `training`, `hris`, `inventory`,
`purchasing`, `crm`, `pos`. Unset means all of them.

Tokens are self-contained and signed with `AUTH_SECRET`, so a token minted by
one service is accepted by another with no shared session store — as long as
they share the secret. Each service 404s on the other's routes.

`docker compose --profile split up -d` runs two such services side by side on
`:9081` and `:9082`, which is the cheapest way to see that the module
boundaries are real before committing to the split.

---

## Troubleshooting

**`config: AUTH_SECRET must be set outside development`** — `APP_ENV` is
production and the secret is still the default. Generate one.

**`config: AUTH_DEMO_OTP must be false in production`** — demo mode accepts any
OTP and lets the login screen sign in without a password. Set it to `false`.

**`config: no payment gateway is implemented`** — see [Before you take real
money](#before-you-take-real-money). Set `PAYMENTS_ALLOW_MOCK=true` only if you
mean it.

**`n staff account(s) already exist`** from `bootstrap` — somebody already ran
it. Create further accounts from the panel. If you have genuinely lost access
to every account, set a password directly:

```sql
-- Only as a last resort, and only with a hash from a trusted source.
UPDATE identity.admin_users
   SET password_hash = '<pbkdf2-sha256$...>', must_change_password = true,
       failed_logins = 0, locked_until = NULL
 WHERE lower(email) = 'alya@studio.example';
```

**Locked out after too many wrong passwords** — wait `AUTH_LOCKOUT_FOR`, or
have another Super Admin reset the password from **Config → Users → Password &
PIN**, which clears the lock.

**The proxy answers 502** — a service behind it is unhealthy. `docker compose
ps` shows which; `docker compose logs <service>` says why.

**A build fails partway with an unrelated error** — check `docker system df`.
A full Docker disk fails builds in ways that do not mention disk.
`docker image prune -a` reclaims unused images; `docker system prune -a
--volumes` also removes volumes, **including database volumes from other
projects**, so read it before running it.

**The apps show stale configuration** — `NEXT_PUBLIC_*` and `VITE_*` are baked
in at build time. Rebuild, do not restart.

---

## What is not finished

Honest list, so nobody discovers these in front of a member:

- **Payments.** No gateway. See above. This is the one that matters.
- **Notifications** are in-app only. No email, no SMS, no push.
- **Offline gate access** is modelled — the schema and the sync-conflict
  handling exist — but there is no device firmware to go with it.
- **The four ERP modules** (purchasing, POS, stock, CRM) have no in-process
  demo implementation, so `*_OFFLINE_DEMO=1` builds show them as needing a
  backend rather than pretending.
- **Manufacturing** (bills of materials, production orders) is not built.
