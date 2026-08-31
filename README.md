# HYROX Studio App

Monorepo implementing the **HYROX Studio Operating System** blueprint — member PWA, admin panel, and a mock backend with a real domain core:

**REGISTER → TOP UP → BOOK → CHECK-IN (QR) → CREDIT DEDUCTION → ATTEND**

Frontend-only for now: all data flows through **MSW** service workers backed by in-memory repositories, but through the exact HTTP contracts a real API will serve later. Swap = point the API client's base URL at a server and delete the worker.

## Apps & packages

| Path | What it is |
|---|---|
| `apps/member` | Member PWA — Vite 7 + React 19 + React Router v7 + vite-plugin-pwa. Dark ground with WIT Red (#ED1C24) accents. |
| `apps/admin` | Admin panel — Next.js 15 (App Router). Light theme, dark sidebar, table-first. |
| `packages/domain` | **Zero-dependency** entities, state machines, ledger math, booking/gate/voucher policies, RBAC. Fully unit-tested. |
| `packages/application` | Use cases + repository ports (clean architecture application layer). |
| `packages/contracts` | zod request schemas + response view models + route map — the future API surface. |
| `packages/api-client` | Typed fetch client used by both apps. |
| `packages/mock-api` | In-memory DB, deterministic seed, MSW handlers (RBAC enforced server-side), localStorage persistence. |
| `packages/ui` | Design tokens (Tailwind v4 `@theme`, WIT Red #ED1C24) + shared primitives. |

Layer rule (eslint-enforced): `domain → nothing`, `application → domain`, `api-client → contracts`, apps talk HTTP only — never to use cases directly.

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

- **Member app**: `demo@hyrox.id` (Fahmi Syaban) — any 6-digit OTP works (`123456`). Or register a fresh member.
- **Admin**: one-click login cards, one per role (Super Admin, HQ Admin, Branch Manager, Front Desk, Coach, Finance). RBAC is enforced by the mock server — a Front Desk token gets a real `403` on finance endpoints, not just hidden buttons.
- **Voucher codes**: `WELCOME10` (10%, new members), `HYROX100` (Rp100k, 10/20-packs).

> Note: the two apps run on different origins, so each has its own copy of the mock DB (seeded identically, persisted per-origin in localStorage). Use the in-app dev tools to reset.

## Walkthrough — Part A (member)

1. **Register** end-to-end: contact → OTP (any code) → personal → emergency contact → waiver + T&C → land on Home with **0 credits**.
2. **Top up**: Wallet → Top up → pick *10 Visit Pack* → apply `WELCOME10` → checkout → "Pay now (simulate success)" → balance **10**, ledger shows the `TOP_UP` entry (payment ≠ ledger: the pending payment added nothing).
3. **Book**: Classes → pick a session → Book. Book a FULL session → you're waitlisted with a position.
4. **QR**: the QR tab shows a token with a countdown ring; it re-issues itself on expiry.
5. **Check in**: open the flask (dev tools) → "Scan my QR at Senopati Gate A" → **ALLOWED**, balance −1, visit logged, booking → CHECKED_IN. Scan again → free **re-entry** (grace window). Wait past grace and scan → **DENIED · ANTI_PASSBACK**.
6. **Cancel** a booking before the deadline → slot released, no charge; after the deadline → credit forfeited (ledger entry).
7. Reload the page — everything survives (localStorage snapshot).

## Walkthrough — Train module (Strava-style)

1. **Feed** (`/train`): seeded activities from other members with route maps, kudos and comments; toggle Everyone / Following.
2. **Record** (`/train/record`): pick Run/Ride/Walk/Workout → Start. "Demo GPS" simulates a route (real GPS works on devices); live timer, distance and pace with auto-pause-aware moving time. Finish → title/description/gear/visibility → Save. Stats, splits and segment matching are computed "server-side" in the domain layer.
3. **Activity detail**: route, stat grid, per-km splits, segment efforts with leaderboard rank and PR badges, kudos + comment thread (owner gets notifications).
4. **You** (`/train/you`): weekly goal ring (editable), 8-week distance chart, all-time totals, PRs (best 1k split, estimated 5k/10k, longest), gear with automatic mileage + retire, your training log.
5. **Explore** (`/train/explore`): segments with leaderboards (best effort per athlete), monthly challenges with progress + leaderboard, clubs with weekly leaderboards, and athletes to follow/unfollow.
6. **Settings & profile**: `/profile/settings` (km/mi units, booking-reminder toggle), `/profile/emergency` (edit emergency contact). Booking reminders are generated once per booking when a class starts within 24h.
7. **Offline**: app shell + fonts are precached; the mock API and its localStorage state live in the browser, so the whole app keeps working offline (a banner shows when the connection drops).

Excluded by request: Strava's paid features (subscriptions, training plans, Beacon live tracking, advanced analysis).

## Walkthrough — HYROX Workout & Races (blueprint phases 3–4)

- **Workout generator** (`/workout`, or the Home quick card): pick Full Simulation / Coverage / Quick / Practice, a division (loads and wall-ball reps adjust per division), exclude unavailable equipment (the substitution engine swaps in the closest alternative). Preview lets you swap any station for a ranked substitute, then **Start** opens the active timer: total time, current block with target, Complete Block, Pause, Stop & Save (partial). Finished sessions land in the training log as WORKOUT activities and in the workout history.
- **Races** (`/races`): discover events by region (upcoming/results), add one to My Races with a division + goal time. My Races shows countdown, **prediction** (best completed Full Simulation −3%), **readiness** (training consistency, last 4 weeks), and a one-tap "Run simulation". Enter a result and get the analysis vs goal and vs prediction.

## Walkthrough — Train v2 (maps, routes, photos)

- Activity details, routes and the personal **heatmap** render on real **OpenLayers + OSM** tile maps; feed thumbnails stay on a lightweight SVG renderer.
- Segments are matched against real **GPS polylines** (start/end gates + distance sanity check); effort times come from actual point timestamps. The demo recorder runs along the Senopati corridor, so recorded runs genuinely earn efforts.
- Activities support **photos** (resized client-side), **edit/delete** (owner-only, gear mileage restored on delete), **save as route** → reuse via "Use this route" (the demo GPS follows the polyline), elevation gain (from GPS altitude; simulated in demo mode), and "trained together" grouping.
- **Member niceties**: avatar upload (profile photo shows across the feed), manual waitlist confirmation when auto-promote is off (offer → "Confirm spot"), and an EN/ID language setting (Bahasa Indonesia covers the member chrome).

## Walkthrough — Admin deepening

- **Schedule**: weekly Monday–Sunday grid with status-coloured session chips, week navigation, branch filter.
- **Vouchers**: full editing (value, window, limits) on top of create + state actions.
- **Coaches / Branches / Gates / Users**: complete CRUD from the UI (users are Super Admin-only, enforced server-side).
- **Campaigns**: custom audience builder (branch, max balance, days since visit, joined-within) with a live audience preview count.
- **Reports › Classes**: attendance per class type + recent no-shows.
- **Access Logs**: offline CONFLICT rows can be approved (deduction applied, audited) or rejected.

## Walkthrough — Part B (admin)

1. Sign in as **Front Desk** → Members → open Fahmi Syaban → 360° view with ledger, bookings, visits, payments, waiver, audit. Note: no "Manual adjustment" button (no permission) — and the API would 403 anyway.
2. **Gate Simulator** (Live Check-in): scan an ACTIVE member → ALLOWED with the 4-step pipeline all green + live feed row + deduction. Scan a SUSPENDED member → DENIED with the failing step highlighted. Double-scan within the anti-passback window → DENIED.
3. **Attendance**: open a session → roster → Check in / No-show (no-show forfeits per policy).
4. Cancel a CONFIRMED booking on a FULL session → the first waitlisted member is **auto-promoted** (they get a notification).
5. Sign in as **Finance** → Payments: simulate a webhook (`PENDING → PAID`), refund a paid payment (reverses its TOP_UP entry). Reports → Credits: outstanding total equals the sum of member balances.
6. Sign in as **Super Admin** → Configuration → Business Rules: change *QR expiration* or *re-entry grace* → the simulator honors it immediately (and the change is audited).

## Architecture notes

- **Ledger-first**: balance is always `Σ(entries)`. Finalized entries are immutable — corrections are `REVERSAL` entries; credit expiry consumes top-up lots FIFO.
- **State machines**: bookings, payments, sessions, vouchers, members, campaigns all use declarative transition maps; illegal transitions are rejected in one place.
- **Gate pipeline**: pure `evaluateGateScan` returns a decision + effects list; the use case applies effects atomically (token consumed, deduction, check-in, access log).
- **Configurable rules**: cancellation deadline, no-show policy, anti-passback, QR TTL, waitlist promotion etc. are data, with per-branch overrides (PIK overrides QR TTL in the seed).
- **Future phases** (workout generator, races, performance) layer on top without touching this core — see the blueprint.
