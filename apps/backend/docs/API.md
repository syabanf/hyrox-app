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
