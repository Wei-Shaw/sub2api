package service

func IsValidUserRole(role string) bool {
	switch role {
	case RoleAdmin, RoleUser, RoleUsageViewer:
		return true
	default:
		return false
	}
}
