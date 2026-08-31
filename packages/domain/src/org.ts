import type { AdminRole } from './rbac';
import type { BusinessRules } from './rules';

export interface Organization {
  id: string;
  name: string;
}

export interface Branch {
  id: string;
  organizationId: string;
  name: string;
  address: string;
  timezone: string;
  operatingHours: string;
  status: 'ACTIVE' | 'INACTIVE';
  managerName: string | null;
  /** Branch-level overrides on top of org business-rule defaults. */
  rulesOverride: Partial<BusinessRules> | null;
}

export interface Gate {
  id: string;
  branchId: string;
  name: string;
  status: 'ONLINE' | 'OFFLINE';
}

export interface Coach {
  id: string;
  name: string;
  bio: string;
  specialization: string;
  branchId: string;
  status: 'ACTIVE' | 'INACTIVE';
}

export interface AdminUser {
  id: string;
  name: string;
  email: string;
  role: AdminRole;
  /** null = all branches (HQ-level user). */
  branchId: string | null;
}
