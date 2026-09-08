# NuHabit Studio — Backend

Go backend for the NuHabit (HYROX) studio operating system: membership, credit
wallet, class booking, QR gate access and coach payroll. PostgreSQL for
storage, no framework, and a module layout designed so any bounded context can
be lifted into its own service without a rewrite.

It serves the same API the member PWA and admin panel already consume, so the
frontend switches from its in-process mock to this backend by pointing at a
base URL.

## Quick start

```bash
cp .env.example .env
make up          # PostgreSQL + API in Docker, then the demo studio is seeded
curl localhost:9080/ready
```

Or against a local Go toolchain:

```bash
docker compose up -d postgres
make migrate && make seed && make run
```

Demo credentials: any member email from the seed (`demo@nuhabit.id`) — the
sign-in code comes back in the response while `AUTH_DEMO_OTP=true`. Staff sign
in by id: `adm_super`, `adm_hq`, `adm_branch`, `adm_desk`, `adm_coach`,
`adm_finance`.

### The core loop, end to end

```bash
API=http://localhost:9080

# 1. Sign in (demo mode returns the code instead of sending an SMS)
CH=$(curl -s -X POST $API/api/auth/otp/request -H 'content-type: application/json' \
      -d '{"identifier":"demo@nuhabit.id"}')
TOKEN=$(curl -s -X POST $API/api/auth/otp/verify -H 'content-type: application/json' \
      -d "{\"challengeId\":\"$(jq -r .challengeId <<<"$CH")\",\"code\":\"$(jq -r .code <<<"$CH")\"}" | jq -r .token)

# 2. Buy credits and settle the invoice
PAY=$(curl -s -X POST $API/api/me/topup -H "Authorization: Bearer $TOKEN" \
      -H 'content-type: application/json' \
      -d '{"packageId":"pkg_starter5","voucherCode":"HYROX100","channel":"QRIS"}' | jq -r .payment.id)
curl -s -X POST $API/api/payments/$PAY/simulate -H "Authorization: Bearer $TOKEN" | jq .entry

# 3. Book a class, then walk through the gate
curl -s -X POST $API/api/sessions/<id>/book -H "Authorization: Bearer $TOKEN"
QR=$(curl -s -X POST $API/api/me/qr -H "Authorization: Bearer $TOKEN" | jq -r .token)
curl -s -X POST $API/api/gates/gat_senopati_a/scan -H 'content-type: application/json' \
      -d "{\"qrToken\":\"$QR\"}" | jq '{decision, entryKind, remainingCredits}'
```

## Layout

```
cmd/                  api, migrate, seed
internal/
  domain/             business rules as pure Go: no database, no HTTP
  platform/           config, database, httpx, auth, audit, outbox, clock, id
  modules/            one package per bounded context
    catalog/          branches, gates, coaches, class types, packages, rules
    identity/         members, staff accounts, OTP sign-in
    wallet/           payments, credit ledger, expiry lots, vouchers
    scheduling/       class sessions, bookings, waitlist, attendance
    access/           QR credentials, the gate pipeline, access logs
    incentives/       coach schemes, statements, payouts
    reporting/        dashboard, reports, member 360 (owns no tables)
  app/                wiring: the only place that knows every module exists
  seed/               the demo studio
migrations/           schema, one file per module, embedded in the binary
```

Three rules hold the shape:

1. **`domain` depends on nothing.** Every rule that decides money or access is
   a pure function, tested without a database.
2. **A module reads only its own schema.** Anything it needs from another
   module it asks for through an interface *it* declares; `internal/app` wires
   those up.
3. **`internal/app` is the only place with the full picture.** Nothing else
   imports more than one module.

`docs/ARCHITECTURE.md` covers the reasoning and the extraction playbook.

## Running it as microservices

The same binary becomes a single service by naming the modules it should mount:

```bash
MODULES=catalog,identity,scheduling,access,reporting ./api   # studio operations
MODULES=wallet,incentives ./api                              # money
```

```bash
docker compose --profile split up --build   # both, on :9081 and :9082
```

That works today because the boundaries are already real: no cross-schema
joins, cross-module effects published through a transactional outbox, and
stateless token verification so each service authenticates callers on its own.

## Business rules worth knowing

- **Balance is never stored.** It is the sum of the credit ledger, and the
  ledger table refuses `UPDATE` and `DELETE` at the database level. A mistake
  is corrected with a REVERSAL row that points at the original.
- **Credits are deducted at the gate, not at booking.** Booking checks that a
  member *could* pay and holds the place; the door charges them. A no-show is
  handled by policy, not by having been charged twice.
- **Credits expire FIFO by lot.** Each purchase creates a lot with its own
  expiry; consumption is allocated to whichever expires soonest.
- **A QR code is a credential, not an identity.** It is random, lives about 45
  seconds, and is burned on use — including on a refused scan, so a rejected
  code cannot be retried at the next gate.
- **Entry requires a booked class.** There is no open-gym access. Re-entry
  inside the grace window is free; a second entry inside the anti-passback
  window is refused.
- **Coach payroll is frozen when approved.** A later attendance correction
  never changes an approved payout; void it and issue a new one.

Each of these has a test that fails if it stops being true.

## Testing

```bash
make test              # domain rules, no database required
make test-integration  # the full HTTP surface against PostgreSQL
```

The integration suite drives real HTTP against a real database and covers the
core loop, single-use QR codes, capacity and waitlist promotion, late
cancellation, package coverage, duplicate payment callbacks, refunds, RBAC
refusals per role, cross-member data access, voucher eligibility, the payout
approval chain, delete guards, draft visibility, and the append-only ledger.
They skip themselves when `TEST_DATABASE_URL` is unset.

## Configuration

Everything comes from the environment; see `.env.example`. The settings that
matter most:

| Variable | Default | Notes |
|---|---|---|
| `DATABASE_URL` | local compose DSN | Required |
| `AUTH_SECRET` | `dev-secret-change-me` | Signs tokens; startup fails in production if left at the default |
| `AUTH_DEMO_OTP` | `true` | Returns sign-in codes in responses; startup fails in production if true |
| `MODULES` | `all` | Which bounded contexts this process mounts |
| `PAYMENTS_PROVIDER` | `mock` | `xendit` for the real gateway |
| `STUDIO_TIMEZONE` | `Asia/Jakarta` | What "today" and a payroll month mean |
| `AUTO_MIGRATE` | `true` | Migrations run at boot behind an advisory lock |

## Status

Implemented and covered by tests: identity and sign-in, catalog and business
rules, the credit wallet with payments, vouchers, refunds and expiry, class
scheduling with bookings, waitlist and attendance, QR gate access with offline
reconciliation, coach incentives and payouts, and the cross-module reporting
layer.

The `training` schema (activities, segments, clubs, gear, generated workouts,
race calendars) ships with the migrations so the tables exist, but its HTTP
surface is not built yet — that is the member app's social and workout tab, and
it is the natural next module.

Payments run against a mock gateway. The `wallet.Gateway` interface is where a
real Xendit client drops in; nothing else changes.
