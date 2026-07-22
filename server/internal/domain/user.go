package domain

import "time"

type Role string

const (
	RoleAdmin   Role = "admin"
	RoleGerente Role = "gerente"
	RoleCajero  Role = "cajero"
	RoleMesero  Role = "mesero"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleGerente, RoleCajero, RoleMesero:
		return true
	}
	return false
}

// In reports whether the role is one of the allowed roles.
func (r Role) In(roles ...Role) bool {
	for _, x := range roles {
		if r == x {
			return true
		}
	}
	return false
}

type User struct {
	ID                 int64     `json:"id"`
	CompanyID          int64     `json:"companyId"`
	CompanySlug        string    `json:"companySlug,omitempty"` // se rellena al emitir sesión (no vive en la fila users)
	Name               string    `json:"name"`
	Username           *string   `json:"username,omitempty"`
	Role               Role      `json:"role"`
	IsActive           bool      `json:"isActive"`
	RecoveryEmail      *string   `json:"recoveryEmail,omitempty"`
	MustChangePassword bool      `json:"mustChangePassword"`
	CreatedAt          time.Time `json:"createdAt"`
}
