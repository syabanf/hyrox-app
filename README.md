# NüHabit App

Monorepo implementing the **NüHabit Studio Operating System** blueprint - member PWA, admin panel, and a Go + PostgreSQL backend, all in one repository:

**REGISTER → TOP UP → BOOK → CHECK-IN (QR) → CREDIT DEDUCTION → ATTEND**

Both apps talk to the Go backend: they call `/api` on their own origin, which nginx routes in the Docker stack and each dev server proxies in development (`API_PROXY_TARGET`, default `http://localhost:8080`). `NEXT_PUBLIC_OFFLINE_DEMO=1` / `VITE_OFFLINE_DEMO=1` takes the server out of the picture and answers every request in-process from a bundled seed instead — a demo with no database behind it, minus the four ERP modules, which say so rather than pretending. Same contracts either way.

## Apps & packages

| Path | What it is |
|---|---|
| `apps/member` | Member PWA - Vite 7 + React 19 + React Router v7 + vite-plugin-pwa. NüHabit brand: White Beige ground (#F3ECE2), Deep Forest Green ink (#00281A), Pale Lime accents (#DAFF59). |
| `apps/admin` | Admin panel - Next.js 15 (App Router). Light theme, dark sidebar, table-first. |
| `apps/backend` | **The API** - Go + PostgreSQL, one module per bounded context, no framework. See `apps/backend/README.md`. |
| `packages/domain` | **Zero-dependency** entities, state machines, ledger math, booking/gate/voucher policies, coach incentive math, RBAC. Fully unit-tested. |
| `packages/application` | Use cases + repository ports (clean architecture application layer). |
| `packages/contracts` | zod request schemas + response view models + route map - the future API surface. |
| `packages/api-client` | Typed fetch client used by both apps. |
| `packages/mock-api` | In-memory DB, deterministic seed, MSW handlers (RBAC enforced server-side), localStorage persistence. |
| `packages/ui` | Design tokens (Tailwind v4 `@theme`) for the NüHabit palette - White Beige #F3ECE2, Deep Forest Green #00281A, Pale Lime #DAFF59, Lettuce #ABDE67, Golden Ochre #C9A227 - with Outfit (titles) + Manrope (body); plus shared primitives. |

Layer rule (eslint-enforced): `domain → nothing`, `application → domain`, `api-client → contracts`, apps talk HTTP only - never to use cases directly.

## Run it

The whole product, one command:

```bash
pnpm stack:up       # PostgreSQL + Go API + member app + admin panel, then seeded
```

| | |
|---|---|
| http://localhost:8088 | member PWA |
| http://localhost:8088/admin | admin panel |
| http://localhost:8088/api | the API (also direct on :9080) |

One origin serves all three, so the browser never makes a cross-origin call.
`pnpm stack:down` stops it, `pnpm stack:reset` also drops the data.

For a real server — configuration, the first staff account, seeding, TLS,
upgrades and backups — see **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)**. Read
its payments section before taking money: no gateway is implemented yet.

Every service reports its own health, and each waits for what it depends on:
the API waits for a healthy database, the seeder waits for a ready API (so the
migrations have run), and the front door waits for all three rather than
answering the first request with a 502. The API image is `scratch`, so its
probe is the binary asking itself — `api -health` reads `/ready` and exits
with the answer.

To build the apps against their bundled seed instead of the API — a demo with
no database at all:

```bash
VITE_OFFLINE_DEMO=1 NEXT_PUBLIC_OFFLINE_DEMO=1 pnpm stack:up
```

Or run the pieces from source:

```bash
pnpm install
pnpm dev            # both apps via turbo
pnpm dev:member     # http://localhost:5173
pnpm dev:admin      # http://localhost:3000 (proxies /api to the backend)
pnpm dev:backend    # http://localhost:8080 (needs PostgreSQL: make -C apps/backend db)
```

Start the backend before the admin panel, or seed and start both:
`make -C apps/backend db && go run ./apps/backend/cmd/seed`.

Sign in at <http://localhost:3000/admin> with any seeded staff email and the
password the seeder prints: `alya@nuhabit.id` / `nuhabit-demo-2026`. With demo
mode on (`AUTH_DEMO_OTP`, the default outside production) the login screen also
offers role cards that sign in without a password. The member app signs itself
in as `demo@nuhabit.id`.

Verification suite:

```bash
pnpm turbo typecheck test build && pnpm lint
```

PWA check: `pnpm --filter @nuhabit/member build && pnpm --filter @nuhabit/member preview` (service worker + manifest are production-build only).

## Demo images

The seed's pictures — member and staff avatars, race cards, shelf photos —
are generated SVGs committed under `apps/backend/internal/media/assets`,
embedded in the API binary and served from `/api/media/...`. No CDN, no
network: the demo has faces on it offline. Regenerate them with
`go run ./internal/media/gen` from `apps/backend`; a change to a picture
should be a change to its file name, because they are cached hard.

## Demo accounts

- **Member app**: opens straight on Home as `demo@nuhabit.id` (Fahmi Syaban) with no OTP step. The login screen (any 6-digit OTP works, e.g. `123456`) only appears after **Sign out**; from there you can sign in as another member or register a fresh one.
- **Admin**: six seeded staff, one per role (Super Admin, HQ Admin, Branch
  Manager, Front Desk, Coach, Finance), all with the password
  `nuhabit-demo-2026` and all reachable from the login screen's demo role cards
  while demo mode is on. RBAC is enforced by the server - a Front Desk token
  gets a real `403` on finance endpoints, not just hidden buttons. A Super
  Admin can set somebody a password from **Config → Users → Password & PIN**;
  a password set by somebody else has to be replaced at the next sign-in.
- **Voucher codes**: `WELCOME10` (10%, new members), `HYROX100` (Rp100k, 10/20-packs).

> Note: the two apps run on different origins, so each has its own copy of the mock DB (seeded identically, persisted per-origin in localStorage). Use the in-app dev tools to reset.

## Walkthrough - Part A (member)

1. **Register** end-to-end: contact → OTP (any code) → personal → emergency contact → waiver + T&C → land on Home with **0 credits**.
2. **Top up**: Wallet → Top up → pick *10 Visit Pack* → apply `WELCOME10` → checkout → "Pay now (simulate success)" → balance **10**, ledger shows the `TOP_UP` entry (payment ≠ ledger: the pending payment added nothing).
3. **Book**: Classes → pick a session → Book. Book a FULL session → you're waitlisted with a position.
4. **QR**: the QR tab shows a token with a countdown ring; it re-issues itself on expiry.
5. **Check in**: the QR tab shows the class you can check in to right now (the seed confirms you on a Senopati class starting in ~20 minutes). Open the flask (dev tools) → "Scan my QR · Senopati Gate A" → **ALLOWED**, balance −1, visit logged, booking → CHECKED_IN. Scan again → free **re-entry** (grace window). Wait past grace and scan → **DENIED · ANTI_PASSBACK**. Reset the demo and scan at **PIK Gate A** instead → **DENIED · NO_BOOKING**: there is **no open gym** - every entry is a check-in to a booked class at that branch, and the gate denies anyone without one.
6. **Cancel** a booking before the deadline → slot released, no charge; after the deadline → credit forfeited (ledger entry).
7. Reload the page - everything survives (localStorage snapshot).

## Walkthrough - Train module (Strava-style)

1. **Feed** (`/train`): seeded activities from other members with route maps, kudos and comments; toggle Everyone / Following.
2. **Record** (`/train/record`): pick Run/Ride/Walk/Workout → Start. "Demo GPS" simulates a route (real GPS works on devices); live timer, distance and pace with auto-pause-aware moving time. Finish → title/description/gear/visibility → Save. Stats, splits and segment matching are computed "server-side" in the domain layer.
3. **Activity detail**: route, stat grid, per-km splits, segment efforts with leaderboard rank and PR badges, kudos + comment thread (owner gets notifications).
4. **You** (`/train/you`): weekly goal ring (editable), 8-week distance chart, all-time totals, PRs (best 1k split, estimated 5k/10k, longest), gear with automatic mileage + retire, your training log.
5. **Explore** (`/train/explore`): segments with leaderboards (best effort per athlete), monthly challenges with progress + leaderboard, clubs with weekly leaderboards, and athletes to follow/unfollow.
6. **Settings & profile**: `/profile/settings` (km/mi units, booking-reminder toggle), `/profile/emergency` (edit emergency contact). Booking reminders are generated once per booking when a class starts within 24h.
7. **Offline**: app shell + fonts are precached; the mock API and its localStorage state live in the browser, so the whole app keeps working offline (a banner shows when the connection drops).

Excluded by request: Strava's paid features (subscriptions, training plans, Beacon live tracking, advanced analysis).

## Walkthrough - HYROX Workout & Races (blueprint phases 3–4)

- **Workout generator** (`/workout`, or the Home quick card): pick Full Simulation / Coverage / Quick / Practice, a division (loads and wall-ball reps adjust per division), exclude unavailable equipment (the substitution engine swaps in the closest alternative). Preview lets you swap any station for a ranked substitute, then **Start** opens the active timer: total time, current block with target, Complete Block, Pause, Stop & Save (partial). Finished sessions land in the training log as WORKOUT activities and in the workout history.
- **Races** (`/races`): discover events by region (upcoming/results), add one to My Races with a division + goal time. My Races shows countdown, **prediction** (best completed Full Simulation −3%), **readiness** (training consistency, last 4 weeks), and a one-tap "Run simulation". Enter a result and get the analysis vs goal and vs prediction.

## Walkthrough - Train v2 (maps, routes, photos)

- Activity details, routes and the personal **heatmap** render on real **OpenLayers + OSM** tile maps; feed thumbnails stay on a lightweight SVG renderer.
- Segments are matched against real **GPS polylines** (start/end gates + distance sanity check); effort times come from actual point timestamps. The demo recorder runs along the Senopati corridor, so recorded runs genuinely earn efforts.
- Activities support **photos** (resized client-side), **edit/delete** (owner-only, gear mileage restored on delete), **save as route** → reuse via "Use this route" (the demo GPS follows the polyline), elevation gain (from GPS altitude; simulated in demo mode), and "trained together" grouping.
- **Member niceties**: avatar upload (profile photo shows across the feed), manual waitlist confirmation when auto-promote is off (offer → "Confirm spot"), and an EN/ID language setting (Bahasa Indonesia covers the member chrome).

## Walkthrough - Admin deepening

- **Schedule**: weekly Monday–Sunday grid with status-coloured session chips, week navigation, branch filter.
- **Vouchers**: full editing (value, window, limits) on top of create + state actions.
- **Coaches / Branches / Gates / Users**: complete CRUD from the UI (users are Super Admin-only, enforced server-side).
- **Campaigns**: custom audience builder (branch, max balance, days since visit, joined-within) with a live audience preview count.
- **Reports › Classes**: attendance per class type + recent no-shows.
- **Coach Incentives**: schemes (default + per-coach overrides), monthly statements from completed sessions, payout state machine DRAFT → APPROVED → PAID (or VOID), audited.
- **Access Logs**: offline CONFLICT rows can be approved (the booked class is deducted and checked in, audited) or rejected. A conflict with no matching booking cannot be approved - there is no open gym to charge.
- **People (HRIS)**: staff directory, weekly shift patterns, the daily roster, timesheets, leave and overtime. See its own walkthrough below.
- **Stock, Purchasing, Loyalty and the Till**: the four ERP modules, ported from nuhabiterp into our own stack. See their walkthrough below.

## Walkthrough - Coach incentives (pembagian insentif pelatih)

Coach pay is IDR payroll computed from completed sessions - separate from the member credit ledger. Sign in as **Finance** (or HQ Admin) → Operations → **Coach Incentives**.

1. **Schemes**: the organization default (Rp150.000 per session + Rp10.000 per attendee + Rp100.000 full-class bonus at ≥ 80% of capacity, no no-show penalty) and per-coach overrides - Kevin Hartono (senior: Rp200.000 / Rp12.500 / Rp150.000 bonus) and Rizky Ramadhan (race coach: Rp175.000, bonus at 75%, **Rp25.000 per no-show**). Edit a rate → audited; existing payouts stay frozen.
2. **Statements**: pick a month (defaults to the current one) and optionally a branch. Every active coach gets a statement: per session `fee + attended × per-attendee + bonus (if attended ≥ threshold % of capacity) − no-shows × penalty`, clamped at Rp0. Expand a row for the per-session lines. Switch to **last month** to see the seeded history and its payouts (Kevin PAID, Maya APPROVED, Rizky DRAFT). For a coach without a payout, **Create payout** snapshots the statement as a DRAFT (one live payout per coach + month; the API returns 409 on a duplicate).
3. **Payouts**: **Approve** a DRAFT → **Mark paid** (asks for the transfer reference) → PAID. **Void** (with a reason) from DRAFT or APPROVED frees the period for a fresh payout; PAID and VOID are terminal. Every transition is audited (Configuration → Audit Trail). A Branch Manager can view statements and payouts but the buttons are hidden - and the server returns 403 either way; Front Desk and Coach roles cannot see the module at all.
4. The dashboard shows **Coach incentives payable** for the current month.

## Walkthrough - People (HRIS)

The staff side, ported from the nuhabiterp ERP into our own stack. Sign in as
**Super Admin**, **HQ Admin**, **Branch Manager** or **Finance** → **People**.
Front Desk and Coach roles cannot see the module at all - employee records hold
home addresses, bank accounts and next of kin.

1. **Roster**: who is expected today and who actually turned up. A person with a
   pattern row and no shift shows as `REST_DAY`; a person with no pattern at all
   shows as `UNSCHEDULED` - the roster has to tell those apart. Once a shift has
   ended with nobody clocked in the row turns `MISSING`, which is a statement of
   fact; marking it `ABSENT` is HR's decision and a separate action.
2. **Clock somebody in** from the roster. Lateness is counted **from the shift
   start**, not from the end of the tolerance: the grace period only decides
   *whether* an arrival is late, so twelve minutes into a shift with ten
   minutes' grace is twelve minutes late, not two.
3. **Leave & Overtime → File leave** across a range containing a public holiday.
   The holiday does not cost a day and the response names it. Collective leave
   (*cuti bersama*) **does** cost a day - that is what `deductsLeave` on the
   calendar marks, and where both fall on one date the national holiday wins.
4. The allowance moves **on approval**, never on filing - but pending days are
   reserved, so two requests cannot together overspend the year. Approve, watch
   the balance drop on the employee's page, then cancel: the days come back.
   Against the Go backend a database CHECK refuses an overspend even if the
   application is bypassed.
5. **Overtime** 22:00 → 01:00 is three hours, not minus twenty-one. Approving it
   lands the hours on that day's timesheet; cancelling takes them back off.
6. **Shifts & Calendar**: shift windows, their unpaid break and their grace
   period, plus the year's public holidays. The government moves these dates by
   decree, so the calendar is a table HR edits rather than a constant.

## Walkthrough - the ERP modules

Four modules ported from the nuhabiterp ERP. They run on the Go backend only —
the offline demo (`NEXT_PUBLIC_OFFLINE_DEMO=1`) says so rather than pretending,
so leave it off and start the API.

**Stock (Gudang)** → **Stock Levels**. Held per branch, because "do we have
protein bars" is not a question until it says where.

1. One shelf is below its reorder point on a fresh install. The figure counts
   stock already on order, so a purchase order already raised does not make the
   screen ask for a second one.
2. **Adjust** a quantity. A reason is required by the server, not just the
   form: an unexplained change to a quantity is indistinguishable from theft.
3. **Transfer** between branches. Two movements bound into one act, so the
   total is unchanged and a transfer larger than the source has moves neither.
4. **Stock Takes** → open a count, type what is on the shelf. The expected
   figure is frozen when an item is counted. Applying writes one adjustment per
   varied line and nothing at all for the lines that matched.
5. **Stock Ledger** shows every one of those movements. It is append-only; the
   database refuses an `UPDATE`.

**Purchasing** → **Requests**. Nobody signs a purchase alone.

6. Raise a request for something cheap and submit it: one signature, from a
   branch manager. Raise one for Rp 35m and the chain grows to head → finance →
   director, decided by the amount and nothing else.
7. Sign as **Branch Manager**, then try to sign again: refused, because the
   request is now waiting on finance. Sign as **Finance**, then as **Super
   Admin**. A higher office may sign for a lower one; the reverse never.
8. **Make an order** from the approved request, **Approve**, **Send**. Sending
   tells stock the goods are coming, so the reorder screen stops asking.
9. **Record a delivery**: accept 57 and reject 3 of an order for 100. Posting
   it puts 57 into stock at the price on the order — watch the item's average
   cost move — leaves the 3 out, and turns the order `PARTIALLY_RECEIVED`.
   Try to receive 50 more of the 43 outstanding: refused.
10. A partly received order can no longer be cancelled: stock has moved.

**Loyalty (CRM)** → **Members**. Points are not credits.

11. Settle a member top-up (Commercial → Payments) and watch points appear:
    loyalty subscribes to the same outbox event engagement does, and neither
    wallet nor scheduling knows it exists.
12. **Adjust** points by hand — a reason is required — until the member reaches
    Silver. **Redeem** a reward: the spendable balance falls, the lifetime
    total does not, and the tier holds. A scheme that demotes people for using
    it teaches them not to use it.
13. Try a Gold-only reward as a Silver member: told the tier is too low rather
    than told to save up for something they will never be allowed.
14. **Rewards** → cancel a claim: the points and the stock both come back.

**The till (POS)** → **Till**.

15. **Open a till** — a sale with no shift behind it has nowhere to be counted,
    and is refused. One till per cashier per branch.
16. Ring up two items and name the member: their tier discount applies
    automatically, and stays a fixed percentage however many lines are scanned.
17. **Take payment**. A card cannot overpay — there is no change to give — but
    cash can, and the change comes out of the cash tendered.
18. **Complete sale**: the stock comes off the shelf, the points land, and the
    sale closes, all in one transaction. Sell more than is on the shelf and the
    whole thing is refused with the money uncounted.
19. **Sales** → **Void** as a manager (the front desk cannot): the stock goes
    back. An unpaid sale is cancelled instead; conflating the two would let the
    counter erase a paid sale by calling it a cancellation.
20. **Till Shifts** → close the till. The drawer counts cash and nothing else —
    counting card takings would make every till look short by exactly them.

## Walkthrough - Part B (admin)

1. Sign in as **Front Desk** → Members → open Fahmi Syaban → 360° view with ledger, bookings, visits, payments, waiver, audit. Note: no "Manual adjustment" button (no permission) - and the API would 403 anyway.
2. **Gate Simulator** (Live Check-in): scan Fahmi Syaban at Senopati Gate A (confirmed on the live class) → ALLOWED with the 5-step pipeline (QR → membership → anti-passback → class booked now → credit) all green + live feed row + deduction. Scan a member with no class booked around now → DENIED · NO_BOOKING (no open gym). Scan a SUSPENDED member → DENIED with the failing step highlighted. Double-scan within the anti-passback window → DENIED.
3. **Attendance**: open a session → roster → Check in / No-show (no-show forfeits per policy).
4. Cancel a CONFIRMED booking on a FULL session → the first waitlisted member is **auto-promoted** (they get a notification).
5. Sign in as **Finance** → Payments: simulate a webhook (`PENDING → PAID`), refund a paid payment (reverses its TOP_UP entry). Reports → Credits: outstanding total equals the sum of member balances.
6. Sign in as **Super Admin** → Configuration → Business Rules: change *QR expiration* or *re-entry grace* → the simulator honors it immediately (and the change is audited).

## Architecture notes

- **Ledger-first**: balance is always `Σ(entries)`. Finalized entries are immutable - corrections are `REVERSAL` entries; credit expiry consumes top-up lots FIFO.
- **State machines**: bookings, payments, sessions, vouchers, members, campaigns and coach payouts all use declarative transition maps; illegal transitions are rejected in one place.
- **Gate pipeline**: pure `evaluateGateScan` returns a decision + effects list; the use case applies effects atomically (token consumed, deduction, check-in, access log). Every entry is a class check-in (`BOOKING`) or a free `RE_ENTRY` within the grace window - there is no open-gym entry, so a scan without a booked class is `DENIED · NO_BOOKING`.
- **Configurable rules**: cancellation deadline, no-show policy, anti-passback, QR TTL, waitlist promotion etc. are data, with per-branch overrides (PIK overrides QR TTL in the seed).
- **Future phases** (workout generator, races, performance) layer on top without touching this core - see the blueprint.
