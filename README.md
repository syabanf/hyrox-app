# NüHabit App

Monorepo implementing the **NüHabit Studio Operating System** blueprint - member PWA, admin panel, and a mock backend with a real domain core:

**REGISTER → TOP UP → BOOK → CHECK-IN (QR) → CREDIT DEDUCTION → ATTEND**

Frontend-only for now: all data flows through **MSW** service workers backed by in-memory repositories, but through the exact HTTP contracts a real API will serve later. Swap = point the API client's base URL at a server and delete the worker.

## Apps & packages

| Path | What it is |
|---|---|
| `apps/member` | Member PWA - Vite 7 + React 19 + React Router v7 + vite-plugin-pwa. NüHabit brand: White Beige ground (#F3ECE2), Deep Forest Green ink (#00281A), Pale Lime accents (#DAFF59). |
| `apps/admin` | Admin panel - Next.js 15 (App Router). Light theme, dark sidebar, table-first. |
| `packages/domain` | **Zero-dependency** entities, state machines, ledger math, booking/gate/voucher policies, coach incentive math, RBAC. Fully unit-tested. |
| `packages/application` | Use cases + repository ports (clean architecture application layer). |
| `packages/contracts` | zod request schemas + response view models + route map - the future API surface. |
| `packages/api-client` | Typed fetch client used by both apps. |
| `packages/mock-api` | In-memory DB, deterministic seed, MSW handlers (RBAC enforced server-side), localStorage persistence. |
| `packages/ui` | Design tokens (Tailwind v4 `@theme`) for the NüHabit palette - White Beige #F3ECE2, Deep Forest Green #00281A, Pale Lime #DAFF59, Lettuce #ABDE67, Golden Ochre #C9A227 - with Outfit (titles) + Manrope (body); plus shared primitives. |

Layer rule (eslint-enforced): `domain → nothing`, `application → domain`, `api-client → contracts`, apps talk HTTP only - never to use cases directly.

## Run it

```bash
pnpm install
pnpm dev            # both apps via turbo
# or individually:
pnpm dev:member     # http://localhost:5173
pnpm dev:admin      # http://localhost:3000
```

Verification suite:

```bash
pnpm turbo typecheck test build && pnpm lint
```

PWA check: `pnpm --filter @hyrox/member build && pnpm --filter @hyrox/member preview` (service worker + manifest are production-build only).

## Demo accounts

- **Member app**: opens straight on Home as `demo@hyrox.id` (Fahmi Syaban) with no OTP step. The login screen (any 6-digit OTP works, e.g. `123456`) only appears after **Sign out**; from there you can sign in as another member or register a fresh one.
- **Admin**: one-click login cards, one per role (Super Admin, HQ Admin, Branch Manager, Front Desk, Coach, Finance). RBAC is enforced by the mock server - a Front Desk token gets a real `403` on finance endpoints, not just hidden buttons.
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

## Walkthrough - Coach incentives (pembagian insentif pelatih)

Coach pay is IDR payroll computed from completed sessions - separate from the member credit ledger. Sign in as **Finance** (or HQ Admin) → Operations → **Coach Incentives**.

1. **Schemes**: the organization default (Rp150.000 per session + Rp10.000 per attendee + Rp100.000 full-class bonus at ≥ 80% of capacity, no no-show penalty) and per-coach overrides - Kevin Hartono (senior: Rp200.000 / Rp12.500 / Rp150.000 bonus) and Rizky Ramadhan (race coach: Rp175.000, bonus at 75%, **Rp25.000 per no-show**). Edit a rate → audited; existing payouts stay frozen.
2. **Statements**: pick a month (defaults to the current one) and optionally a branch. Every active coach gets a statement: per session `fee + attended × per-attendee + bonus (if attended ≥ threshold % of capacity) − no-shows × penalty`, clamped at Rp0. Expand a row for the per-session lines. Switch to **last month** to see the seeded history and its payouts (Kevin PAID, Maya APPROVED, Rizky DRAFT). For a coach without a payout, **Create payout** snapshots the statement as a DRAFT (one live payout per coach + month; the API returns 409 on a duplicate).
3. **Payouts**: **Approve** a DRAFT → **Mark paid** (asks for the transfer reference) → PAID. **Void** (with a reason) from DRAFT or APPROVED frees the period for a fresh payout; PAID and VOID are terminal. Every transition is audited (Configuration → Audit Trail). A Branch Manager can view statements and payouts but the buttons are hidden - and the server returns 403 either way; Front Desk and Coach roles cannot see the module at all.
4. The dashboard shows **Coach incentives payable** for the current month.

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
