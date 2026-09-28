package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/secrets"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Cipher guarda un secreto de modo que un respaldo de la base no lo revele. En producción es Cloud
// KMS; en desarrollo, AES-GCM local (ver internal/secrets).
type Cipher interface {
	Encrypt(ctx context.Context, plaintext []byte, aad string) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte, aad string) ([]byte, error)
}

// PlatformClient es un cliente ya armado con las credenciales de UNA empresa. Cada servicio lo
// recibe por su propia interfaz —el de menús solo como LectorDeMenu—, así que la garantía de «el
// servicio de menús no puede escribir» sigue siendo de tipos.
type PlatformClient interface {
	LectorDeMenu
	DecisorDePedidos
	// Verify pide un token con esas credenciales. Devuelve los errores de dominio de la captura
	// (ErrCredentialsRejected, ErrCredentialsMissingScopes, ErrPlatformUnavailable).
	Verify(ctx context.Context) error
}

// PlatformClientFactory arma el cliente de una plataforma. Una por plataforma; la de Uber vive en
// el borde (cmd/api), que es donde se traducen los errores de un tercero al vocabulario del negocio.
type PlatformClientFactory func(domain.AppCredentials) (PlatformClient, error)

// PlatformClients entrega el cliente de la empresa del contexto. Es lo que reemplaza al mapa armado
// al arrancar con las credenciales del entorno: aquel era uno para todas las empresas.
type PlatformClients interface {
	MenuReaderFor(ctx context.Context, platform string) (LectorDeMenu, error)
	OrderDeciderFor(ctx context.Context, platform string) (DecisorDePedidos, error)
}

// PlatformCredentialsService captura las credenciales de la aplicación de cada plataforma y arma,
// por empresa, el cliente con el que se habla con ella.
type PlatformCredentialsService struct {
	store     *store.Store
	cipher    Cipher
	factories map[string]PlatformClientFactory // por nombre de plataforma: "Uber Eats"
	// environment es "sandbox" o "production": con cuál app de la plataforma habla este despliegue.
	// La pantalla lo dice porque la causa de rechazo más común es capturar la app del otro.
	environment string

	// clients guarda el cliente armado de cada empresa. NO es una optimización: el cliente guarda su
	// token, y Uber da 100 por hora e invalida el más viejo a partir del 101. Armar uno por aviso
	// invalidaría el que otro aviso está usando.
	mu      sync.Mutex
	clients map[clientKey]builtClient
}

type clientKey struct {
	company  int64
	platform int16
}

// builtClient recuerda con QUÉ credenciales se armó: si la fila cambió (se recapturó), la huella no
// coincide y se arma otro. Así el cliente viejo, con su token, deja de atender aunque el proceso no
// se haya enterado del cambio por la vía de Save.
type builtClient struct {
	fingerprint [sha256.Size]byte
	client      PlatformClient
}

func NewPlatformCredentialsService(s *store.Store, c Cipher, factories map[string]PlatformClientFactory, environment string) *PlatformCredentialsService {
	return &PlatformCredentialsService{
		store: s, cipher: c, factories: factories, environment: environment,
		clients: map[clientKey]builtClient{},
	}
}

func fingerprintOf(clientID string, ciphertext []byte) [sha256.Size]byte {
	h := sha256.New()
	h.Write([]byte(clientID))
	h.Write([]byte{0})
	h.Write(ciphertext)
	var k [sha256.Size]byte
	copy(k[:], h.Sum(nil))
	return k
}

// CredentialsState es lo que la pantalla puede saber: si hay, cuál app (el client id no es secreto)
// y si este ambiente puede leerlas. Nunca el secreto: el tipo no tiene dónde llevarlo.
type CredentialsState struct {
	// Available: este despliegue sabe hablar con esa plataforma. Si no, no tiene caso pedir
	// credenciales que nadie va a usar.
	Available bool `json:"available"`
	// Environment: de cuál app copiar ("sandbox" = la de pruebas, "production" = la real).
	Environment    string     `json:"environment,omitempty"`
	Configured     bool       `json:"configured"`
	ClientID       string     `json:"clientId,omitempty"`
	NeedsRecapture bool       `json:"needsRecapture"`
	UpdatedAt      *time.Time `json:"updatedAt,omitempty"`
	UpdatedBy      string     `json:"updatedBy,omitempty"`
}

// State dice qué hay capturado para la plataforma de ESTA empresa.
func (s *PlatformCredentialsService) State(ctx context.Context, platformID int16) (CredentialsState, error) {
	plat, err := s.store.QC(ctx).GetPlatformByID(ctx, platformID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CredentialsState{}, fmt.Errorf("%w: esa plataforma no existe para esta empresa", domain.ErrValidation)
	}
	if err != nil {
		return CredentialsState{}, fmt.Errorf("plataforma %d: %w", platformID, err)
	}
	st := CredentialsState{Available: s.factories[plat.Name] != nil}
	if st.Available {
		st.Environment = s.environment
	}

	row, err := s.store.QC(ctx).GetPlatformCredentialState(ctx, platformID)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return CredentialsState{}, fmt.Errorf("leer las credenciales: %w", err)
	}
	company, err := sessionCompanyFor(ctx, row.CompanyID)
	if err != nil {
		return CredentialsState{}, err
	}
	st.Configured, st.ClientID, st.UpdatedBy = true, row.ClientID, row.UpdatedByName
	st.UpdatedAt = &row.UpdatedAt
	// Se intenta descifrar para saber si ESTE ambiente puede leerlas. Con el servicio de llaves
	// caído no se afirma nada: decir «por recapturar» mandaría a reemplazar una credencial buena.
	_, err = s.cipher.Decrypt(ctx, row.ClientSecretEncrypted,
		domain.CredentialAAD(company, platformID, domain.SecretClientSecret))
	switch {
	case errors.Is(err, secrets.ErrUnreadable):
		st.NeedsRecapture = true
	case err != nil:
		slog.WarnContext(ctx, "platform_credentials_unavailable", "platform_id", platformID, "error", err)
	}
	return st, nil
}

// Save captura o reemplaza las credenciales de la app. SOLO SE GUARDAN SI LA PLATAFORMA LAS ACEPTA:
// un dedazo que se guarda se descubre con un pedido real esperando.
//
// `company` entra explícita porque es la AAD con la que se cifra, y tiene que ser la del usuario que
// captura — la misma que el middleware fijó como tenant.
func (s *PlatformCredentialsService) Save(ctx context.Context, company, user int64, platformID int16, clientID, clientSecret string) error {
	creds, err := domain.NormalizeAppCredentials(clientID, clientSecret)
	if err != nil {
		return err
	}
	plat, err := s.store.QC(ctx).GetPlatformByID(ctx, platformID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Va por aquí y no por la FK: los chequeos de integridad saltan RLS.
		return fmt.Errorf("%w: esa plataforma no existe para esta empresa", domain.ErrValidation)
	}
	if err != nil {
		return fmt.Errorf("plataforma %d: %w", platformID, err)
	}
	factory := s.factories[plat.Name]
	if factory == nil {
		return fmt.Errorf("%w: %s todavía no se puede conectar desde aquí", domain.ErrValidation, plat.Name)
	}

	client, err := factory(creds)
	if err != nil {
		return fmt.Errorf("armar el cliente de %s: %w", plat.Name, err)
	}
	if err := client.Verify(ctx); err != nil {
		return err
	}

	ciphertext, err := s.cipher.Encrypt(ctx, []byte(creds.ClientSecret),
		domain.CredentialAAD(company, platformID, domain.SecretClientSecret))
	if err != nil {
		return keyServiceError(ctx, "cifrar el client secret", err)
	}
	err = s.store.QC(ctx).UpsertPlatformCredential(ctx, db.UpsertPlatformCredentialParams{
		DeliveryPlatformID: platformID, ClientID: creds.ClientID,
		ClientSecretEncrypted: ciphertext, UpdatedBy: user,
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23503" {
		return fmt.Errorf("%w: esa plataforma no existe para esta empresa", domain.ErrValidation)
	}
	if err != nil {
		return fmt.Errorf("guardar las credenciales: %w", err)
	}

	// El cliente que se acaba de comprobar es el que se queda atendiendo: ya trae su token.
	s.mu.Lock()
	s.clients[clientKey{company, platformID}] = builtClient{fingerprint: fingerprintOf(creds.ClientID, ciphertext), client: client}
	s.mu.Unlock()
	return nil
}

func (s *PlatformCredentialsService) MenuReaderFor(ctx context.Context, platform string) (LectorDeMenu, error) {
	return s.clientFor(ctx, platform)
}

func (s *PlatformCredentialsService) OrderDeciderFor(ctx context.Context, platform string) (DecisorDePedidos, error) {
	return s.clientFor(ctx, platform)
}

// clientFor lee la credencial de la empresa del contexto —bajo RLS— y devuelve su cliente. La fila
// manda: la memoria de clientes solo evita volver a armar el mismo.
func (s *PlatformCredentialsService) clientFor(ctx context.Context, platform string) (PlatformClient, error) {
	factory := s.factories[platform]
	if factory == nil {
		return nil, fmt.Errorf("%w (%s)", domain.ErrPlataformaSinCredenciales, platform)
	}
	row, err := s.store.QC(ctx).GetPlatformCredentialByName(ctx, platform)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w (%s)", domain.ErrPlataformaSinCredenciales, platform)
	}
	if err != nil {
		return nil, fmt.Errorf("leer las credenciales de %s: %w", platform, err)
	}

	company, err := sessionCompanyFor(ctx, row.CompanyID)
	if err != nil {
		return nil, err
	}
	key := clientKey{company, row.DeliveryPlatformID}
	fp := fingerprintOf(row.ClientID, row.ClientSecretEncrypted)
	s.mu.Lock()
	built, found := s.clients[key]
	s.mu.Unlock()
	if found && built.fingerprint == fp {
		return built.client, nil
	}

	secret, err := s.cipher.Decrypt(ctx, row.ClientSecretEncrypted,
		domain.CredentialAAD(company, row.DeliveryPlatformID, domain.SecretClientSecret))
	if errors.Is(err, secrets.ErrUnreadable) {
		return nil, fmt.Errorf("%w (%s)", domain.ErrCredentialsUnreadable, platform)
	}
	if err != nil {
		return nil, keyServiceError(ctx, "descifrar las credenciales de "+platform, err)
	}
	client, err := factory(domain.AppCredentials{ClientID: row.ClientID, ClientSecret: string(secret)})
	if err != nil {
		return nil, fmt.Errorf("armar el cliente de %s: %w", platform, err)
	}
	s.mu.Lock()
	s.clients[key] = builtClient{fingerprint: fp, client: client}
	s.mu.Unlock()
	return client, nil
}

// errForeignRow: RLS dejó pasar una fila que no es de la sesión. No debería ocurrir nunca, y por
// eso no se trata como «sin credenciales»: es un 500 con su evento, para que se vea.
var errForeignRow = errors.New("a row from another company reached this session: RLS did not isolate it")

// sessionCompanyFor devuelve la empresa de la SESIÓN y exige que la fila sea suya. Es la segunda
// barrera además de RLS: la AAD se arma con la empresa de la sesión, nunca con la que dice la fila,
// así que aunque RLS falle el secreto de una empresa no descifra para otra. Sin sesión se niega:
// adivinar la empresa es justo lo que esta barrera existe para no hacer.
func sessionCompanyFor(ctx context.Context, rowCompany int64) (int64, error) {
	company, ok := store.CompanyFrom(ctx)
	if !ok {
		return 0, fmt.Errorf("%w: no company in the session", errForeignRow)
	}
	if company != rowCompany {
		logging.SecurityEvent(ctx, "tenant_row_mismatch", "session_company", company, "row_company", rowCompany)
		return 0, errForeignRow
	}
	return company, nil
}

// keyServiceError traduce la falla del servicio de llaves a su error de dominio, para que se
// responda 503 «intenta de nuevo» y no un 500 sin nombre. El detalle —el estado HTTP de Google—
// queda en el log, nunca en la respuesta.
func keyServiceError(ctx context.Context, what string, err error) error {
	if errors.Is(err, secrets.ErrUnavailable) {
		slog.WarnContext(ctx, "key_service_unavailable", "operation", what, "error", err)
		return fmt.Errorf("%s: %w", what, domain.ErrKeyServiceUnavailable)
	}
	return fmt.Errorf("%s: %w", what, err)
}
