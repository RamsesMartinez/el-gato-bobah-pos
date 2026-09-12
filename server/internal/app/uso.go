package app

import (
	"context"
	"fmt"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// UsageService mide qué se usa del sistema (spec 017).
//
// Dos caminos que no se cruzan: `Registrar` escribe con la conexión del NEGOCIO (bajo RLS, la
// empresa sale del token) y `Mapa` lee con la de la CONSOLA, que no tiene permiso sobre el grano
// fino. `Recortar` es el único que necesita al dueño, y abre su propia conexión para eso.
type UsageService struct {
	store *store.Store
}

func NewUsageService(st *store.Store) *UsageService {
	return &UsageService{store: st}
}

// retenciónEnDías: cuánto vive cada cosa.
//
// El grano fino dura poco porque su trabajo es permitir recontar si mañana se cuenta distinto, y
// darle dónde caer a las coordenadas del futuro. El agregado dura trece meses —y no doce— para
// poder comparar un mes contra el mismo mes del año pasado, que es la primera comparación que pide
// un negocio de comida.
const (
	RetencionDeEventosEnDias   = 14
	RetencionDelAgregadoEnDias = 396 // 13 meses
)

// Registrar guarda un lote de eventos y devuelve cuántos se descartaron por no estar en la lista
// blanca.
//
// Ese número es el único testigo de que una versión del front dejó de medir: si nadie lo mira, el
// mapa simplemente muestra menos, que es indistinguible de «se usó menos».
//
// El rol lo pone quien llama desde el TOKEN, nunca el cuerpo del request.
func (s *UsageService) Registrar(ctx context.Context, rol domain.Role, lote []domain.EventoDeUso) (int, error) {
	lote = domain.RecortarLoteDeUso(lote)
	agregado := domain.PreAgregarUso(lote)

	validos := 0
	for _, a := range agregado {
		validos += a.Veces
	}
	descartados := len(lote) - validos
	if len(agregado) == 0 {
		return descartados, nil
	}

	// LA SUPRESIÓN SE DECIDE AQUÍ, AL ESCRIBIR, y por eso no se puede deshacer leyendo.
	//
	// Quien leyera no podría decidirlo: la consola no tiene permiso sobre `users` para contar la
	// plantilla de un cliente, y dárselo abriría la puerta que la spec 016 cerró.
	rolAGuardar, err := s.rolQueSePuedeGuardar(ctx, rol)
	if err != nil {
		return descartados, err
	}

	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		for _, a := range agregado {
			var accion *string
			if a.Accion != "" {
				accion = &a.Accion
			}
			// El grano fino guarda UNO POR TOQUE, no uno por combinación: el día que lleve
			// coordenadas, un punto por dedo es justo lo que hace que sirva.
			for i := 0; i < a.Veces; i++ {
				if err := q.InsertUsageEvent(ctx, db.InsertUsageEventParams{
					Screen: a.Pantalla, Action: accion, Role: rolAGuardar,
				}); err != nil {
					return fmt.Errorf("guardar evento de uso: %w", err)
				}
			}
			if err := q.UpsertUsageDaily(ctx, db.UpsertUsageDailyParams{
				Screen: a.Pantalla, Action: accion, Role: rolAGuardar, Hits: int64(a.Veces),
			}); err != nil {
				return fmt.Errorf("sumar el uso del día: %w", err)
			}
		}
		return nil
	})
	return descartados, err
}

// rolQueSePuedeGuardar devuelve el rol, o nil si guardarlo identificaría a una persona.
func (s *UsageService) rolQueSePuedeGuardar(ctx context.Context, rol domain.Role) (*db.UserRole, error) {
	if !domain.RolMedible(rol) {
		return nil, nil
	}
	activos, err := s.store.QC(ctx).CountActiveUsersByRole(ctx, string(rol))
	if err != nil {
		return nil, fmt.Errorf("contar usuarios del rol: %w", err)
	}
	if !domain.CorteDeRolPermitido(int(activos)) {
		return nil, nil
	}
	r := db.UserRole(rol)
	return &r, nil
}
