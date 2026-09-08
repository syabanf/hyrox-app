package domain

// AdminRole is a staff account's role. Roles are coarse; everything internal
// checks a fine-grained permission so the matrix can change without touching
// call sites.
type AdminRole string

const (
	RoleSuperAdmin    AdminRole = "SUPER_ADMIN"
	RoleHQAdmin       AdminRole = "HQ_ADMIN"
	RoleBranchManager AdminRole = "BRANCH_MANAGER"
	RoleFrontDesk     AdminRole = "FRONT_DESK"
	RoleCoach         AdminRole = "COACH"
	RoleFinance       AdminRole = "FINANCE"
)

var AdminRoles = []AdminRole{
	RoleSuperAdmin, RoleHQAdmin, RoleBranchManager, RoleFrontDesk, RoleCoach, RoleFinance,
}

// Permission names a single capability. The string values are part of the API
// contract: the admin panel receives the caller's list at login and uses it to
// decide what to render.
type Permission string

const (
	PermDashboardView       Permission = "dashboard.view"
	PermMembersView         Permission = "members.view"
	PermMembersManage       Permission = "members.manage"
	PermMembersAdjustCredit Permission = "members.adjust_credits"
	PermLedgerView          Permission = "ledger.view"
	PermLedgerReverse       Permission = "ledger.reverse"
	PermOperationsView      Permission = "operations.view"
	PermClassTypesManage    Permission = "class_types.manage"
	PermSessionsManage      Permission = "sessions.manage"
	PermCoachesManage       Permission = "coaches.manage"
	PermBookingsManage      Permission = "bookings.manage"
	PermAttendanceManage    Permission = "attendance.manage"
	PermAccessView          Permission = "access.view"
	PermAccessSimulate      Permission = "access.simulate"
	PermCommercialView      Permission = "commercial.view"
	PermPackagesManage      Permission = "packages.manage"
	PermPaymentsView        Permission = "payments.view"
	PermPaymentsSimulate    Permission = "payments.simulate"
	PermVouchersManage      Permission = "vouchers.manage"
	PermRefundsManage       Permission = "refunds.manage"
	PermEngagementView      Permission = "engagement.view"
	PermCampaignsManage     Permission = "campaigns.manage"
	PermReportsView         Permission = "reports.view"
	PermReportsFinancial    Permission = "reports.financial"
	PermIncentivesView      Permission = "incentives.view"
	PermIncentivesManage    Permission = "incentives.manage"
	PermConfigView          Permission = "config.view"
	PermBranchesManage      Permission = "branches.manage"
	PermGatesManage         Permission = "gates.manage"
	PermUsersManage         Permission = "users.manage"
	PermRulesUpdate         Permission = "rules.update"
	// HRIS is staff administration: the people who run the studio rather than
	// the members who train in it.
	PermHRISView       Permission = "hris.view"
	PermHRISManage     Permission = "hris.manage"
	PermHRISAttendance Permission = "hris.attendance"
	PermHRISApprove    Permission = "hris.approve"
)

// Permissions is the full list, in the order the admin panel expects.
var Permissions = []Permission{
	PermDashboardView, PermMembersView, PermMembersManage, PermMembersAdjustCredit,
	PermLedgerView, PermLedgerReverse, PermOperationsView, PermClassTypesManage,
	PermSessionsManage, PermCoachesManage, PermBookingsManage, PermAttendanceManage,
	PermAccessView, PermAccessSimulate, PermCommercialView, PermPackagesManage,
	PermPaymentsView, PermPaymentsSimulate, PermVouchersManage, PermRefundsManage,
	PermEngagementView, PermCampaignsManage, PermReportsView, PermReportsFinancial,
	PermIncentivesView, PermIncentivesManage, PermConfigView, PermBranchesManage,
	PermGatesManage, PermUsersManage, PermRulesUpdate,
	PermHRISView, PermHRISManage, PermHRISAttendance, PermHRISApprove,
}

// RolePermissions is the authorization matrix. Super Admin holds everything;
// HQ Admin holds everything except the two that reshape the system itself
// (staff accounts and business rules).
var RolePermissions = map[AdminRole][]Permission{
	RoleSuperAdmin: Permissions,
	RoleHQAdmin:    without(Permissions, PermUsersManage, PermRulesUpdate),
	RoleBranchManager: {
		PermDashboardView, PermMembersView, PermMembersManage, PermLedgerView,
		PermOperationsView, PermClassTypesManage, PermSessionsManage, PermCoachesManage,
		PermBookingsManage, PermAttendanceManage, PermAccessView, PermAccessSimulate,
		PermReportsView, PermIncentivesView, PermConfigView,
		PermHRISView, PermHRISAttendance, PermHRISApprove,
	},
	RoleFrontDesk: {
		PermDashboardView, PermMembersView, PermLedgerView, PermOperationsView,
		PermBookingsManage, PermAttendanceManage, PermAccessView, PermAccessSimulate,
		PermCommercialView, PermPaymentsView,
	},
	RoleCoach: {
		PermDashboardView, PermOperationsView, PermAttendanceManage, PermMembersView,
	},
	RoleFinance: {
		PermDashboardView, PermMembersView, PermLedgerView, PermCommercialView,
		PermPaymentsView, PermPaymentsSimulate, PermRefundsManage, PermReportsView,
		PermReportsFinancial, PermIncentivesView, PermIncentivesManage,
		PermHRISView,
	},
}

// HasPermission is the single authorization check in the system.
func HasPermission(role AdminRole, permission Permission) bool {
	return contains(RolePermissions[role], permission)
}

// PermissionsFor returns the caller's capability list, sent at login.
func PermissionsFor(role AdminRole) []Permission {
	perms, ok := RolePermissions[role]
	if !ok {
		return []Permission{}
	}
	out := make([]Permission, len(perms))
	copy(out, perms)
	return out
}

// IsAdminRole validates a role string arriving from a request.
func IsAdminRole(value string) bool {
	return contains(AdminRoles, AdminRole(value))
}

func without(all []Permission, excluded ...Permission) []Permission {
	out := make([]Permission, 0, len(all))
	for _, p := range all {
		if !contains(excluded, p) {
			out = append(out, p)
		}
	}
	return out
}
