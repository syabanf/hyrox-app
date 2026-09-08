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
  'incentives.view',
  'incentives.manage',
  'config.view',
  'branches.manage',
  'gates.manage',
  'users.manage',
  'rules.update',
  // The people side. Employee records carry home addresses, bank accounts and
  // next of kin, so 'hris.view' is a narrower grant than it looks.
  'hris.view',
  'hris.manage',
  'hris.attendance',
  'hris.approve',
  // Stock. Looking is broad — the front desk has to know whether a shirt is on
  // the shelf — but changing a quantity is not.
  'inventory.view',
  'inventory.manage',
  'inventory.count',
  // Buying. Raising, approving and receiving are three separate grants,
  // because one person doing all three is how invoices get paid for goods that
  // never arrived.
  'purchasing.view',
  'purchasing.manage',
  'purchasing.approve',
  'purchasing.receive',
  // Paying a supplier is a finance act, not a buying one: the person who
  // chooses what to order should not also move the money out.
  'purchasing.pay',
  // The till. Selling is the counter's job; unwinding a paid sale is not.
  'pos.view',
  'pos.sell',
  'pos.manage',
  'pos.void',
  // Loyalty. Handing out points by hand is its own grant, for the same reason
  // adjusting somebody's credits is.
  'crm.view',
  'crm.manage',
  'crm.approve',
  'crm.adjust',
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
    'incentives.view',
    'config.view',
    'hris.view',
    'hris.attendance',
    'hris.approve',
    'inventory.view',
    'inventory.manage',
    'inventory.count',
    // A branch manager is the first signature on the chain, and the person who
    // signs for goods arriving at their own branch.
    'purchasing.view',
    'purchasing.manage',
    'purchasing.approve',
    'purchasing.receive',
    'pos.view',
    'pos.sell',
    'pos.manage',
    'pos.void',
    'crm.view',
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
    // They run the till and answer "have you got this in a medium", so they
    // sell and they look. Voiding a paid sale needs a manager, and changing a
    // stock figure is not a counter job.
    'pos.view',
    'pos.sell',
    'inventory.view',
    'crm.view',
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
    'incentives.view',
    'incentives.manage',
    'hris.view',
    // Finance is the second signature on a purchase and the reason stock
    // valuation exists, but never touches a quantity or a till.
    'inventory.view',
    'purchasing.view',
    'purchasing.approve',
    // And the only role besides the super admin that may actually pay one.
    'purchasing.pay',
    'pos.view',
    'crm.view',
  ],
};

export function hasPermission(role: AdminRole, permission: Permission): boolean {
  return ROLE_PERMISSIONS[role].includes(permission);
}
