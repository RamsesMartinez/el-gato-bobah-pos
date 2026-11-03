package app

import (
	"context"
	"fmt"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// ResetCompanyAdmin le pone contraseña y PIN nuevos al admin de UNA empresa, o lo crea si no
// existe.
//
// Tiene que recibir el store del ROL DE LA APLICACIÓN, no el del dueño. La consulta filtra solo por
// usuario —sqlc no ve el `company_id` de `users`— y lo que la acota a la empresa es RLS. Como dueño,
// RLS no aplica: resetear el admin de una empresa reescribía el de todas, y si la pedida no tenía
// admin no se creaba, porque el update sí encontraba la fila de otra.
func ResetCompanyAdmin(ctx context.Context, st *store.Store, companyID int64, p db.CreateUserParams) error {
	err := st.WithTenant(ctx, companyID, func(q *db.Queries) error {
		n, err := q.SetUserSecretsByUsername(ctx, db.SetUserSecretsByUsernameParams{
			Username: p.Username, PasswordHash: p.PasswordHash, PinHash: p.PinHash, PinLookup: p.PinLookup,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			_, err = q.CreateUser(ctx, p)
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("resetear el admin de la empresa %d: %w", companyID, err)
	}
	return nil
}
