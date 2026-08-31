import type {
  CreditLedgerEntry,
  Payment,
  PaymentChannel,
  Result,
  TopUpLot,
  Voucher,
} from '@hyrox/domain';
import {
  PAYMENT_TRANSITIONS,
  buildReversalEntry,
  computeExpiringCredits,
  deriveExpirationEntries,
  err,
  ok,
  transition,
  validateVoucher,
  addDaysIso,
} from '@hyrox/domain';
import type { Actor, AppError } from '../common';
import { appError, balanceOf, notify, recordAudit } from '../common';
import type { UseCaseDeps } from '../ports';

/** A member counts as "new" for voucher segments until their first PAID top-up. */
function memberIsNew(deps: UseCaseDeps, memberId: string): boolean {
  return !deps.payments.forMember(memberId).some((p) => p.status === 'PAID');
}

/** Lazily expire credits whose lots are past due. Idempotent. */
export function sweepMemberExpiry(deps: UseCaseDeps, memberId: string): CreditLedgerEntry[] {
  const drafts = deriveExpirationEntries(
    deps.ledger.lotsFor(memberId),
    deps.ledger.forMember(memberId),
    deps.clock.now(),
  );
  const appended: CreditLedgerEntry[] = [];
  for (const draft of drafts) {
    const entry: CreditLedgerEntry = {
      id: deps.ids.next('led'),
      memberId: draft.memberId,
      type: draft.type,
      amount: draft.amount,
      description: draft.description,
      sourceType: draft.sourceType,
      sourceId: draft.sourceId,
      reversesEntryId: null,
      actorId: null,
      reason: null,
      createdAt: deps.clock.now(),
    };
    deps.ledger.append(entry);
    appended.push(entry);
    notify(
      deps,
      memberId,
      'CREDIT_EXPIRY',
      'Credits expired',
      `${-draft.amount} credit${draft.amount === -1 ? '' : 's'} expired.`,
    );
  }
  return appended;
}

export function runExpirySweep(deps: UseCaseDeps): { affectedMembers: number; entries: number } {
  let affected = 0;
  let entries = 0;
  for (const member of deps.members.all()) {
    const appended = sweepMemberExpiry(deps, member.id);
    if (appended.length > 0) {
      affected += 1;
      entries += appended.length;
    }
  }
  return { affectedMembers: affected, entries };
}

export function expiringCreditsFor(deps: UseCaseDeps, memberId: string): number {
  return computeExpiringCredits(
    deps.ledger.lotsFor(memberId),
    deps.ledger.forMember(memberId),
    deps.clock.now(),
    deps.rules.defaults().expiryReminderDays,
  );
}

export function quoteVoucher(
  deps: UseCaseDeps,
  args: { memberId: string; code: string; packageId: string },
): Result<{ voucher: Voucher; discountIdr: number }, AppError> {
  const voucher = deps.vouchers.byCode(args.code);
  if (!voucher) return err(appError('VOUCHER_NOT_FOUND', 'Voucher code not recognized.', 404));
  const pkg = deps.packages.byId(args.packageId);
  if (!pkg) return err(appError('PACKAGE_NOT_FOUND', 'Package not found.', 404));
  const res = validateVoucher({
    voucher,
    pkg,
    memberIsNew: memberIsNew(deps, args.memberId),
    memberRedemptionCount: deps.vouchers.memberRedemptionCount(voucher.id, args.memberId),
    totalRedemptionCount: deps.vouchers.redemptionCount(voucher.id),
    now: deps.clock.now(),
  });
  if (!res.ok) {
    return err(appError(`VOUCHER_${res.error.reason}`, voucherRejectionMessage(res.error.reason)));
  }
  return ok({ voucher, discountIdr: res.value.discountIdr });
}

function voucherRejectionMessage(reason: string): string {
  const messages: Record<string, string> = {
    NOT_ACTIVE: 'This voucher is not active.',
    NOT_STARTED: 'This voucher is not valid yet.',
    ENDED: 'This voucher has ended.',
    USAGE_LIMIT_REACHED: 'This voucher has been fully redeemed.',
    PER_MEMBER_LIMIT_REACHED: 'You have already used this voucher.',
    PACKAGE_NOT_ELIGIBLE: 'This voucher does not apply to the selected package.',
    SEGMENT_NOT_ELIGIBLE: 'This voucher is reserved for new members.',
  };
  return messages[reason] ?? 'Voucher cannot be applied.';
}

export function topUpWallet(
  deps: UseCaseDeps,
  args: {
    memberId: string;
    packageId: string;
    voucherCode: string | null;
    channel: PaymentChannel;
  },
): Result<{ payment: Payment; discountIdr: number }, AppError> {
  const member = deps.members.byId(args.memberId);
  if (!member) return err(appError('NOT_FOUND', 'Member not found.', 404));
  if (member.status !== 'ACTIVE')
    return err(appError('MEMBER_NOT_ACTIVE', 'Membership is not active.'));
  const pkg = deps.packages.byId(args.packageId);
  if (!pkg || pkg.status !== 'ACTIVE')
    return err(appError('PACKAGE_NOT_AVAILABLE', 'This package is not available.', 404));
  if (pkg.purchaseLimitPerMember !== null) {
    const purchases = deps.payments
      .forMember(args.memberId)
      .filter((p) => p.packageId === pkg.id && p.status === 'PAID').length;
    if (purchases >= pkg.purchaseLimitPerMember)
      return err(appError('PURCHASE_LIMIT_REACHED', 'Purchase limit reached for this package.'));
  }

  let discountIdr = 0;
  if (args.voucherCode) {
    const quote = quoteVoucher(deps, {
      memberId: args.memberId,
      code: args.voucherCode,
      packageId: args.packageId,
    });
    if (!quote.ok) return quote;
    discountIdr = quote.value.discountIdr;
  }

  const payment: Payment = {
    id: deps.ids.next('pay'),
    memberId: args.memberId,
    packageId: pkg.id,
    credits: pkg.credits,
    amountIdr: pkg.priceIdr,
    discountIdr,
    totalIdr: pkg.priceIdr - discountIdr,
    voucherCode: args.voucherCode,
    channel: args.channel,
    status: 'PENDING',
    createdAt: deps.clock.now(),
    paidAt: null,
    refundedAt: null,
  };
  deps.payments.save(payment);
  return ok({ payment, discountIdr });
}

/** Mock Xendit webhook: PENDING → PAID, then the ledger entry + lot are created. */
export function confirmPayment(
  deps: UseCaseDeps,
  paymentId: string,
): Result<{ payment: Payment; entry: CreditLedgerEntry; lot: TopUpLot }, AppError> {
  const payment = deps.payments.byId(paymentId);
  if (!payment) return err(appError('NOT_FOUND', 'Payment not found.', 404));
  const res = transition(PAYMENT_TRANSITIONS, payment.status, 'PAID');
  if (!res.ok)
    return err(appError('INVALID_TRANSITION', `Payment is ${payment.status}, cannot mark PAID.`));
  const now = deps.clock.now();
  payment.status = 'PAID';
  payment.paidAt = now;
  deps.payments.save(payment);

  const pkg = deps.packages.byId(payment.packageId);
  const entry: CreditLedgerEntry = {
    id: deps.ids.next('led'),
    memberId: payment.memberId,
    type: 'TOP_UP',
    amount: payment.credits,
    description: `Top up — ${pkg?.name ?? payment.packageId}`,
    sourceType: 'PAYMENT',
    sourceId: payment.id,
    reversesEntryId: null,
    actorId: null,
    reason: null,
    createdAt: now,
  };
  deps.ledger.append(entry);
  const lot: TopUpLot = {
    id: deps.ids.next('lot'),
    memberId: payment.memberId,
    ledgerEntryId: entry.id,
    credits: payment.credits,
    expiresAt: addDaysIso(now, pkg?.validityDays ?? deps.rules.defaults().defaultCreditExpiryDays),
    createdAt: now,
  };
  deps.ledger.addLot(lot);

  if (payment.voucherCode) {
    const voucher = deps.vouchers.byCode(payment.voucherCode);
    if (voucher) {
      deps.vouchers.addRedemption({
        id: deps.ids.next('red'),
        voucherId: voucher.id,
        memberId: payment.memberId,
        paymentId: payment.id,
        discountIdr: payment.discountIdr,
        createdAt: now,
      });
    }
  }

  notify(
    deps,
    payment.memberId,
    'ANNOUNCEMENT',
    'Top up successful',
    `${payment.credits} credits added to your wallet.`,
  );
  return ok({ payment, entry, lot });
}

export function failPayment(
  deps: UseCaseDeps,
  paymentId: string,
  to: 'FAILED' | 'EXPIRED',
): Result<Payment, AppError> {
  const payment = deps.payments.byId(paymentId);
  if (!payment) return err(appError('NOT_FOUND', 'Payment not found.', 404));
  const res = transition(PAYMENT_TRANSITIONS, payment.status, to);
  if (!res.ok)
    return err(appError('INVALID_TRANSITION', `Payment is ${payment.status}, cannot mark ${to}.`));
  payment.status = to;
  deps.payments.save(payment);
  return ok(payment);
}

/** Money refund → payment REFUNDED + a REVERSAL of its TOP_UP ledger entry. */
export function refundPayment(
  deps: UseCaseDeps,
  args: { paymentId: string; actor: Actor; reason: string },
): Result<{ payment: Payment; reversal: CreditLedgerEntry | null }, AppError> {
  const payment = deps.payments.byId(args.paymentId);
  if (!payment) return err(appError('NOT_FOUND', 'Payment not found.', 404));
  const res = transition(PAYMENT_TRANSITIONS, payment.status, 'REFUNDED');
  if (!res.ok)
    return err(appError('INVALID_TRANSITION', `Payment is ${payment.status}, cannot refund.`));
  const now = deps.clock.now();
  payment.status = 'REFUNDED';
  payment.refundedAt = now;
  deps.payments.save(payment);

  const original = deps.ledger
    .forMember(payment.memberId)
    .find((e) => e.type === 'TOP_UP' && e.sourceId === payment.id);
  let reversal: CreditLedgerEntry | null = null;
  if (original) {
    const draft = buildReversalEntry(original, {
      actorId: args.actor.id,
      reason: args.reason,
      alreadyReversed: deps.ledger.hasReversalOf(original.id),
    });
    if (draft.ok) {
      reversal = { ...draft.value, id: deps.ids.next('led'), createdAt: now };
      deps.ledger.append(reversal);
    }
  }
  recordAudit(deps, {
    entityType: 'PAYMENT',
    entityId: payment.id,
    action: 'REFUND',
    previousValue: 'PAID',
    newValue: 'REFUNDED',
    actor: args.actor,
    reason: args.reason,
  });
  return ok({ payment, reversal });
}

export function adjustCredits(
  deps: UseCaseDeps,
  args: { memberId: string; amount: number; reason: string; actor: Actor },
): Result<CreditLedgerEntry, AppError> {
  const member = deps.members.byId(args.memberId);
  if (!member) return err(appError('NOT_FOUND', 'Member not found.', 404));
  const entry: CreditLedgerEntry = {
    id: deps.ids.next('led'),
    memberId: args.memberId,
    type: 'ADJUSTMENT',
    amount: args.amount,
    description: `Manual adjustment (${args.amount > 0 ? '+' : ''}${args.amount})`,
    sourceType: 'ADMIN',
    sourceId: null,
    reversesEntryId: null,
    actorId: args.actor.id,
    reason: args.reason,
    createdAt: deps.clock.now(),
  };
  deps.ledger.append(entry);
  recordAudit(deps, {
    entityType: 'LEDGER',
    entityId: entry.id,
    action: 'ADJUSTMENT',
    newValue: String(args.amount),
    actor: args.actor,
    reason: args.reason,
  });
  return ok(entry);
}

export function reverseEntry(
  deps: UseCaseDeps,
  args: { entryId: string; reason: string; actor: Actor },
): Result<CreditLedgerEntry, AppError> {
  const original = deps.ledger.byId(args.entryId);
  if (!original) return err(appError('NOT_FOUND', 'Ledger entry not found.', 404));
  const draft = buildReversalEntry(original, {
    actorId: args.actor.id,
    reason: args.reason,
    alreadyReversed: deps.ledger.hasReversalOf(original.id),
  });
  if (!draft.ok) {
    const messages = {
      CANNOT_REVERSE_REVERSAL: 'A reversal cannot be reversed.',
      ALREADY_REVERSED: 'This entry has already been reversed.',
    } as const;
    return err(appError(draft.error.type, messages[draft.error.type]));
  }
  const reversal: CreditLedgerEntry = {
    ...draft.value,
    id: deps.ids.next('led'),
    createdAt: deps.clock.now(),
  };
  deps.ledger.append(reversal);
  recordAudit(deps, {
    entityType: 'LEDGER',
    entityId: original.id,
    action: 'REVERSAL',
    previousValue: String(original.amount),
    newValue: String(reversal.amount),
    actor: args.actor,
    reason: args.reason,
  });
  return ok(reversal);
}

export function walletSnapshot(deps: UseCaseDeps, memberId: string) {
  sweepMemberExpiry(deps, memberId);
  const rules = deps.rules.defaults();
  const balance = balanceOf(deps, memberId);
  return {
    balance,
    expiringCredits: expiringCreditsFor(deps, memberId),
    lowBalance: balance < rules.lowBalanceThreshold,
    lowBalanceThreshold: rules.lowBalanceThreshold,
    entries: deps.ledger.forMember(memberId),
    lots: deps.ledger.lotsFor(memberId),
  };
}
