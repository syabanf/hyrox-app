# Architecture

This backend is a modular monolith built so that becoming microservices is a
deployment decision, not a rewrite. This document explains the boundaries, why
they sit where they do, and what it actually takes to pull one out.

## The shape

```
                     ┌──────────────────────────────┐
   member PWA ─────► │        HTTP (net/http)       │ ◄───── admin panel
   gate scanner ───► │  router · auth · CORS · logs │
                     └──────────────┬───────────────┘
                                    │
        ┌──────────┬────────────┬───┴────┬───────────┬────────────┐
        ▼          ▼            ▼        ▼           ▼            ▼
    identity  catalog  wallet  scheduling  access  incentives  hris
    inventory ─── purchasing        pos ─── crm
        │          │            │        │           │            │
        └──────────┴────────────┴────┬───┴───────────┴────────────┘
                                     ▼
                                  reporting          (reads, owns no tables)
                                     │
                     ┌───────────────┴───────────────┐
                     │      internal/domain          │  pure rules, no I/O
                     └───────────────────────────────┘
                                     │
                     ┌───────────────┴───────────────┐
                     │   PostgreSQL, schema per module │
                     └───────────────────────────────┘
```

## Layers

**`internal/domain`** holds the rules that decide money and access: booking
eligibility, the credit ledger and its FIFO expiry, the gate pipeline, voucher
validation, coach payroll, the RBAC matrix, and every state machine. It imports
nothing but the standard library. That constraint is what makes these rules
cheap to test exhaustively — the whole suite runs in under a second with no
database — and it is why the interesting logic is readable without knowing
anything about HTTP or SQL.

**`internal/platform`** is the technical kernel: configuration, the pgx pool
and its transaction manager, the HTTP kernel (router, error envelope,
middleware), token issue and verification, the audit recorder, the outbox, a
clock and an id generator. It knows nothing about studios.

**`internal/modules/*`** are the bounded contexts. Each has the same four
pieces: a `Repository` (its schema, and only its schema), a `Service` (use
cases), a `Handler` (routes and DTOs), and the ports it needs from elsewhere.

**`internal/app`** wires them together. It is the only package that imports
more than one module.

## Why these boundaries

The split follows what changes together and what must be atomic together.

- **wallet** owns money and credits because a payment settling, the TOP_UP
  entry it produces and the lot it expires from must commit as one transaction.
- **scheduling** owns places in classes because taking the last one is a
  contention point that needs a row lock and a uniqueness constraint in one
  place.
- **access** owns the door because opening it, burning the credential and
  charging the credit are a single atomic decision. The rule "the gate never
  opens without its deduction" is only enforceable if one module commits all
  three.
- **catalog** owns configuration because everything else reads it and nothing
  else writes it.
- **identity** owns accounts because sign-in is the one thing every other
  module trusts the result of.
- **hris** owns employees because a person on the payroll is not a member and
  not a login. The same human can be all three, and each of the three ends
  independently: leaving the company does not cancel a membership, and losing a
  login does not erase a timesheet. Modelling them as one row would make every
  one of those endings a special case.
- **inventory** owns stock because a quantity that purchasing, the till and a
  stock take all change needs exactly one place that decides whether a change
  is legal. It is the only module two others write through rather than to.
- **purchasing** and **pos** own documents, not quantities. Both reach
  inventory through narrow ports — five methods and four — and the adapters in
  `internal/app` are the complete list of what becomes a network hop.
- **crm** owns loyalty, and deliberately not money. Points and credits are two
  ledgers that never meet: a reward can hand over credits, credits can never
  become points.
- **reporting** owns nothing. It exists so that cross-module reads happen in
  one visible place rather than as joins that quietly grow between contexts.

The boundary that would have been wrong is a "member service" owning members,
their credits and their bookings together. That is one table's worth of
cohesion and three transactions' worth of contention.

## Rules that keep the boundaries real

### One schema per module, no cross-schema joins

Each module's tables live in its own PostgreSQL schema. References across a
boundary are plain `TEXT` ids with an index and **no foreign key** — a booking
stores `member_id` but the database does not enforce that it exists, because
identity may one day live in another database.

Within a module, foreign keys are used freely. The `wallet.top_up_lots` row
references its ledger entry, and it should.

The practical test: `grep` a module's SQL and every table named is in its own
schema. That property is what makes extraction mechanical.

### Consumers declare their ports

A module never exports "here is my interface for you". It declares what it
needs from others:

```go
// internal/modules/scheduling/service.go
type Wallet interface {
    Balance(ctx context.Context, memberID string) (int, error)
    CoveredClassTypes(ctx context.Context, memberID string) ([]string, error)
    Forfeit(ctx context.Context, memberID string, amount int, description, sourceID string) (domain.CreditLedgerEntry, error)
}
```

Scheduling needs three things from the wallet, so its port has three methods,
not the fifty the wallet service actually exposes. When the wallet moves out,
the HTTP client that replaces it has three methods to implement.

Most ports are satisfied structurally — the wallet service already has those
methods, so wiring is `scheduling.NewService(..., walletService, ...)` with no
adapter. Where shapes differ, the adapter lives in `internal/app`
(`usage.go`, `members_adapter.go`) and is a handful of lines. Those files are
the complete list of places a network hop would be introduced.

### Cross-module effects go through the outbox

When a booking confirms, a member should be notified. That notification must
not be written by the scheduling module reaching into engagement's tables, and
it must not be lost if the notifier is down.

So scheduling publishes to `platform.outbox_messages` **inside the same
transaction** as the booking. A dispatcher picks it up and delivers it. Today
delivery is an in-process function call; tomorrow it is a queue. The publisher
does not know or care, and the message cannot exist without the booking that
justified it.

`FOR UPDATE SKIP LOCKED` on the claim query means several instances can run the
dispatcher without duplicating work.

### Tokens are self-contained

Authentication is an HS256 JWT signed with a shared secret, implemented in
~120 lines against the standard library. Any service can verify a caller
without calling an auth service or sharing a session table — which is exactly
what a split needs.

The cost is revocation: a token stays valid until it expires. That is why the
TTL is configuration and why sensitive operations re-read the live row (a
suspended member is refused at the gate even holding a valid token).

## Invariants the database enforces

Application code can have bugs. These rules are enforced where they cannot be
bypassed:

| Invariant | Mechanism |
|---|---|
| The ledger is append-only | `BEFORE UPDATE OR DELETE` trigger raising `restrict_violation` |
| The audit trail is append-only | Same trigger |
| Access logs are never deleted | `BEFORE DELETE` trigger (updates allowed, for conflict resolution) |
| One active booking per member per class | Partial unique index on the active statuses |
| An entry is reversed at most once | Partial unique index on `reverses_entry_id` |
| A payment's total is price minus discount | `CHECK` constraint |
| One live payout per coach per period | Partial unique index excluding `VOID` |
| Exactly one organization default incentive scheme | Partial unique index on `coach_id IS NULL` |
| A paid payout has a payment reference | `CHECK` constraint |
| One attendance row per employee per day | `UNIQUE (employee_id, date)` |
| A leave allowance is never overspent | `CHECK (annual_used <= annual_total)` |
| A rejected leave request carries a reason | `CHECK` constraint |
| One employee record per coach | Partial unique index on `coach_id` |
| The stock ledger is append-only | `BEFORE UPDATE OR DELETE` trigger |
| A stock movement's arithmetic holds | `CHECK (qty_after = qty_before + qty)` |
| Stock never goes negative | `CHECK (qty_on_hand >= 0)` |
| More cannot be received than ordered | `CHECK (qty_received <= qty_ordered)` |
| More cannot be returned than accepted | `CHECK (qty_returned <= qty_accepted)` |
| The XP ledger is append-only | Same trigger |
| An event awards points at most once | Partial unique index on `idempotency_key` |
| Current + spent XP equals lifetime | `CHECK` constraint |
| A reward is not over-subscribed | `CHECK (stock_redeemed <= stock_total)` |
| One open till per cashier per branch | Partial unique index on `status = 'OPEN'` |
| Only a cash tender gives change | `CHECK (change_idr = 0 OR method = 'CASH')` |

`TestAppendOnlyLedgerIsEnforcedByTheDatabase` asserts the first one by issuing
raw SQL and expecting it to fail.

## Concurrency

- **Booking the last place**: the session row is locked `FOR UPDATE` before
  counting, so two simultaneous requests serialize. The partial unique index is
  the backstop.
- **A duplicated payment callback**: the payment row is locked `FOR UPDATE`;
  the second caller sees `PAID` and gets `409 ALREADY_PAID` instead of granting
  credits twice.
- **A replayed QR code**: consumption is a conditional `UPDATE ... WHERE
  consumed_at IS NULL`, and a zero-row result turns the decision into
  `TOKEN_CONSUMED` even mid-transaction.
- **Migrations on rollout**: a PostgreSQL advisory lock means one replica
  applies and the rest wait, then find nothing to do.

`InTxSerializable` is available with retry-on-`40001` for anything that needs
true serializable isolation later.

## Extracting a module

Say the wallet needs to scale independently.

**1. Run it separately.** No code change:

```bash
MODULES=wallet,incentives ./api
MODULES=catalog,identity,scheduling,access,reporting AUTO_MIGRATE=false ./api
```

Put a gateway in front routing `/api/me/topup`, `/api/payments/*`,
`/api/admin/payments/*`, `/api/admin/vouchers/*` to the first. Both processes
still share a database, so nothing has become eventually consistent yet. This
alone gives independent deploys and independent scaling.

**2. Split the database.** Point the wallet process at its own instance and
move the `wallet` schema. Nothing else queried those tables, so nothing else
breaks — that is what the no-cross-schema-joins rule bought.

**3. Replace the in-process ports with clients.** The consumers of wallet are
scheduling (`Balance`, `CoveredClassTypes`, `Forfeit`) and reporting. Write an
HTTP client satisfying those interfaces and change the wiring in
`internal/app`. No module's own code changes.

**4. Handle what stops being atomic.** One place matters: the gate deducts a
credit and checks a booking in one transaction. Once wallet is remote that is
no longer possible, and the honest options are:

- have the access module publish `visit.recorded` and let wallet deduct
  asynchronously, accepting a brief window where a member has entered but the
  credit has not moved; or
- keep wallet and access together, which is why they are adjacent in the module
  list.

This is the real cost of the split, and it is better to see it written down
than to discover it in production. Every other seam is mechanical.

## Adding a module

1. `migrations/00NN_<name>.sql` creating `CREATE SCHEMA <name>`.
2. `internal/modules/<name>/` with `repository.go`, `service.go`, `http.go`.
3. Declare the ports the module needs as interfaces in its own package.
4. Register it in `internal/app/app.go` behind a `Modules.IsEnabled` check and
   add its name to the constants.

`hris` is the most recent worked example: schema in `0010_hris.sql`, rules in
`internal/domain/hris.go`, the module in `internal/modules/hris`, and four lines
in `internal/app/app.go`. It reads no other schema and reaches catalog through a
two-method port for branch and coach names.

## What is deliberately not here

- **An ORM.** Queries are SQL. The performance-sensitive paths (a balance, a
  session's counts) are single aggregate queries rather than loading rows to
  count them in Go.
- **A web framework.** `net/http`'s `ServeMux` handles method and path
  patterns since Go 1.22, so the router is 100 lines and the module handlers
  have no framework types in their signatures.
- **A DI container.** `internal/app.New` is a hundred lines of explicit
  construction, in dependency order, which is easier to follow than any
  container's graph.
- **Generated code.** Nothing here needs regenerating before it will compile.

The only third-party dependency is `jackc/pgx`.
