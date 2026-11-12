package domain

import (
	"errors"
	"testing"
)

// Un `status` mal escrito no puede caer en la lista de activos: la pantalla diría «Dadas de baja»
// sobre una lista de gente activa, y quien la lee reactivaría a alguien que nunca se fue.
func TestParseUserListStatus(t *testing.T) {
	cases := []struct {
		in      string
		want    UserListStatus
		wantErr bool
	}{
		{in: "", want: UsersActive},
		{in: "active", want: UsersActive},
		{in: "inactive", want: UsersInactive},
		{in: "INACTIVE", wantErr: true},
		{in: "inactivo", wantErr: true},
		{in: "all", wantErr: true},
		{in: " inactive", wantErr: true},
	}
	for _, c := range cases {
		got, err := ParseUserListStatus(c.in)
		if c.wantErr {
			if !errors.Is(err, ErrValidation) {
				t.Errorf("%q: err = %v, quiere ErrValidation (no caer a activos en silencio)", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q = %q, %v; quiere %q", c.in, got, err, c.want)
		}
	}
}
