/**
 * Business rules the quotation leaves to discovery. Stored as configuration,
 * never hard-coded; branches may override org defaults.
 */
export interface BusinessRules {
  openGymCreditCost: number;
  defaultCreditExpiryDays: number;
  cancellationDeadlineHours: number;
  lateCancellationPolicy: 'FORFEIT' | 'FREE';
  noShowPolicy: 'FORFEIT' | 'FREE';
  reEntryGraceMinutes: number;
  antiPassbackMinutes: number;
  qrTtlSeconds: number;
  waitlistAutoPromote: boolean;
  lowBalanceThreshold: number;
  expiryReminderDays: number;
  bookingOpensDaysBefore: number;
  bookingClosesMinutesBefore: number;
}

export const DEFAULT_BUSINESS_RULES: BusinessRules = {
  openGymCreditCost: 1,
  defaultCreditExpiryDays: 60,
  cancellationDeadlineHours: 4,
  lateCancellationPolicy: 'FORFEIT',
  noShowPolicy: 'FORFEIT',
  reEntryGraceMinutes: 15,
  antiPassbackMinutes: 60,
  qrTtlSeconds: 45,
  waitlistAutoPromote: true,
  lowBalanceThreshold: 3,
  expiryReminderDays: 7,
  bookingOpensDaysBefore: 7,
  bookingClosesMinutesBefore: 0,
};

export function resolveRules(
  base: BusinessRules,
  override?: Partial<BusinessRules> | null,
): BusinessRules {
  return override ? { ...base, ...override } : base;
}
