//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// El aislamiento de tenant a través del store (RLS) SOLO es real conectando como el rol de app
// (no-superusuario); el owner lo saltaría. Crea dos empresas con un usuario cada una y verifica
// que, bajo el rol de app, cada tenant ve exclusivamente lo suyo.
func TestTenantIsolationViaAppRole(t *testing.T) {
	owner := newTestStore(t) // owner: siembra fixtures (salta RLS)
	acme := makeCompany(t, owner, "acme")
	makeUserIn(t, owner, defaultCompanyID, "solo_gato", "cajero")
	makeUserIn(t, owner, acme, "solo_acme", "cajero")

	appSt := appRoleStore(t) // gatobobah_app: RLS aplica

	usernames := func(companyID int64) []string {
		var out []string
		if err := appSt.WithTenant(context.Background(), companyID, func(q *db.Queries) error {
			rows, err := q.ListActiveUsers(context.Background())
			if err != nil {
				return err
			}
			for _, u := range rows {
				if u.Username != nil {
					out = append(out, *u.Username)
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("WithTenant(%d): %v", companyID, err)
		}
		return out
	}

	gato := usernames(defaultCompanyID)
	if len(gato) != 1 || gato[0] != "solo_gato" {
		t.Fatalf("tenant gatobobah debe ver solo [solo_gato], vio %v", gato)
	}
	acmeUsers := usernames(acme)
	if len(acmeUsers) != 1 || acmeUsers[0] != "solo_acme" {
		t.Fatalf("tenant acme debe ver solo [solo_acme], vio %v", acmeUsers)
	}

	// Escritura cruzada: bajo el tenant gatobobah, insertar un usuario marcado company=acme debe
	// ser rechazado por la RLS (WITH CHECK).
	err := appSt.WithTenant(context.Background(), defaultCompanyID, func(q *db.Queries) error {
		_, e := q.CreateUser(context.Background(), db.CreateUserParams{
			Name: "intruso", Username: strptr("intruso"), Role: string(domain.RoleCajero),
		})
		return e
	})
	// El default de company_id = GUC (=1) hace que el insert caiga en gatobobah (permitido); lo
	// que NO se puede es forzar otra empresa. Verificamos que el intruso quedó en gatobobah, no en acme.
	if err != nil {
		t.Fatalf("insert bajo tenant propio no debería fallar: %v", err)
	}
	if got := usernames(acme); len(got) != 1 {
		t.Fatalf("acme no debe haber ganado usuarios por un insert de otro tenant, vio %v", got)
	}
}

// Login está scopeado por empresa: dos empresas pueden tener el mismo username y cada login
// resuelve al usuario de SU slug, con su propia contraseña.
func TestLoginIsScopedByCompany(t *testing.T) {
	owner := newTestStore(t)
	acme := makeCompany(t, owner, "acme")
	appSt := appRoleStore(t)
	users := app.NewUsersService(appSt, nil, false) // HIBP off en test
	jm := auth.NewManager("0123456789abcdef0123456789abcdef", nil)
	authSvc := app.NewAuthService(appSt, jm, clock)

	const pwGato = "Contrasena-Gato-2026"
	const pwAcme = "Contrasena-Acme-2026"

	// Mismo username 'jefe' en ambas empresas, con contraseñas distintas. Create corre bajo el
	// tenant de cada empresa (AcquireTenant → QC usa esa conexión).
	createIn := func(companyID int64, pw string) {
		ctx, release, err := appSt.AcquireTenant(context.Background(), companyID)
		if err != nil {
			t.Fatalf("AcquireTenant(%d): %v", companyID, err)
		}
		defer release()
		if _, err := users.Create(ctx, app.CreateUserInput{
			Name: "Jefe", Username: strptr("jefe"), Role: domain.RoleAdmin, Password: pw,
		}); err != nil {
			t.Fatalf("Create jefe en %d: %v", companyID, err)
		}
	}
	createIn(defaultCompanyID, pwGato)
	createIn(acme, pwAcme)

	ctx := context.Background()
	// jefe@gatobobah con su contraseña → ok, empresa 1.
	s, err := authSvc.Login(ctx, "jefe", "gatobobah", pwGato)
	if err != nil {
		t.Fatalf("login jefe@gatobobah: %v", err)
	}
	if s.User.CompanyID != defaultCompanyID {
		t.Fatalf("login resolvió empresa %d, esperaba %d", s.User.CompanyID, defaultCompanyID)
	}
	// jefe@acme con la contraseña de gatobobah → credenciales inválidas (aislamiento por empresa).
	if _, err := authSvc.Login(ctx, "jefe", "acme", pwGato); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("login jefe@acme con pw de gatobobah debe fallar, got %v", err)
	}
	// jefe@acme con su propia contraseña → ok, empresa acme.
	s2, err := authSvc.Login(ctx, "jefe", "acme", pwAcme)
	if err != nil {
		t.Fatalf("login jefe@acme: %v", err)
	}
	if s2.User.CompanyID != acme {
		t.Fatalf("login acme resolvió empresa %d, esperaba %d", s2.User.CompanyID, acme)
	}
	// slug inexistente → credenciales inválidas (sin filtrar existencia).
	if _, err := authSvc.Login(ctx, "jefe", "no-existe", pwGato); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("login con slug inexistente debe fallar, got %v", err)
	}
}

func strptr(s string) *string { return &s }
