import { describe, expect, it } from 'vitest';
import { ADMIN_ROLES, PERMISSIONS, ROLE_PERMISSIONS, hasPermission, resolveRules, DEFAULT_BUSINESS_RULES } from '../index';

describe('RBAC', () => {
  it('super admin has every permission', () => {
    for (const p of PERMISSIONS) expect(hasPermission('SUPER_ADMIN', p)).toBe(true);
  });

  it('only super admin can update business rules or manage users', () => {
    for (const role of ADMIN_ROLES.filter((r) => r !== 'SUPER_ADMIN')) {
      expect(hasPermission(role, 'rules.update'), role).toBe(false);
      expect(hasPermission(role, 'users.manage'), role).toBe(false);
    }
  });

  it('front desk can support operations but not touch money rules', () => {
    expect(hasPermission('FRONT_DESK', 'bookings.manage')).toBe(true);
    expect(hasPermission('FRONT_DESK', 'access.simulate')).toBe(true);
    expect(hasPermission('FRONT_DESK', 'members.adjust_credits')).toBe(false);
    expect(hasPermission('FRONT_DESK', 'packages.manage')).toBe(false);
    expect(hasPermission('FRONT_DESK', 'reports.financial')).toBe(false);
  });

  it('finance sees money, not operations mutations', () => {
    expect(hasPermission('FINANCE', 'reports.financial')).toBe(true);
    expect(hasPermission('FINANCE', 'refunds.manage')).toBe(true);
    expect(hasPermission('FINANCE', 'sessions.manage')).toBe(false);
    expect(hasPermission('FINANCE', 'attendance.manage')).toBe(false);
  });

  it('coach is scoped to classes and attendance', () => {
    expect(hasPermission('COACH', 'attendance.manage')).toBe(true);
    expect(hasPermission('COACH', 'ledger.view')).toBe(false);
  });

  it('every role permission is a declared permission', () => {
    for (const role of ADMIN_ROLES) {
      for (const p of ROLE_PERMISSIONS[role]) expect(PERMISSIONS).toContain(p);
    }
  });
});

describe('resolveRules', () => {
  it('branch overrides win over defaults', () => {
    const merged = resolveRules(DEFAULT_BUSINESS_RULES, { qrTtlSeconds: 60 });
    expect(merged.qrTtlSeconds).toBe(60);
    expect(merged.antiPassbackMinutes).toBe(DEFAULT_BUSINESS_RULES.antiPassbackMinutes);
  });
  it('null override returns the defaults', () => {
    expect(resolveRules(DEFAULT_BUSINESS_RULES, null)).toEqual(DEFAULT_BUSINESS_RULES);
  });
});
