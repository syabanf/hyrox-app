export const ADMIN_ROLES = [
  'SUPER_ADMIN',
  'HQ_ADMIN',
  'BRANCH_MANAGER',
  'FRONT_DESK',
  'COACH',
  'FINANCE',
] as const;
export type AdminRole = (typeof ADMIN_ROLES)[number];

/** Users are assigned roles, but everything internal checks fine-grained permissions. */
export const PERMISSIONS = [
  'dashboard.view',
  'members.view',
  'members.manage',
  'members.adjust_credits',
  'ledger.view',
  'ledger.reverse',
  'operations.view',
  'class_types.manage',
  'sessions.manage',
  'coaches.manage',
  'bookings.manage',
  'attendance.manage',
  'access.view',
  'access.simulate',
  'commercial.view',
  'packages.manage',
  'payments.view',
  'payments.simulate',
  'vouchers.manage',
  'refunds.manage',
  'engagement.view',
  'campaigns.manage',
  'reports.view',
  'reports.financial',
  'config.view',
  'branches.manage',
  'gates.manage',
  'users.manage',
  'rules.update',
] as const;
export type Permission = (typeof PERMISSIONS)[number];

const ALL: readonly Permission[] = PERMISSIONS;

export const ROLE_PERMISSIONS: Record<AdminRole, readonly Permission[]> = {
  SUPER_ADMIN: ALL,
  HQ_ADMIN: ALL.filter((p) => p !== 'users.manage' && p !== 'rules.update'),
  BRANCH_MANAGER: [
    'dashboard.view',
    'members.view',
    'members.manage',
    'ledger.view',
    'operations.view',
    'class_types.manage',
    'sessions.manage',
    'coaches.manage',
    'bookings.manage',
    'attendance.manage',
    'access.view',
    'access.simulate',
    'reports.view',
    'config.view',
  ],
  FRONT_DESK: [
    'dashboard.view',
    'members.view',
    'ledger.view',
    'operations.view',
    'bookings.manage',
    'attendance.manage',
    'access.view',
    'access.simulate',
    'commercial.view',
    'payments.view',
  ],
  COACH: ['dashboard.view', 'operations.view', 'attendance.manage', 'members.view'],
  FINANCE: [
    'dashboard.view',
    'members.view',
    'ledger.view',
    'commercial.view',
    'payments.view',
    'payments.simulate',
    'refunds.manage',
    'reports.view',
    'reports.financial',
  ],
};

export function hasPermission(role: AdminRole, permission: Permission): boolean {
  return ROLE_PERMISSIONS[role].includes(permission);
}
