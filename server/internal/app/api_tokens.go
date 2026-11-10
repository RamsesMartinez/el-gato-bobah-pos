package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// APITokensService administra los tokens del servidor MCP (spec 072) y lo que cada llamada deja:
// la clave de idempotencia de una escritura y su evento en la bitácora.
//
// Dos maneras de llegar a la base, a propósito. Crear, listar y revocar vienen de la pantalla y
// usan la conexión de la empresa del request (WithTenant). Autenticar, la idempotencia y la
// bitácora de una llamada MCP abren cada una su propia transacción corta con la empresa del token:
// ninguna conexión se retiene mientras la herramienta se despacha, porque el despacho toma la suya
// y con el pool lleno las dos esperarían para siempre.
type APITokensService struct {
	store *store.Store
	now   func() time.Time
}

func NewAPITokensService(st *store.Store) *APITokensService {
	return &APITokensService{store: st, now: time.Now}
}

// APITokenView es un token como lo ve la pantalla. Nunca trae el secreto ni la huella.
type APITokenView struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	OwnerID    int64      `json:"ownerId"`
	OwnerName  string     `json:"ownerName"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
	// Status: active | revoked | expired.
	Status string `json:"status"`
}

// CreatedAPIToken es la única respuesta que lleva el token completo.
type CreatedAPIToken struct {
	APITokenView
	Token string `json:"token"`
}

func tokenStatus(now time.Time, expires time.Time, revoked *time.Time) string {
	switch {
	case revoked != nil:
		return "revoked"
	case !expires.After(now):
		return "expired"
	}
	return "active"
}

// Create crea un token a nombre de quien llama, después de verificar su contraseña: un PIN visto en
// la tableta no basta para llevarse una llave que opera el negocio desde fuera.
func (s *APITokensService) Create(ctx context.Context, actorID int64, password string, in domain.APITokenInput) (CreatedAPIToken, error) {
	norm, err := in.Normalize()
	if err != nil {
		return CreatedAPIToken{}, err
	}
	company, ok := store.CompanyFrom(ctx)
	if !ok {
		return CreatedAPIToken{}, domain.ErrUnauthorized
	}
	u, err := s.store.QC(ctx).GetUserByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CreatedAPIToken{}, domain.ErrUnauthorized
		}
		return CreatedAPIToken{}, err
	}
	if u.PasswordHash == nil || !auth.CheckSecret(*u.PasswordHash, password) {
		return CreatedAPIToken{}, domain.ErrInvalidCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return CreatedAPIToken{}, fmt.Errorf("generar el secreto del token: %w", err)
	}
	secret := hex.EncodeToString(raw)
	scopes := make([]string, len(norm.Scopes))
	for i, sc := range norm.Scopes {
		scopes[i] = string(sc)
	}
	expires := s.now().Add(time.Duration(norm.ExpiresInDays) * 24 * time.Hour)
	var out CreatedAPIToken
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.InsertAPIToken(ctx, db.InsertAPITokenParams{
			UserID: actorID, Name: norm.Name, SecretHint: domain.APITokenHint(secret),
			SecretHash: domain.APITokenSecretHash(secret), Scopes: scopes, ExpiresAt: expires,
		})
		if err != nil {
			return err
		}
		out = CreatedAPIToken{
			APITokenView: APITokenView{
				ID: row.ID, Name: norm.Name, Prefix: apiTokenPrefix(company, row.ID, secret),
				OwnerID: actorID, OwnerName: u.Name, Scopes: scopes, CreatedAt: row.CreatedAt, ExpiresAt: expires,
				Status: "active",
			},
			Token: domain.FormatAPIToken(company, row.ID, secret),
		}
		return auditEntity(ctx, q, domain.AuditAPITokenCreated, "api_token", row.ID, nil, &actorID, nil,
			map[string]any{"name": norm.Name, "scopes": scopes, "expiresInDays": norm.ExpiresInDays, "owner": actorID})
	})
	return out, err
}

// apiTokenPrefix es lo que se puede mostrar de un token: todo menos el secreto, más su pista.
func apiTokenPrefix(company, id int64, hintOrSecret string) string {
	return fmt.Sprintf("egbpat_%d_%d_%s", company, id, domain.APITokenHint(hintOrSecret))
}

// List devuelve los tokens de la empresa, activos primero.
func (s *APITokensService) List(ctx context.Context) ([]APITokenView, error) {
	company, ok := store.CompanyFrom(ctx)
	if !ok {
		return nil, domain.ErrUnauthorized
	}
	rows, err := s.store.QC(ctx).ListAPITokens(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]APITokenView, 0, len(rows))
	for _, r := range rows {
		revoked := tsPtr(r.RevokedAt)
		scopes := r.Scopes
		if scopes == nil {
			scopes = []string{}
		}
		out = append(out, APITokenView{
			ID: r.ID, Name: r.Name, Prefix: apiTokenPrefix(company, r.ID, r.SecretHint), OwnerID: r.UserID,
			OwnerName: r.OwnerName, Scopes: scopes, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt,
			LastUsedAt: tsPtr(r.LastUsedAt), RevokedAt: revoked, Status: tokenStatus(now, r.ExpiresAt, revoked),
		})
	}
	return out, nil
}

// Revoke revoca un token de la empresa. Revocar uno ya revocado no hace nada; uno de otra empresa no
// existe (RLS).
func (s *APITokensService) Revoke(ctx context.Context, actorID, tokenID int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		r, err := q.RevokeAPIToken(ctx, db.RevokeAPITokenParams{RevokedBy: &actorID, TokenID: tokenID})
		if err != nil {
			return err
		}
		if r.Found == 0 {
			return domain.ErrNotFound
		}
		if r.Revoked == 0 {
			return nil
		}
		return auditEntity(ctx, q, domain.AuditAPITokenRevoked, "api_token", tokenID, nil, &actorID, nil,
			map[string]any{"revokedBy": actorID})
	})
}

// APIPrincipal es quién llama por el servidor MCP. La empresa es la de la fila encontrada bajo RLS,
// no la que dice el texto del token (que solo sirvió para buscarla).
type APIPrincipal struct {
	TokenID   int64
	TokenName string
	CompanyID int64
	UserID    int64
	UserName  string
	RoleCode  string
	Scopes    []domain.APIScope
}

// dummyAPITokenHash es contra lo que se compara un token que no existe, para que tarde lo mismo que
// uno que sí: la diferencia de tiempo diría qué ids existen.
var dummyAPITokenHash = domain.APITokenSecretHash("no-existe")

// Authenticate valida un token. Todo rechazo es ErrUnauthorized, sin decir cuál fue; la razón viene
// aparte para el evento de seguridad, nunca para la respuesta.
func (s *APITokensService) Authenticate(ctx context.Context, raw string) (APIPrincipal, error) {
	p, _, err := s.AuthenticateWithReason(ctx, raw)
	return p, err
}

// AuthenticateWithReason es Authenticate más la razón del rechazo (para el log, nunca la respuesta).
func (s *APITokensService) AuthenticateWithReason(ctx context.Context, raw string) (APIPrincipal, string, error) {
	parsed, err := domain.ParseAPIToken(raw)
	if err != nil {
		return APIPrincipal{}, "malformed", domain.ErrUnauthorized
	}
	var row db.GetAPITokenForAuthRow
	found := true
	err = s.store.WithTenant(ctx, parsed.CompanyID, func(q *db.Queries) error {
		r, err := q.GetAPITokenForAuth(ctx, parsed.TokenID)
		if errors.Is(err, pgx.ErrNoRows) {
			found = false
			return nil
		}
		row = r
		return err
	})
	if err != nil {
		return APIPrincipal{}, "", err
	}
	hash := row.SecretHash
	if !found {
		hash = dummyAPITokenHash
	}
	matches := domain.APITokenSecretMatches(parsed.Secret, hash)
	reason := ""
	switch {
	case !found:
		reason = "not_found"
	case !matches:
		reason = "bad_secret"
	case row.CompanyID != parsed.CompanyID:
		// Bajo RLS no llega aquí; si un día se sirve como owner (o RLS falla), la empresa del texto
		// no puede ser otra que la de la fila.
		reason = "company_mismatch"
	case row.RevokedAt.Valid:
		reason = "revoked"
	case !row.ExpiresAt.After(s.now()):
		reason = "expired"
	case !row.UserActive:
		reason = "user_inactive"
	case !row.RoleActive:
		reason = "role_inactive"
	}
	if reason != "" {
		return APIPrincipal{}, reason, domain.ErrUnauthorized
	}
	// El último uso no detiene la llamada: si falla, el token igual sirve.
	_ = s.store.WithTenant(ctx, parsed.CompanyID, func(q *db.Queries) error { return q.TouchAPIToken(ctx, row.ID) })
	scopes := make([]domain.APIScope, len(row.Scopes))
	for i, sc := range row.Scopes {
		scopes[i] = domain.APIScope(sc)
	}
	return APIPrincipal{
		TokenID: row.ID, TokenName: row.Name, CompanyID: row.CompanyID, UserID: row.UserID,
		UserName: row.UserName, RoleCode: row.RoleCode, Scopes: scopes,
	}, "", nil
}

// ---- Idempotencia ----

// BeginIdempotent toma la clave o dice qué hacer con ella. Con IdemReplay devuelve la respuesta
// guardada. Purga antes las claves de más de 7 días del mismo token.
func (s *APITokensService) BeginIdempotent(ctx context.Context, k domain.IdempotencyKeyRef, argsHash []byte) (domain.IdempotencyDecision, json.RawMessage, error) {
	decision := domain.IdemProceed
	var saved json.RawMessage
	err := s.store.WithTenant(ctx, k.CompanyID, func(q *db.Queries) error {
		if err := q.PurgeOldIdempotencyKeys(ctx, k.TokenID); err != nil {
			return err
		}
		_, err := q.InsertIdempotencyKey(ctx, db.InsertIdempotencyKeyParams{
			TokenID: k.TokenID, IdemKey: k.Key, Tool: k.Tool, ArgsHash: argsHash,
		})
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		existing, err := q.GetIdempotencyKey(ctx, db.GetIdempotencyKeyParams{TokenID: k.TokenID, IdemKey: k.Key})
		if err != nil {
			return err
		}
		decision = domain.DecideIdempotency(&domain.IdempotencyRecord{Status: existing.Status, ArgsHash: existing.ArgsHash}, argsHash)
		saved = existing.Result
		return nil
	})
	return decision, saved, err
}

// FinishIdempotent guarda la respuesta de una escritura que terminó bien.
func (s *APITokensService) FinishIdempotent(ctx context.Context, k domain.IdempotencyKeyRef, result json.RawMessage) error {
	return s.store.WithTenant(ctx, k.CompanyID, func(q *db.Queries) error {
		return q.FinishIdempotencyKey(ctx, db.FinishIdempotencyKeyParams{TokenID: k.TokenID, IdemKey: k.Key, Result: result})
	})
}

// AbandonIdempotent suelta la clave de una escritura que la operación rechazó: nada se aplicó y se
// puede reintentar con la misma clave.
func (s *APITokensService) AbandonIdempotent(ctx context.Context, k domain.IdempotencyKeyRef) error {
	return s.store.WithTenant(ctx, k.CompanyID, func(q *db.Queries) error {
		return q.DeleteIdempotencyKey(ctx, db.DeleteIdempotencyKeyParams{TokenID: k.TokenID, IdemKey: k.Key})
	})
}

// MarkIdempotentUnknown deja la clave en «desconocida»: la escritura terminó en error interno o se
// cortó y pudo haberse aplicado.
func (s *APITokensService) MarkIdempotentUnknown(ctx context.Context, k domain.IdempotencyKeyRef) error {
	return s.store.WithTenant(ctx, k.CompanyID, func(q *db.Queries) error {
		return q.MarkIdempotencyKeyUnknown(ctx, db.MarkIdempotencyKeyUnknownParams{TokenID: k.TokenID, IdemKey: k.Key})
	})
}

// ---- Bitácora ----

// MCPCall es una llamada de herramienta para la bitácora.
type MCPCall struct {
	Tool string
	// Mode: read | preview | apply.
	Mode         string
	Status       int
	Args         json.RawMessage
	EntityBefore json.RawMessage
	EntityAfter  json.RawMessage
}

// maxAuditArgs acota lo que un agente puede meter en un evento: los argumentos de una herramienta
// son chicos, y uno que no lo es se registra truncado en vez de engordar la bitácora.
const maxAuditArgs = 8 << 10

// RecordCall escribe el evento mcp.tool_called de una llamada, en su propia transacción: si la
// operación falló y se revirtió, el intento igual queda.
func (s *APITokensService) RecordCall(ctx context.Context, p APIPrincipal, c MCPCall) error {
	after := map[string]any{"tool": c.Tool, "mode": c.Mode, "status": c.Status,
		"token": p.TokenName, "tokenId": p.TokenID}
	if len(c.Args) > 0 {
		if len(c.Args) > maxAuditArgs {
			after["args"] = "(argumentos de más de 8 KB, omitidos)"
		} else {
			after["args"] = c.Args
		}
	}
	if len(c.EntityBefore) > 0 {
		after["entityBefore"] = c.EntityBefore
	}
	if len(c.EntityAfter) > 0 {
		after["entityAfter"] = c.EntityAfter
	}
	uid := p.UserID
	return s.store.WithTenant(ctx, p.CompanyID, func(q *db.Queries) error {
		return auditEntity(ctx, q, domain.AuditMCPToolCalled, "api_token", p.TokenID, nil, &uid, nil, after)
	})
}
