//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Reuse-detection de refresh: presentar un token ya revocado revoca TODA la familia. Es el
// cableado (GetRefreshToken → ClassifyRefresh → RevokeUserRefreshTokens) que no se puede
// probar sin BD, así que va aquí.
func TestRefreshReuseRevokesFamily(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)

	hash, err := auth.HashSecret("passw0rd")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	uid := makeUser(t, st, "op_reuse", "cajero")
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, uid, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}

	// Login → sesión A.
	sA, err := svc.Login(ctx, "op_reuse", "passw0rd")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	// Refresh legítimo → sesión B (rota; el token A queda revocado).
	sB, err := svc.Refresh(ctx, sA.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh legítimo: %v", err)
	}
	// Reuso del token A (ya revocado) → unauthorized + revoca la familia.
	if _, err := svc.Refresh(ctx, sA.RefreshToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reuso de token revocado debe dar ErrUnauthorized, got %v", err)
	}
	// La familia quedó revocada: el token B (legítimo) ya no sirve.
	if _, err := svc.Refresh(ctx, sB.RefreshToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("tras el reuso, el token B (familia) debe quedar revocado, got %v", err)
	}
}

// Rotación normal: un refresh válido entrega una sesión nueva y el token viejo deja de
// servir (no es reuso, es rotación) — verifica que el camino feliz no dispara la detección.
func TestRefreshRotationHappyPath(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)

	hash, _ := auth.HashSecret("passw0rd")
	uid := makeUser(t, st, "op_rot", "cajero")
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, uid, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}

	sA, err := svc.Login(ctx, "op_rot", "passw0rd")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	sB, err := svc.Refresh(ctx, sA.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if sB.RefreshToken == sA.RefreshToken {
		t.Fatal("el refresh debe rotar el token")
	}
	// El nuevo token sí rota otra vez.
	if _, err := svc.Refresh(ctx, sB.RefreshToken); err != nil {
		t.Fatalf("segundo refresh del token vigente debe funcionar: %v", err)
	}
}
