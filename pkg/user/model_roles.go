package user

type UserRoleType string

const (
	UserRoleAdmin     UserRoleType = "admin"
	UserRoleDeveloper UserRoleType = "developer"
	UserRoleModerator UserRoleType = "moderator"
	UserRoleWorker    UserRoleType = "worker" // reserved for system/cronjob use
	UserRolePlayer    UserRoleType = "player"
	UserRoleGuest     UserRoleType = "guest"
)

type UserRoles []UserRoleType

func (r UserRoles) Strings() []string {
	if r == nil {
		return []string{}
	}

	strs := make([]string, len(r))
	for i, role := range r {
		strs[i] = string(role)
	}
	return strs
}

// HasRole checks if the UserRoles slice contains a specific role
func (r UserRoles) HasRole(target UserRoleType) bool {
	for _, role := range r {
		if role == target {
			return true
		}
	}
	return false
}

// Convenience methods for specific roles
func (r UserRoles) IsAdmin() bool {
	return r.HasRole(UserRoleAdmin)
}

func (r UserRoles) IsWorker() bool {
	return r.HasRole(UserRoleWorker)
}

func (r UserRoles) IsPlayer() bool {
	return r.HasRole(UserRolePlayer)
}
