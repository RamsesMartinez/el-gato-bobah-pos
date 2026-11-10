//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

const tokenPassword = "Contraseña-de-prueba-larga-1"

// tokenWorld es una empresa con un admin que tiene contraseña, y otra empresa ajena.
type tokenWorld struct {
	owner     *store.Store
	app       *store.Store
	company   int64
	other     int64
	admin     int64
	otherUser int64
}

func newTokenWorld(t *testing.T) tokenWorld {
	t.Helper()
	st := newTestStore(t)
	w := tokenWorld{owner: st, company: makeCompany(t, st, "tokens-a"), other: makeCompany(t, st, "tokens-b")}
	w.admin = makeUserIn(t, st, w.company, "admin-tokens", "admin")
	w.otherUser = makeUserIn(t, st, w.other, "admin-ajeno", "admin")
	h, err := auth.HashSecret(tokenPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(context.Background(), `update users set password_hash = $1 where id in ($2, $3)`,
		h, w.admin, w.otherUser); err != nil {
		t.Fatal(err)
	}
	w.app = appRoleStore(t)
	return w
}

// tenant devuelve un ctx con la conexión de la empresa y la persona del request, como WithTenant.
func (w tokenWorld) tenant(t *testing.T, company, user int64) context.Context {
	t.Helper()
	ctx, release, err := w.app.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return app.WithActorUser(ctx, user)
}

func (w tokenWorld) create(t *testing.T, scopes ...string) app.CreatedAPIToken {
	t.Helper()
	svc := app.NewAPITokensService(w.app)
	tok, err := svc.Create(w.tenant(t, w.company, w.admin), w.admin, tokenPassword,
		domain.APITokenInput{Name: "Claude de prueba", Scopes: scopes, ExpiresInDays: 90})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAPITokenCreateListRevoke(t *testing.T) {
	t.Parallel()
	w := newTokenWorld(t)
	svc := app.NewAPITokensService(w.app)
	ctx := w.tenant(t, w.company, w.admin)

	t.Run("contraseña equivocada no crea nada", func(t *testing.T) {
		_, err := svc.Create(ctx, w.admin, "otra", domain.APITokenInput{Name: "x", ExpiresInDays: 30})
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("esperaba credenciales inválidas, salió %v", err)
		}
	})

	tok := w.create(t, "write:expenses")
	t.Run("el token se ve una vez y la base solo guarda la huella", func(t *testing.T) {
		p, err := domain.ParseAPIToken(tok.Token)
		if err != nil || p.CompanyID != w.company || p.TokenID != tok.ID {
			t.Fatalf("token %q mal formado: %v", tok.Token, err)
		}
		var hint string
		var hash []byte
		if err := w.owner.Pool.QueryRow(context.Background(),
			`select secret_hint, secret_hash from api_tokens where id = $1`, tok.ID).Scan(&hint, &hash); err != nil {
			t.Fatal(err)
		}
		if hint != p.Secret[:4] || !domain.APITokenSecretMatches(p.Secret, hash) {
			t.Fatal("la pista o la huella no corresponden al secreto")
		}
		if strings.Contains(string(hash), p.Secret) {
			t.Fatal("el secreto quedó en claro")
		}
		list, err := svc.List(ctx)
		if err != nil || len(list) != 1 {
			t.Fatalf("lista: %v %+v", err, list)
		}
		if list[0].OwnerID != w.admin || list[0].Status != "active" || strings.Join(list[0].Scopes, ",") != "read,write:expenses" {
			t.Fatalf("renglón: %+v", list[0])
		}
	})

	t.Run("crear y revocar quedan en la bitácora sin el secreto", func(t *testing.T) {
		if err := svc.Revoke(ctx, w.admin, tok.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.Revoke(ctx, w.admin, tok.ID); err != nil {
			t.Fatalf("revocar dos veces debe ser idempotente: %v", err)
		}
		rows, err := w.owner.Pool.Query(context.Background(),
			`select action, coalesce(after::text, '') from audit_events where entity_type = 'api_token' and entity_id = $1 order by id`, tok.ID)
		if err != nil {
			t.Fatal(err)
		}
		var actions []string
		for rows.Next() {
			var a, after string
			_ = rows.Scan(&a, &after)
			actions = append(actions, a)
			p, _ := domain.ParseAPIToken(tok.Token)
			if strings.Contains(after, p.Secret) || strings.Contains(after, p.Secret[:4]) {
				t.Errorf("%s guardó parte del secreto: %s", a, after)
			}
		}
		rows.Close()
		if len(actions) != 2 || actions[0] != "api_token.created" || actions[1] != "api_token.revoked" {
			t.Fatalf("bitácora: %v", actions)
		}
	})

	t.Run("revocar un token de otra empresa es no encontrado", func(t *testing.T) {
		ajeno := app.NewAPITokensService(w.app)
		octx := w.tenant(t, w.other, w.otherUser)
		if err := ajeno.Revoke(octx, w.otherUser, tok.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("esperaba no encontrado, salió %v", err)
		}
	})
}

func TestAPITokenRejectionsLookAlike(t *testing.T) {
	t.Parallel()
	w := newTokenWorld(t)
	svc := app.NewAPITokensService(w.app)
	ctx := context.Background()
	good := w.create(t)
	if p, err := svc.Authenticate(ctx, good.Token); err != nil || p.UserID != w.admin || p.CompanyID != w.company {
		t.Fatalf("un token bueno no entró: %v %+v", err, p)
	}

	revoked := w.create(t)
	if err := svc.Revoke(w.tenant(t, w.company, w.admin), w.admin, revoked.ID); err != nil {
		t.Fatal(err)
	}
	expired := w.create(t)
	if _, err := w.owner.Pool.Exec(ctx,
		`update api_tokens set created_at = now() - interval '100 days', expires_at = now() - interval '1 day' where id = $1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	p, _ := domain.ParseAPIToken(good.Token)
	wrongSecret := domain.FormatAPIToken(w.company, good.ID, strings.Repeat("0", 64))
	missing := domain.FormatAPIToken(w.company, 999999, p.Secret)

	cases := map[string]string{
		"revocado": revoked.Token, "vencido": expired.Token, "secreto ajeno": wrongSecret,
		"no existe": missing, "basura": "egbpat_x",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Authenticate(ctx, raw)
			if !errors.Is(err, domain.ErrUnauthorized) {
				t.Fatalf("esperaba no autenticado, salió %v", err)
			}
			if err.Error() != domain.ErrUnauthorized.Error() {
				t.Fatalf("el rechazo dice más que «no autenticado»: %q", err.Error())
			}
		})
	}

	t.Run("persona dada de baja", func(t *testing.T) {
		if _, err := w.owner.Pool.Exec(ctx, `update users set is_active = false where id = $1`, w.admin); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = w.owner.Pool.Exec(ctx, `update users set is_active = true where id = $1`, w.admin) })
		if _, err := svc.Authenticate(ctx, good.Token); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("una persona dada de baja entró: %v", err)
		}
	})
}

func TestAPITokenFromOtherCompanyIsUnknown(t *testing.T) {
	t.Parallel()
	w := newTokenWorld(t)
	svc := app.NewAPITokensService(w.app)
	tok := w.create(t, "write:expenses")
	p, _ := domain.ParseAPIToken(tok.Token)
	// El mismo id y el mismo secreto, declarando la otra empresa: bajo RLS de esa empresa la fila
	// no existe.
	forged := domain.FormatAPIToken(w.other, p.TokenID, p.Secret)
	if _, err := svc.Authenticate(context.Background(), forged); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("un token con la empresa cambiada entró: %v", err)
	}
}

func TestAPITokenLastUsedAtMostOncePerMinute(t *testing.T) {
	t.Parallel()
	w := newTokenWorld(t)
	svc := app.NewAPITokensService(w.app)
	tok := w.create(t)
	ctx := context.Background()
	for range 3 {
		if _, err := svc.Authenticate(ctx, tok.Token); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := w.owner.Pool.QueryRow(ctx,
		`select count(*) from api_tokens where id = $1 and last_used_at is not null`, tok.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("último uso no quedó: %v %d", err, n)
	}
}

func TestIdempotencyKeysEndToEnd(t *testing.T) {
	t.Parallel()
	w := newTokenWorld(t)
	svc := app.NewAPITokensService(w.app)
	tok := w.create(t, "write:expenses")
	ctx := context.Background()
	hashA := domain.IdempotencyArgsHash("expense_create", []byte(`{"amount":"10.00"}`))
	hashB := domain.IdempotencyArgsHash("expense_create", []byte(`{"amount":"11.00"}`))
	key := domain.IdempotencyKeyRef{CompanyID: w.company, TokenID: tok.ID, Key: "gasto-0001", Tool: "expense_create"}

	d, _, err := svc.BeginIdempotent(ctx, key, hashA)
	if err != nil || d != domain.IdemProceed {
		t.Fatalf("primera: %v %v", d, err)
	}
	if d, _, _ := svc.BeginIdempotent(ctx, key, hashA); d != domain.IdemInProgress {
		t.Fatalf("mientras corre: %v", d)
	}
	if err := svc.FinishIdempotent(ctx, key, json.RawMessage(`{"id":7}`)); err != nil {
		t.Fatal(err)
	}
	d, saved, err := svc.BeginIdempotent(ctx, key, hashA)
	if err != nil || d != domain.IdemReplay || string(saved) != `{"id": 7}` && string(saved) != `{"id":7}` {
		t.Fatalf("repetición: %v %s %v", d, saved, err)
	}
	if d, _, _ := svc.BeginIdempotent(ctx, key, hashB); d != domain.IdemConflict {
		t.Fatalf("otros argumentos: %v", d)
	}

	key2 := key
	key2.Key = "gasto-0002"
	_, _, _ = svc.BeginIdempotent(ctx, key2, hashA)
	if err := svc.AbandonIdempotent(ctx, key2); err != nil {
		t.Fatal(err)
	}
	if d, _, _ := svc.BeginIdempotent(ctx, key2, hashA); d != domain.IdemProceed {
		t.Fatalf("tras un 4xx la clave se libera: %v", d)
	}
	if err := svc.MarkIdempotentUnknown(ctx, key2); err != nil {
		t.Fatal(err)
	}
	if d, _, _ := svc.BeginIdempotent(ctx, key2, hashA); d != domain.IdemUnknown {
		t.Fatalf("tras un 5xx no se repite sola: %v", d)
	}

	t.Run("la clave de un token no la ve otra empresa", func(t *testing.T) {
		other := key
		other.CompanyID = w.other
		if d, _, err := svc.BeginIdempotent(ctx, other, hashA); err == nil && d != domain.IdemProceed {
			t.Fatalf("otra empresa vio la clave: %v", d)
		}
	})
}

func TestAPITokenRecordCallWritesAuditEvent(t *testing.T) {
	t.Parallel()
	w := newTokenWorld(t)
	svc := app.NewAPITokensService(w.app)
	tok := w.create(t, "write:expenses")
	p, err := svc.Authenticate(context.Background(), tok.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordCall(context.Background(), p, app.MCPCall{Tool: "expenses_list", Mode: "read", Status: 200,
		Args: json.RawMessage(`{"page":1}`)}); err != nil {
		t.Fatal(err)
	}
	var actor int64
	var after string
	if err := w.owner.Pool.QueryRow(context.Background(), `
		select actor_user_id, after::text from audit_events
		 where action = 'mcp.tool_called' and entity_type = 'api_token' and entity_id = $1`, tok.ID).Scan(&actor, &after); err != nil {
		t.Fatal(err)
	}
	if actor != w.admin || !strings.Contains(after, `"tool": "expenses_list"`) || !strings.Contains(after, `"token": "Claude de prueba"`) {
		t.Fatalf("evento: actor %d, after %s", actor, after)
	}
}

// Aislamiento de las dos tablas nuevas en los tres casos donde RLS ya falló.
func TestAPITokenTablesInTheThreeCases(t *testing.T) {
	t.Parallel()
	w := newTokenWorld(t)
	tok := w.create(t, "write:expenses")
	key := domain.IdempotencyKeyRef{CompanyID: w.company, TokenID: tok.ID, Key: "aislada-01", Tool: "expense_create"}
	if _, _, err := app.NewAPITokensService(w.app).BeginIdempotent(context.Background(), key,
		domain.IdempotencyArgsHash("expense_create", []byte(`{}`))); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, w.company, w.other, func(t *testing.T, st *store.Store, ctx context.Context) {
		list, err := app.NewAPITokensService(st).List(ctx)
		if err == nil && len(list) > 0 {
			t.Fatalf("se vieron %d tokens ajenos", len(list))
		}
		if _, err := st.QC(ctx).GetAPITokenForAuth(ctx, tok.ID); err == nil {
			t.Fatal("se leyó el token de otra empresa")
		}
		if _, err := st.QC(ctx).GetIdempotencyKey(ctx, db.GetIdempotencyKeyParams{TokenID: tok.ID, IdemKey: key.Key}); err == nil {
			t.Fatal("se leyó la clave de idempotencia de otra empresa")
		}
	})
}

// Si la API sirviera como owner (o RLS fallara), la empresa del texto del token no puede ser otra
// que la de su fila: la empresa del principal sale de la fila, y una distinta se rechaza.
func TestAPITokenCompanyComesFromTheRowNotTheText(t *testing.T) {
	t.Parallel()
	w := newTokenWorld(t)
	tok := w.create(t)
	p, _ := domain.ParseAPIToken(tok.Token)
	asOwner := app.NewAPITokensService(w.owner)
	if got, err := asOwner.Authenticate(context.Background(), tok.Token); err != nil || got.CompanyID != w.company {
		t.Fatalf("el token bueno como owner: %v %+v", err, got)
	}
	forged := domain.FormatAPIToken(w.other, p.TokenID, p.Secret)
	if _, reason, err := asOwner.AuthenticateWithReason(context.Background(), forged); !errors.Is(err, domain.ErrUnauthorized) || reason != "company_mismatch" {
		t.Fatalf("empresa cambiada como owner: %v (%s)", err, reason)
	}
}
