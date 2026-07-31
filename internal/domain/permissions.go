package domain

// Permission represents a specific permission in the system
type Permission string

const (
	// Admin permissions
	PermissionAdminAccess   Permission = "admin:access"
	PermissionUserManage    Permission = "users:manage"
	PermissionTenantManage  Permission = "tenant:manage"
	PermissionBillingManage Permission = "billing:manage"
	PermissionSystemConfig  Permission = "system:config"

	// Manager permissions
	PermissionTransformerManage Permission = "transformer:manage"
	PermissionUCManage          Permission = "uc:manage"
	PermissionReadingManage     Permission = "reading:manage"
	PermissionImportManage      Permission = "import:manage"
	PermissionReportView        Permission = "report:view"
	PermissionReportExport      Permission = "report:export"

	// Operator permissions
	PermissionTransformerView Permission = "transformer:view"
	PermissionUCView          Permission = "uc:view"
	PermissionReadingView     Permission = "reading:view"
	PermissionDashboardView   Permission = "dashboard:view"
)

// RolePermissions maps each role to its permissions
var RolePermissions = map[UserRole][]Permission{
	UserRoleSuperAdmin: {
		PermissionAdminAccess,
		PermissionUserManage,
		PermissionTenantManage,
		PermissionBillingManage,
		PermissionSystemConfig,
		PermissionTransformerManage,
		PermissionUCManage,
		PermissionReadingManage,
		PermissionImportManage,
		PermissionReportView,
		PermissionReportExport,
		PermissionTransformerView,
		PermissionUCView,
		PermissionReadingView,
		PermissionDashboardView,
	},
	UserRoleOwner: {
		PermissionTransformerManage,
		PermissionUCManage,
		PermissionReadingManage,
		PermissionImportManage,
		PermissionReportView,
		PermissionReportExport,
		PermissionTransformerView,
		PermissionUCView,
		PermissionReadingView,
		PermissionDashboardView,
	},
	UserRoleAdmin: {
		PermissionTransformerManage,
		PermissionUCManage,
		PermissionReadingManage,
		PermissionImportManage,
		PermissionReportView,
		PermissionReportExport,
		PermissionTransformerView,
		PermissionUCView,
		PermissionReadingView,
		PermissionDashboardView,
	},
	UserRoleManager: {
		PermissionTransformerView,
		PermissionUCView,
		PermissionReadingView,
		PermissionDashboardView,
	},
	UserRoleEngineer: {
		PermissionTransformerView,
		PermissionUCView,
		PermissionReadingView,
		PermissionDashboardView,
	},
	UserRoleViewer: {
		PermissionDashboardView,
	},
}

// HasPermission checks if a role has a specific permission
func HasPermission(role UserRole, permission Permission) bool {
	permissions := RolePermissions[role]
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}
