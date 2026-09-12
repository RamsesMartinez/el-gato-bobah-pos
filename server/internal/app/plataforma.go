package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/migrations"
)

// PlatformService atiende la consola de quien VENDE el producto.
//
// Su store es OTRO: el pool del rol `gatobobah_platform`, que solo puede leer `companies` y
// `platform_operators`. Por eso este servicio no recibe el store del negocio ni puede recibirlo
// por error — un `select` sobre pedidos aquí no devuelve datos de más, falla con 42501.
type PlatformService struct {
	store *store.Store
	jwt   *auth.ManagerDePlataforma
	now   func() time.Time
}

func NewPlatformService(st *store.Store, jwt *auth.ManagerDePlataforma, now func() time.Time) *PlatformService {
	if now == nil {
		now = time.Now
	}
	return &PlatformService{store: st, jwt: jwt, now: now}
}

// SesionDePlataforma es lo que se lleva la consola al entrar: un acceso corto y quién es.
//
// Sin refresh, a diferencia del negocio. El POS necesita sesiones que duren un turno porque una
// tableta que pide contraseña a media comanda detiene el mostrador; la consola la abre una persona
// en una computadora y volver a entrar no le cuesta nada a nadie. Menos superficie y una familia
// de tokens menos que revocar.
type SesionDePlataforma struct {
	AccessToken string          `json:"accessToken"`
	Operador    domain.Operador `json:"operator"`
}

// Login autentica a un operador de plataforma.
//
// Los tres caminos de rechazo —no existe, contraseña equivocada, desactivado— devuelven el MISMO
// error y gastan el mismo tiempo: la consola vive en un subdominio público y responder distinto le
// diría a quien toca la puerta cuáles usuarios existen.
func (s *PlatformService) Login(ctx context.Context, usuario, password string) (*SesionDePlataforma, error) {
	usuario = domain.NormalizarUsuario(usuario)
	op, err := s.store.Q.GetPlatformOperatorByUsername(ctx, usuario)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// bcrypt de descarte: sin él la rama "no existe" responde en microsegundos y la de
			// "contraseña incorrecta" en decenas de milisegundos, y esa diferencia es la lista de
			// usuarios que existen.
			auth.CheckDummySecret(password)
			return nil, domain.ErrCredencialDePlataforma
		}
		return nil, fmt.Errorf("buscar operador: %w", err)
	}
	if !auth.CheckSecret(op.PasswordHash, password) {
		return nil, domain.ErrCredencialDePlataforma
	}
	// El desactivado se comprueba DESPUÉS de bcrypt, no antes: comprobarlo primero devolvería el
	// rechazo sin gastar el hash y delataría por tiempo a los operadores apagados.
	operador := domain.Operador{ID: op.ID, Username: op.Username, Name: op.Name, Activo: op.IsActive}
	if err := operador.PuedeEntrar(); err != nil {
		return nil, err
	}

	tok, err := s.jwt.Issue(operador)
	if err != nil {
		return nil, fmt.Errorf("emitir acceso de plataforma: %w", err)
	}
	return &SesionDePlataforma{AccessToken: tok, Operador: operador}, nil
}

// Operador vuelve a leer al operador del token. La usa el middleware en CADA request.
//
// Esto es lo que hace que desactivar a alguien le corte el acceso de inmediato y no cuando caduque
// su token: FR-013 dice "sin esperar", y un acceso de quince minutos que sigue sirviendo no es
// "sin esperar". Cuesta una lectura por llave primaria sobre una tabla de una o dos filas.
func (s *PlatformService) Operador(ctx context.Context, id int64) (domain.Operador, error) {
	op, err := s.store.Q.GetPlatformOperatorByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Operador{}, domain.ErrCredencialDePlataforma
		}
		return domain.Operador{}, fmt.Errorf("leer operador: %w", err)
	}
	operador := domain.Operador{ID: op.ID, Username: op.Username, Name: op.Name, Activo: op.IsActive}
	if err := operador.PuedeEntrar(); err != nil {
		return domain.Operador{}, err
	}
	return operador, nil
}

// EmpresaEnLaConsola es un cliente visto desde la plataforma: quién es y desde cuándo.
//
// No lleva una sola cifra de dinero (FR-008) ni nada de la gente que trabaja ahí (FR-016). No es
// que no se hayan escrito los campos: la conexión que atiende esto no tiene permiso para leerlos.
type EmpresaEnLaConsola struct {
	ID        int64     `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Activa    bool      `json:"activa"`
	CreatedAt time.Time `json:"createdAt"`
}

// EsquemaDeLaInstalacion es la versión de migración, que es de la INSTALACIÓN y no de cada
// cliente: todas las empresas comparten esquema, así que repetirla por renglón aparentaría
// informar algo por cliente que no lo es.
type EsquemaDeLaInstalacion struct {
	Version int64 `json:"version"`
}

// Empresas es el catálogo de clientes de la instalación.
type Empresas struct {
	Items  []EmpresaEnLaConsola   `json:"items"`
	Schema EsquemaDeLaInstalacion `json:"schema"`
}

// Empresas lista todas las empresas de la instalación.
func (s *PlatformService) Empresas(ctx context.Context) (Empresas, error) {
	filas, err := s.store.Q.ListCompaniesForPlatform(ctx)
	if err != nil {
		return Empresas{}, fmt.Errorf("listar empresas: %w", err)
	}
	// Lista vacía, nunca nil: una instalación sin clientes tiene que poder decirlo, y un `null`
	// hace que la pantalla truene en vez de decir que todavía no hay ninguno (FR-015).
	items := make([]EmpresaEnLaConsola, 0, len(filas))
	for _, f := range filas {
		items = append(items, EmpresaEnLaConsola{
			ID: f.ID, Slug: f.Slug, Name: f.Name, Activa: f.IsActive, CreatedAt: f.CreatedAt,
		})
	}
	version, err := migrations.VersionMaxima()
	if err != nil {
		return Empresas{}, fmt.Errorf("versión del esquema: %w", err)
	}
	return Empresas{Items: items, Schema: EsquemaDeLaInstalacion{Version: version}}, nil
}
