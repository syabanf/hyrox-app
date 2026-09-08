# API reference

Base URL: `http://localhost:9080` in the default compose stack.

## Conventions

**Success** responses return the view object directly, unwrapped.

**Errors** always use the same envelope, which is what the web clients already
parse:

```json
{ "error": { "code": "INSUFFICIENT_CREDITS", "message": "You do not have enough credits for this class." } }
```

The `code` is stable and safe to branch on; the `message` is for people and may
change. Codes worth handling: `VALIDATION_FAILED`, `UNAUTHORIZED`, `FORBIDDEN`,
`NOT_FOUND`, `IN_USE`, `INVALID_TRANSITION`, `ALREADY_BOOKED`,
`INSUFFICIENT_CREDITS`, `PACKAGE_NOT_COVERED`, `SLOT_TAKEN`, `ALREADY_PAID`,
`ALREADY_REVERSED`, `VOUCHER_*`.

**Authentication** is a bearer token: `Authorization: Bearer <token>`. Member
and staff tokens are distinct; an endpoint says which it wants.

**Times** are RFC 3339 with an offset. Money is IDR as a plain integer.
Credits are signed integers.

Every request carries an `X-Request-Id` in the response, echoed from the
request when supplied.

---

## Public

| Method | Path | Notes |
|---|---|---|
| `GET` | `/health` | Liveness. |
| `GET` | `/ready` | Readiness: pings the database, reports mounted modules. |
| `GET` | `/api/branches` | Branch list. |
| `GET` | `/api/class-types` | Active class templates. |
| `GET` | `/api/packages` | Purchasable packages with `coverageNames`. |
| `GET` | `/api/sessions` | Schedule. Query: `branchId`, `from`, `to`, `limit`. With a member token each row also carries `myBooking`. Drafts never appear. |
| `GET` | `/api/sessions/{id}` | One class. |

## Sign-in

| Method | Path | Body | Returns |
|---|---|---|---|
| `POST` | `/api/auth/otp/request` | `{identifier}` | `{challengeId, memberExists, hint}` — plus `code` in demo mode |
| `POST` | `/api/auth/otp/verify` | `{challengeId, code}` | `{token, member, expiresAt}` |
| `POST` | `/api/auth/register` | `{fullName, email, phone, waiverAccepted, termsAccepted, ...}` | `{token, member, expiresAt}` |
| `GET` | `/api/admin/auth/users` | | Staff roster (public in demo mode only) |
| `POST` | `/api/admin/auth/login` | `{userId}` or `{email}` | `{token, user, permissions, expiresAt}` |

A code may be verified once; five wrong attempts lock the challenge.

## Member

All require a member token.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/me/profile` | The signed-in member. |
| `PATCH` | `/api/me` | Partial update. `null` clears a field; absent leaves it. |
| `GET` | `/api/me/wallet` | Balance, expiring credits, full ledger, lots, purchased packages. Sweeps expired credits first. |
| `POST` | `/api/me/topup` | `{packageId, voucherCode?, channel}` → `{payment, discountIdr, invoice}`. Creates a PENDING payment; no credits move yet. |
| `POST` | `/api/vouchers/validate` | `{code, packageId}` → `{voucher, discountIdr}` |
| `GET` | `/api/payments/{id}` | Own payment only; another member's returns 404. |
| `POST` | `/api/payments/{id}/simulate` | Stands in for the gateway webhook. Settles the payment, issues credits and the expiry lot. Owner or `payments.simulate` staff. |
| `GET` | `/api/me/bookings` | Bookings with class details. |
| `POST` | `/api/sessions/{id}/book` | → `{booking, decision}` where decision is `CONFIRMED` or `WAITLIST`. |
| `POST` | `/api/bookings/{id}/cancel` | → `{booking, outcome, penaltyCredits, promotedMemberName}`. Owner or `bookings.manage` staff. |
| `POST` | `/api/bookings/{id}/confirm-spot` | Accept an offered waitlist place. |
| `POST` | `/api/me/qr` | Mint an access credential: `{token, issuedAt, expiresAt, ttlSeconds}`. |
| `GET` | `/api/me/visits` | Own entry history. |
| `GET` | `/api/exercises` | Movement library plus substitution rules. |

## Gate

| Method | Path | Notes |
|---|---|---|
| `POST` | `/api/gates/{gateId}/scan` | `{qrToken}` — or `{memberId}` to simulate. |

A presented `qrToken` is self-authorizing: the token is the credential, which
is what lets scanner hardware call this. Naming a `memberId` instead requires
the member themselves, staff with `access.simulate`, or the gate's shared key.

The response is always `200` with the decision inside, because a refusal is a
valid answer:

```json
{
  "decision": "ALLOWED",
  "reason": null,
  "entryKind": "BOOKING",
  "memberName": "Fahmi Syaban",
  "remainingCredits": 14,
  "gateName": "Senopati Gate A",
  "accessLog": { "creditDelta": -1, "...": "..." }
}
```

`reason` on a refusal: `TOKEN_INVALID`, `TOKEN_EXPIRED`, `TOKEN_CONSUMED`,
`MEMBER_NOT_ACTIVE`, `ANTI_PASSBACK`, `NO_BOOKING`, `INSUFFICIENT_CREDITS`.
`entryKind` is `BOOKING` (charged) or `RE_ENTRY` (free, inside the grace
window).

## Admin

Every route needs a staff token and the named permission. A role lacking it
gets `403 FORBIDDEN` — checked server-side, not only in the UI.

### Members

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/members` | `members.view` — query: `query`, `status`, `limit`, `offset` |
| `GET` | `/api/admin/members/{id}` | `members.view` — the 360° view |
| `POST` | `/api/admin/members` | `members.manage` |
| `PATCH` | `/api/admin/members/{id}` | `members.manage` — status changes go through the state machine and are audited |
| `POST` | `/api/admin/members/{id}/adjust` | `members.adjust_credits` — `{amount, reason}`; a reason is required |
| `POST` | `/api/admin/ledger/{entryId}/reverse` | `ledger.reverse` — `{reason}` |

### Operations

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/sessions` | `operations.view` — includes drafts |
| `GET` | `/api/admin/sessions/{id}` | `operations.view` — with roster |
| `POST` `PATCH` `DELETE` | `/api/admin/sessions[/{id}]` | `sessions.manage` |
| `POST` | `/api/admin/sessions/{id}/publish\|cancel\|complete` | `sessions.manage` |
| `POST` | `/api/admin/bookings` | `bookings.manage` — `{memberId, sessionId}` |
| `POST` | `/api/admin/bookings/{id}/check-in` | `attendance.manage` — charges the class |
| `POST` | `/api/admin/bookings/{id}/no-show` | `attendance.manage` — applies the policy |
| `GET` `POST` `PATCH` `DELETE` | `/api/admin/class-types[/{id}]` | `operations.view` / `class_types.manage` |
| `GET` `POST` `PATCH` `DELETE` | `/api/admin/coaches[/{id}]` | `operations.view` / `coaches.manage` |
| `GET` | `/api/admin/exercises` | `operations.view` |
| `PATCH` | `/api/admin/exercises/{id}` | `class_types.manage` — name, difficulty, how-to video |

### Commercial

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/packages` | `commercial.view` — with sales figures |
| `POST` `PATCH` `DELETE` | `/api/admin/packages[/{id}]` | `packages.manage` |
| `GET` | `/api/admin/payments` | `payments.view` — query: `memberId`, `status`, `limit` |
| `POST` | `/api/admin/payments/{id}/refund` | `refunds.manage` — `{reason}`; reverses the credits |
| `GET` | `/api/admin/vouchers` | `commercial.view` |
| `POST` `PATCH` `DELETE` | `/api/admin/vouchers[/{id}]` | `vouchers.manage` — created as DRAFT |
| `POST` | `/api/admin/vouchers/{id}/status` | `vouchers.manage` — `{status}`, validated against the lifecycle |

### Access

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/gates` | `access.view` |
| `POST` `PATCH` `DELETE` | `/api/admin/gates[/{id}]` | `gates.manage` |
| `GET` | `/api/admin/access-logs` | `access.view` — query: `memberId`, `branchId`, `gateId`, `result`, `mode`, `limit`. `result=ALLOWED` also matches offline and synced entries |
| `POST` | `/api/admin/access-logs/{id}/resolve` | `access.simulate` — `{action: APPROVE\|REJECT, reason}` |

### Coach incentives

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/incentives/schemes` | `incentives.view` |
| `POST` | `/api/admin/incentives/schemes` | `incentives.manage` |
| `PUT` | `/api/admin/incentives/schemes/{id}` | `incentives.manage` |
| `GET` | `/api/admin/incentives/statements` | `incentives.view` — `period=YYYY-MM` required, `branchId` optional |
| `GET` | `/api/admin/incentives/payouts` | `incentives.view` — query: `period`, `coachId`, `status` |
| `POST` | `/api/admin/incentives/payouts` | `incentives.manage` — `{coachId, periodMonth}`, freezes the statement |
| `POST` | `/api/admin/incentives/payouts/{id}/{approve\|pay\|void}` | `incentives.manage` — `pay` needs `paymentReference`, `void` needs `note` |

### People (HRIS)

The staff side: who works here, when they are expected, whether they turned up,
and the leave they are owed. Employee records carry home addresses, bank
accounts and next of kin, so there is no public or member-facing route here and
the front desk is not admitted at all.

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/hris/overview` | `hris.view` — the HR dashboard |
| `GET` | `/api/admin/hris/me` | *any staff token* — your own day |
| `POST` | `/api/admin/hris/me/clock-in\|clock-out` | *any staff token* |
| `GET` `POST` | `/api/admin/hris/departments` | `hris.view` / `hris.manage` |
| `PUT` `DELETE` | `/api/admin/hris/departments/{id}` | `hris.manage` |
| `GET` `POST` | `/api/admin/hris/positions` | `hris.view` / `hris.manage` |
| `PUT` `DELETE` | `/api/admin/hris/positions/{id}` | `hris.manage` |
| `GET` | `/api/admin/hris/employment-statuses` | `hris.view` |
| `GET` | `/api/admin/hris/employees` | `hris.view` — query: `query`, `departmentId`, `branchId`, `activeOnly`, `limit` |
| `GET` | `/api/admin/hris/employees/{id}` | `hris.view` — record, balance, pattern, timesheet, reports |
| `POST` `PUT` | `/api/admin/hris/employees[/{id}]` | `hris.manage` — a whole record, not a patch |
| `GET` `POST` | `/api/admin/hris/employees/{id}/schedule` | `hris.view` / `hris.manage` |
| `DELETE` | `/api/admin/hris/employees/{id}/schedule/{rowId}` | `hris.manage` |
| `GET` `PUT` | `/api/admin/hris/employees/{id}/balance` | `hris.view` / `hris.manage` |
| `GET` `POST` | `/api/admin/hris/shifts` | `hris.view` / `hris.manage` |
| `PUT` | `/api/admin/hris/shifts/{id}` | `hris.manage` |
| `GET` | `/api/admin/hris/roster` | `hris.view` — query: `date`, `branchId` |
| `GET` | `/api/admin/hris/attendance` | `hris.view` — query: `employeeId`, `from`, `to`, `status`, `limit` |
| `POST` | `/api/admin/hris/attendance/clock-in\|clock-out` | `hris.attendance` |
| `POST` | `/api/admin/hris/attendance/mark` | `hris.attendance` — record a day by hand |
| `GET` `POST` | `/api/admin/hris/leaves` | `hris.view` / `hris.attendance` |
| `POST` | `/api/admin/hris/leaves/{id}/{approve\|reject\|cancel}` | `hris.approve` |
| `GET` `POST` | `/api/admin/hris/overtime` | `hris.view` / `hris.attendance` |
| `POST` | `/api/admin/hris/overtime/{id}/{approve\|reject\|cancel}` | `hris.approve` |
| `GET` `POST` | `/api/admin/hris/holidays` | `hris.view` / `hris.manage` |
| `DELETE` | `/api/admin/hris/holidays/{id}` | `hris.manage` |

Codes worth handling here: `ALREADY_CLOCKED_IN`, `ALREADY_CLOCKED_OUT`,
`NOT_CLOCKED_IN`, `NOT_EMPLOYED`, `NO_WORKING_DAYS`, `OVERLAPS_EXISTING`,
`INSUFFICIENT_BALANCE`, `INVALID_TRANSITION`.

Three rules the numbers depend on:

- **Lateness is measured from the shift start, not from the end of the
  tolerance.** Twelve minutes into a shift with ten minutes' grace is twelve
  minutes late, not two.
- **A public holiday is free; collective leave is not.** `deductsLeave` on the
  calendar is what separates them, and where both land on one date the national
  holiday wins.
- **The allowance moves on approval, not on filing** — but pending days are
  reserved, so two requests cannot together overspend the year.

### Stock (Gudang)

Stock is held **per branch**, and movements are append-only: a miscount is
corrected with an ADJUSTMENT that says so, never by editing history.

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/inventory/overview` | `inventory.view` — valuation, low stock, recent movements |
| `GET` `POST` | `/api/admin/inventory/categories` | `inventory.view` / `inventory.manage` |
| `PUT` `DELETE` | `/api/admin/inventory/categories/{id}` | `inventory.manage` |
| `GET` | `/api/admin/inventory/items` | `inventory.view` — query: `query`, `categoryId`, `kind`, `activeOnly` |
| `GET` | `/api/admin/inventory/items/{id}` | `inventory.view` — item, levels, movements |
| `POST` `PUT` | `/api/admin/inventory/items[/{id}]` | `inventory.manage` |
| `PUT` | `/api/admin/inventory/items/{id}/reorder` | `inventory.manage` — minimum, maximum, bin |
| `GET` | `/api/admin/inventory/stock` | `inventory.view` — query: `branchId`, `categoryId`, `lowOnly` |
| `GET` | `/api/admin/inventory/movements` | `inventory.view` — the ledger |
| `POST` | `/api/admin/inventory/adjust` | `inventory.count` — signed `qty`, **`reason` required** |
| `POST` | `/api/admin/inventory/transfer` | `inventory.count` — two movements, one act |
| `GET` `POST` | `/api/admin/inventory/stock-takes` | `inventory.view` / `inventory.count` |
| `POST` | `/api/admin/inventory/stock-takes/{id}/count` | `inventory.count` |
| `POST` | `/api/admin/inventory/stock-takes/{id}/{apply\|cancel}` | `inventory.count` |

Codes: `INSUFFICIENT_STOCK`, `NOT_TRACKED`, `ITEM_INACTIVE`, `INVALID_TRANSITION`.

### Purchasing

Request → order → receipt, each a different grant. The approval chain is
decided by amount alone: every request needs a head's signature, finance joins
above Rp 5m and a director above Rp 25m.

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/purchasing/overview` | `purchasing.view` |
| `GET` `POST` | `/api/admin/purchasing/suppliers` | `purchasing.view` / `purchasing.manage` |
| `GET` `PUT` `DELETE` | `/api/admin/purchasing/suppliers/{id}` | `purchasing.view` / `purchasing.manage` |
| `GET` `POST` | `/api/admin/purchasing/suppliers/{id}/prices` | `purchasing.view` / `purchasing.manage` |
| `GET` `POST` | `/api/admin/purchasing/requests` | `purchasing.view` / `purchasing.manage` |
| `POST` `DELETE` | `/api/admin/purchasing/requests/{id}/lines[/{lineId}]` | `purchasing.manage` |
| `POST` | `/api/admin/purchasing/requests/{id}/submit` | `purchasing.manage` |
| `POST` | `/api/admin/purchasing/requests/{id}/{approve\|reject}` | `purchasing.approve` — signs whichever level is next, if the role reaches it |
| `POST` | `/api/admin/purchasing/requests/{id}/convert` | `purchasing.manage` |
| `GET` `POST` | `/api/admin/purchasing/orders` | `purchasing.view` / `purchasing.manage` |
| `PUT` | `/api/admin/purchasing/orders/{id}/terms` | `purchasing.manage` |
| `POST` `DELETE` | `/api/admin/purchasing/orders/{id}/lines[/{lineId}]` | `purchasing.manage` |
| `POST` | `/api/admin/purchasing/orders/{id}/approve` | `purchasing.approve` |
| `POST` | `/api/admin/purchasing/orders/{id}/{send\|cancel}` | `purchasing.manage` |
| `GET` `POST` | `/api/admin/purchasing/receipts` | `purchasing.view` / `purchasing.receive` |
| `POST` | `/api/admin/purchasing/receipts/{id}/lines` | `purchasing.receive` |
| `POST` | `/api/admin/purchasing/receipts/{id}/post` | `purchasing.receive` — **this is where stock moves** |
| `GET` `POST` | `/api/admin/purchasing/returns` | `purchasing.view` / `purchasing.receive` |
| `POST` | `/api/admin/purchasing/returns/{id}/{lines\|post}` | `purchasing.receive` |

Codes: `OVER_DELIVERED`, `OVER_RETURNED`, `ORDER_NOT_OPEN`, `NOT_DRAFT`,
`SUPPLIER_NOT_ORDERABLE`, `ITEM_NOT_IN_CATALOGUE`, `ALREADY_APPROVED`.

### Loyalty (CRM)

XP is not money: it cannot be topped up and it never pays for a class. Points
are earned from outbox events (a booking, a settled payment, a gate scan) with
the message id as an idempotency key, so a redelivery awards nothing.

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/crm/overview` | `crm.view` |
| `GET` | `/api/admin/crm/members` | `crm.view` — query: `tierCode`, `status` |
| `GET` | `/api/admin/crm/members/{id}` | `crm.view` — profile, ledger, claims |
| `POST` | `/api/admin/crm/members/{id}/adjust` | `crm.adjust` — **`reason` required** |
| `POST` | `/api/admin/crm/members/{id}/redeem` | `crm.approve` |
| `GET` | `/api/admin/crm/ledger` | `crm.view` |
| `GET` `PUT` | `/api/admin/crm/tiers` | `crm.view` / `crm.manage` |
| `GET` `PUT` | `/api/admin/crm/rules` | `crm.view` / `crm.manage` |
| `GET` `PUT` | `/api/admin/crm/rewards` | `crm.view` / `crm.manage` |
| `GET` | `/api/admin/crm/redemptions` | `crm.view` |
| `POST` | `/api/admin/crm/redemptions/{id}/{approve\|fulfil\|cancel}` | `crm.approve` |
| `GET` | `/api/me/loyalty` | *member token* — their own standing |

Codes: `INSUFFICIENT_XP`, `TIER_TOO_LOW`, `OUT_OF_STOCK`, `PER_MEMBER_LIMIT`,
`ALREADY_POSTED`, `PROFILE_SUSPENDED`.

### The till (POS)

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/pos/overview` | `pos.view` — today, month, best sellers, open tills |
| `GET` `PUT` | `/api/admin/pos/categories` | `pos.view` / `pos.manage` |
| `GET` `PUT` | `/api/admin/pos/products` | `pos.view` / `pos.manage` |
| `GET` `POST` | `/api/admin/pos/shifts` | `pos.view` / `pos.sell` |
| `POST` | `/api/admin/pos/shifts/{id}/close` | `pos.sell` — counts the drawer |
| `GET` `POST` | `/api/admin/pos/orders` | `pos.view` / `pos.sell` |
| `PUT` | `/api/admin/pos/orders/{id}` | `pos.sell` — customer, discount, note |
| `POST` `DELETE` | `/api/admin/pos/orders/{id}/lines[/{lineId}]` | `pos.sell` |
| `POST` | `/api/admin/pos/orders/{id}/tender` | `pos.sell` — several tenders is a split payment |
| `POST` | `/api/admin/pos/orders/{id}/complete` | `pos.sell` — **stock out, points on, sale closed** |
| `POST` | `/api/admin/pos/orders/{id}/cancel` | `pos.sell` — unpaid sales only |
| `POST` | `/api/admin/pos/orders/{id}/void` | `pos.void` — paid sales, **`reason` required** |

Codes: `NO_OPEN_SHIFT`, `SHIFT_ALREADY_OPEN`, `ORDERS_STILL_OPEN`,
`OVER_TENDERED`, `NOT_SETTLED`, `ALREADY_PAID`, `EMPTY_ORDER`,
`PRODUCT_NOT_SELLABLE`, `INSUFFICIENT_STOCK`.

### Reports and configuration

| Method | Path | Permission |
|---|---|---|
| `GET` | `/api/admin/reports/dashboard` | `dashboard.view` |
| `GET` | `/api/admin/reports/sales` | `reports.view` — `days` (default 30) |
| `GET` | `/api/admin/reports/visits` | `reports.view` — `days` |
| `GET` | `/api/admin/reports/credits` | `reports.view` |
| `GET` | `/api/admin/reports/classes` | `reports.view` — `days` (default 90) |
| `GET` | `/api/admin/audit` | `config.view` — `limit` |
| `GET` | `/api/admin/branches` | `config.view` |
| `POST` `PATCH` `DELETE` | `/api/admin/branches[/{id}]` | `branches.manage` |
| `POST` `PATCH` `DELETE` | `/api/admin/users[/{id}]` | `users.manage` — the last Super Admin cannot be removed or demoted |
| `GET` | `/api/admin/rules` | `config.view` — defaults plus branch overrides |
| `PUT` | `/api/admin/rules` | `rules.update` — partial; values are bounds-checked |

### Maintenance

| Method | Path | Notes |
|---|---|---|
| `POST` | `/api/dev/expiry-sweep` | Runs the credit expiry sweep. Open in development, `rules.update` otherwise. |

The sweep also runs on a five-minute background loop, alongside no-show
marking and credential cleanup, so calling it is only ever a convenience.

## Permissions by role

| | Super Admin | HQ Admin | Branch Manager | Front Desk | Coach | Finance |
|---|:--:|:--:|:--:|:--:|:--:|:--:|
| dashboard, members.view | Y | Y | Y | Y | Y | Y |
| members.manage | Y | Y | Y | – | – | – |
| members.adjust_credits | Y | Y | – | – | – | – |
| ledger.reverse | Y | Y | – | – | – | – |
| sessions/class types/coaches | Y | Y | Y | – | – | – |
| bookings.manage | Y | Y | Y | Y | – | – |
| attendance.manage | Y | Y | Y | Y | Y | – |
| access view/simulate | Y | Y | Y | Y | – | – |
| payments.view | Y | Y | – | Y | – | Y |
| refunds, payments.simulate | Y | Y | – | – | – | Y |
| packages, vouchers | Y | Y | – | – | – | – |
| reports.view | Y | Y | Y | – | – | Y |
| incentives.view | Y | Y | Y | – | – | Y |
| incentives.manage | Y | Y | – | – | – | Y |
| branches, gates | Y | Y | – | – | – | – |
| users.manage, rules.update | Y | – | – | – | – | – |
| hris.view | Y | Y | Y | – | – | Y |
| hris.attendance, hris.approve | Y | Y | Y | – | – | – |
| hris.manage | Y | Y | – | – | – | – |
| inventory.view | Y | Y | Y | Y | – | Y |
| inventory.manage, inventory.count | Y | Y | Y | – | – | – |
| purchasing.view, purchasing.approve | Y | Y | Y | – | – | Y |
| purchasing.manage, purchasing.receive | Y | Y | Y | – | – | – |
| pos.view, pos.sell | Y | Y | Y | Y | – | view only |
| pos.manage, pos.void | Y | Y | Y | – | – | – |
| crm.view | Y | Y | Y | Y | – | Y |
| crm.manage, crm.approve, crm.adjust | Y | Y | – | – | – | – |
