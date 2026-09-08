package app

import "github.com/syabanf/hyrox-app/apps/backend/internal/domain"

// domainHasPermission adapts the domain's RBAC matrix to the string-based
// signature the auth guard uses, keeping the platform layer free of any
// dependency on the business domain.
func domainHasPermission(role, permission string) bool {
	return domain.HasPermission(domain.AdminRole(role), domain.Permission(permission))
}
