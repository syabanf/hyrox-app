/**
 * Human-readable copy for gate outcomes, shared by the member and admin apps.
 * Keyed by string so the UI package stays free of domain imports; unknown
 * codes fall back to a de-snaked version of the code itself.
 */
const GATE_REASON_LABELS: Record<string, string> = {
  TOKEN_INVALID: 'QR code not recognised - refresh and try again',
  TOKEN_EXPIRED: 'QR code expired - refresh and try again',
  TOKEN_CONSUMED: 'QR code already used - refresh and try again',
  MEMBER_NOT_ACTIVE: 'Membership is not active - see the front desk',
  ANTI_PASSBACK: 'Already checked in recently - re-entry window closed',
  NO_BOOKING: 'No class booked for now - book a session first',
  INSUFFICIENT_CREDITS: 'Not enough credits for this class - top up first',
};

const GATE_ENTRY_KIND_LABELS: Record<string, string> = {
  BOOKING: 'Class check-in',
  RE_ENTRY: 'Re-entry (free, within grace window)',
};

const deSnake = (code: string): string => code.replaceAll('_', ' ').toLowerCase();

export function gateReasonLabel(reason: string): string {
  return GATE_REASON_LABELS[reason] ?? deSnake(reason);
}

export function gateEntryKindLabel(kind: string): string {
  return GATE_ENTRY_KIND_LABELS[kind] ?? deSnake(kind);
}
