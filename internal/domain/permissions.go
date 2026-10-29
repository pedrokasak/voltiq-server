package domain

import "sort"

// Permission represents a specific permission in the system
type Permission string

const (
	// Plataforma (Painel Master)
	PermissionAdminAccess Permission = "admin:access"

	// Administracao do tenant
	PermissionUserManage    Permission = "users:manage"
	PermissionTenantManage  Permission = "tenant:manage"
	PermissionBillingManage Permission = "billing:manage"
	PermissionAuditView     Permission = "audit:view"
	PermissionSystemConfig  Permission = "system:config"

	// Operacao
	PermissionTransformerManage Permission = "transformer:manage"
	PermissionUCManage          Permission = "uc:manage"
	PermissionReadingManage     Permission = "reading:manage"
	PermissionImportManage      Permission = "import:manage"
	PermissionBalanceCalculate  Permission = "balance:calculate"
	PermissionAlertManage       Permission = "alert:manage"
	PermissionReportView        Permission = "report:view"
	PermissionReportExport      Permission = "report:export"
	PermissionFinancialView     Permission = "financial:view"

	// Consulta
	PermissionTransformerView Permission = "transformer:view"
	PermissionUCView          Permission = "uc:view"
	PermissionReadingView     Permission = "reading:view"
	PermissionDashboardView   Permission = "dashboard:view"
)

var viewPermissions = []Permission{
	PermissionDashboardView, PermissionTransformerView, PermissionUCView, PermissionReadingView,
	PermissionReportView, PermissionReportExport,
}

var operationPermissions = []Permission{
	PermissionImportManage, PermissionReadingManage, PermissionBalanceCalculate, PermissionAlertManage,
}

var registryPermissions = []Permission{PermissionTransformerManage, PermissionUCManage}

var tenantAdminPermissions = []Permission{
	PermissionUserManage, PermissionTenantManage, PermissionBillingManage, PermissionAuditView, PermissionFinancialView,
}

func join(groups ...[]Permission) []Permission {
	var out []Permission
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// RolePermissions maps each role to its permissions.
//
// SDD 05-SEGURANCA (ADMIN, ENGENHEIRO, VISUALIZADOR) estendido pelos papeis
// do billing (CLAUDE.md):
//   - SUPER_ADMIN: tudo, inclusive o Painel Master da plataforma
//   - OWNER/ADMIN: tudo no tenant (usuarios, plano, auditoria, cadastros)
//   - MANAGER: operacao completa e dados financeiros (R$)
//   - ENGINEER: importa, calcula e configura alertas; sem R$ e sem editar cadastros
//   - VIEWER: somente leitura de dashboard e relatorios
var RolePermissions = map[UserRole][]Permission{
	UserRoleSuperAdmin: join(viewPermissions, operationPermissions, registryPermissions, tenantAdminPermissions,
		[]Permission{PermissionAdminAccess, PermissionSystemConfig}),
	UserRoleOwner:    join(viewPermissions, operationPermissions, registryPermissions, tenantAdminPermissions),
	UserRoleAdmin:    join(viewPermissions, operationPermissions, registryPermissions, tenantAdminPermissions),
	UserRoleManager:  join(viewPermissions, operationPermissions, registryPermissions, []Permission{PermissionFinancialView}),
	UserRoleEngineer: join(viewPermissions, operationPermissions),
	UserRoleViewer:   join(viewPermissions),
}

// HasPermission checks if a role has a specific permission
func HasPermission(role UserRole, permission Permission) bool {
	for _, p := range RolePermissions[role] {
		if p == permission {
			return true
		}
	}
	return false
}

// PermissionsOf returns the sorted permissions of a role.
func PermissionsOf(role UserRole) []string {
	perms := RolePermissions[role]
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}

// ValidTenantRole reports whether a role can be assigned inside a tenant
// (SUPER_ADMIN is reserved to the platform).
func ValidTenantRole(r UserRole) bool {
	switch r {
	case UserRoleOwner, UserRoleAdmin, UserRoleManager, UserRoleEngineer, UserRoleViewer:
		return true
	}
	return false
}

// roleRank orders roles by privilege.
var roleRank = map[UserRole]int{
	UserRoleViewer: 1, UserRoleEngineer: 2, UserRoleManager: 3, UserRoleAdmin: 4, UserRoleOwner: 5, UserRoleSuperAdmin: 6,
}

// CanAssignRole reports whether actor may grant target to someone: nobody
// grants a role above their own, and only OWNER/SUPER_ADMIN grant OWNER.
func CanAssignRole(actor, target UserRole) bool {
	if !ValidTenantRole(target) {
		return false
	}
	if target == UserRoleOwner {
		return actor == UserRoleOwner || actor == UserRoleSuperAdmin
	}
	return roleRank[actor] >= roleRank[target] && roleRank[actor] >= roleRank[UserRoleAdmin]
}

// RoleRank exposes the privilege order (higher is more privileged).
func RoleRank(r UserRole) int { return roleRank[r] }
