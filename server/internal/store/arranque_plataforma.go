package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// AssertPlatformGrants verifica en runtime que la consola de plataforma sirve con SU rol y no con
// uno prestado: (1) no es superusuario ni tiene `bypassrls`; (2) prueba FUNCIONAL — un `select`
// canario sobre `orders` tiene que fallar con `42501`.
//
// La segunda mitad es la que atrapa el caso real: un `PLATFORM_DATABASE_URL` copiado de
// `APP_DATABASE_URL` pasa la primera —el rol del negocio tampoco es superusuario— y leería pedidos,
// pagos y usuarios de todas las empresas sin que nada fallara. Los grants son la ÚNICA barrera de
// la consola, así que se comprueba preguntando por lo que no debe poder, no por lo que debería.
//
// Vive aquí y no en `cmd/api` por una razón concreta: en `main` no se puede probar contra las tres
// conexiones que importan (plataforma, dueño y negocio), y un chequeo de seguridad sin test es una
// intención, no un control.
func AssertPlatformGrants(ctx context.Context, st *Store) error {
	var bypass bool
	if err := st.Pool.QueryRow(ctx,
		"select coalesce(bool_or(rolsuper or rolbypassrls), false) from pg_roles where rolname = current_user").Scan(&bypass); err != nil {
		return fmt.Errorf("leer el rol de la consola: %w", err)
	}
	if bypass {
		return errors.New("el rol de la consola es superusuario o salta RLS: sus grants dejarían de ser una barrera y vería la operación de todas las empresas")
	}

	var uno int
	err := st.Pool.QueryRow(ctx, "select 1 from orders limit 1").Scan(&uno)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42501" {
		return nil // permiso denegado: es exactamente lo que se espera.
	}
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		return errors.New("el rol de la consola puede leer `orders`: está sirviendo con una conexión del negocio (revisa PLATFORM_DATABASE_URL)")
	}
	return fmt.Errorf("el canario sobre `orders` falló por algo distinto a un permiso denegado: %w", err)
}
