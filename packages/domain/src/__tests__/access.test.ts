import { describe, expect, it } from 'vitest';
import type { Member, QrToken } from '../index';
import {
  DEFAULT_BUSINESS_RULES,
  checkQrToken,
  evaluateGateScan,
  issueQrToken,
  qrSecondsRemaining,
} from '../index';

const NOW = '2026-01-10T17:00:00.000Z';

const activeMember: Member = {
  id: 'mem_1',
  fullName: 'Fahmi',
  email: 'f@example.com',
  phone: '+62812',
  dateOfBirth: null,
  gender: null,
  emergencyContact: null,
  preferredBranchId: null,
  avatarUrl: null,
  status: 'ACTIVE',
  waiverVersion: 'v1',
  waiverAcceptedAt: NOW,
  notes: null,
  createdAt: NOW,
  updatedAt: NOW,
};

const freshToken = (): QrToken =>
  issueQrToken({ memberId: 'mem_1', now: NOW, ttlSeconds: 45, nonce: 'abc' });

const baseArgs = {
  tokenCheck: checkQrToken(freshToken(), NOW),
  member: activeMember,
  balance: 5,
  lastAllowedEntryAt: null,
  candidateBooking: null,
  rules: DEFAULT_BUSINESS_RULES,
  now: NOW,
};

describe('QR token', () => {
  it('issues with the configured TTL', () => {
    const t = freshToken();
    expect(qrSecondsRemaining(t, NOW)).toBe(45);
    expect(t.token).toBe('qr_abc');
  });
  it('rejects expired / consumed / missing tokens', () => {
    const t = freshToken();
    expect(checkQrToken(t, '2026-01-10T17:01:00.000Z')).toMatchObject({
      ok: false,
      error: { reason: 'EXPIRED' },
    });
    expect(checkQrToken({ ...t, consumedAt: NOW }, NOW)).toMatchObject({
      ok: false,
      error: { reason: 'CONSUMED' },
    });
    expect(checkQrToken(null, NOW)).toMatchObject({ ok: false, error: { reason: 'NOT_FOUND' } });
  });
});

describe('evaluateGateScan', () => {
  it('denies invalid tokens with the mapped reason', () => {
    const evalExpired = evaluateGateScan({
      ...baseArgs,
      tokenCheck: checkQrToken(freshToken(), '2026-01-10T18:00:00.000Z'),
    });
    expect(evalExpired).toMatchObject({ decision: 'DENIED', reason: 'TOKEN_EXPIRED' });
    const evalMissing = evaluateGateScan({ ...baseArgs, tokenCheck: checkQrToken(null, NOW) });
    expect(evalMissing).toMatchObject({ decision: 'DENIED', reason: 'TOKEN_INVALID' });
  });

  it('denies non-active members', () => {
    const res = evaluateGateScan({
      ...baseArgs,
      member: { ...activeMember, status: 'SUSPENDED' },
    });
    expect(res).toMatchObject({ decision: 'DENIED', reason: 'MEMBER_NOT_ACTIVE' });
  });

  it('open-gym entry deducts the configured cost', () => {
    const res = evaluateGateScan(baseArgs);
    expect(res.decision).toBe('ALLOWED');
    expect(res.entryKind).toBe('OPEN_GYM');
    expect(res.effects).toContainEqual({
      kind: 'DEDUCT_CREDITS',
      amount: 1,
      description: 'Studio entry',
    });
  });

  it('booked entry deducts the session cost and checks the booking in', () => {
    const res = evaluateGateScan({
      ...baseArgs,
      candidateBooking: { id: 'bok_1', creditCost: 2 },
    });
    expect(res.entryKind).toBe('BOOKING');
    expect(res.effects).toContainEqual({ kind: 'CHECK_IN_BOOKING', bookingId: 'bok_1' });
    expect(res.effects).toContainEqual({
      kind: 'DEDUCT_CREDITS',
      amount: 2,
      description: 'Class check-in',
    });
  });

  it('denies on insufficient credits (booking and open gym)', () => {
    expect(
      evaluateGateScan({ ...baseArgs, balance: 1, candidateBooking: { id: 'b', creditCost: 2 } }),
    ).toMatchObject({ decision: 'DENIED', reason: 'INSUFFICIENT_CREDITS' });
    expect(evaluateGateScan({ ...baseArgs, balance: 0 })).toMatchObject({
      decision: 'DENIED',
      reason: 'INSUFFICIENT_CREDITS',
    });
  });

  it('allows free re-entry within the grace period', () => {
    const res = evaluateGateScan({
      ...baseArgs,
      lastAllowedEntryAt: '2026-01-10T16:50:00.000Z', // 10 min ago < 15 min grace
    });
    expect(res).toMatchObject({ decision: 'ALLOWED', entryKind: 'RE_ENTRY' });
    expect(res.effects.some((e) => e.kind === 'DEDUCT_CREDITS')).toBe(false);
  });

  it('denies anti-passback between grace and passback window', () => {
    const res = evaluateGateScan({
      ...baseArgs,
      lastAllowedEntryAt: '2026-01-10T16:30:00.000Z', // 30 min ago: > 15 grace, < 60 passback
    });
    expect(res).toMatchObject({ decision: 'DENIED', reason: 'ANTI_PASSBACK' });
  });

  it('allows a fresh entry after the anti-passback window', () => {
    const res = evaluateGateScan({
      ...baseArgs,
      lastAllowedEntryAt: '2026-01-10T15:30:00.000Z', // 90 min ago > 60 passback
    });
    expect(res).toMatchObject({ decision: 'ALLOWED', entryKind: 'OPEN_GYM' });
  });

  it('honors configured windows (rules are data, not code)', () => {
    const res = evaluateGateScan({
      ...baseArgs,
      rules: { ...DEFAULT_BUSINESS_RULES, reEntryGraceMinutes: 45 },
      lastAllowedEntryAt: '2026-01-10T16:30:00.000Z',
    });
    expect(res).toMatchObject({ decision: 'ALLOWED', entryKind: 'RE_ENTRY' });
  });

  it('every evaluation with a readable token consumes it — even denials', () => {
    const res = evaluateGateScan({ ...baseArgs, balance: 0 });
    expect(res.effects).toContainEqual({ kind: 'CONSUME_TOKEN', token: 'qr_abc' });
  });
});
