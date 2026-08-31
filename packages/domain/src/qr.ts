import type { Result } from './shared/result';
import { err, ok } from './shared/result';
import type { IsoDate } from './shared/time';
import { addSecondsIso, isAfter, secondsBetween } from './shared/time';

/**
 * DYNAMIC QR ≠ MEMBER ID: the QR encodes a short-lived, single-use access
 * credential, never a permanent identifier.
 */
export interface QrToken {
  token: string;
  memberId: string;
  issuedAt: IsoDate;
  expiresAt: IsoDate;
  consumedAt: IsoDate | null;
}

export function issueQrToken(args: {
  memberId: string;
  now: IsoDate;
  ttlSeconds: number;
  nonce: string;
}): QrToken {
  return {
    token: `qr_${args.nonce}`,
    memberId: args.memberId,
    issuedAt: args.now,
    expiresAt: addSecondsIso(args.now, args.ttlSeconds),
    consumedAt: null,
  };
}

export type QrTokenProblem = 'NOT_FOUND' | 'EXPIRED' | 'CONSUMED';

export function checkQrToken(
  token: QrToken | null | undefined,
  now: IsoDate,
): Result<QrToken, { reason: QrTokenProblem }> {
  if (!token) return err({ reason: 'NOT_FOUND' });
  if (token.consumedAt !== null) return err({ reason: 'CONSUMED' });
  if (isAfter(now, token.expiresAt)) return err({ reason: 'EXPIRED' });
  return ok(token);
}

export function qrSecondsRemaining(token: QrToken, now: IsoDate): number {
  return Math.max(0, Math.floor(secondsBetween(now, token.expiresAt)));
}
