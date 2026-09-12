package domain

import "errors"

// Operador es quien VENDE y mantiene el producto: no pertenece a ninguna empresa y no aparece en
// `users`. Esa ausencia es la separación — el login del negocio consulta `users`, así que no puede
// encontrarlo aunque alguien registre el mismo nombre en las dos superficies.
//
// Sin rol: la consola tiene una sola clase de acceso hoy. Cuando haga falta distinguir, será una
// columna nueva y no un rol de negocio reutilizado, que es la ambigüedad que ya costó caro.
type Operador struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Activo   bool   `json:"activo"`
}

// ErrCredencialDePlataforma es el ÚNICO error de autenticación de la consola, a propósito.
//
// Usuario inexistente, contraseña equivocada y operador desactivado devuelven este mismo valor: la
// consola se sirve en un subdominio público y distinguirlos le diría a quien toca la puerta cuáles
// usuarios existen. Tener un solo sentinel hace que la fuga no dependa de que cada handler nuevo
// se acuerde de no distinguirlos.
var ErrCredencialDePlataforma = errors.New("credenciales de plataforma inválidas")

// PuedeEntrar dice si el operador puede iniciar sesión hoy.
//
// Desactivar es como se retira el acceso sin borrar el rastro de quién hizo qué, y tiene que
// morder en el siguiente request: una sesión que sigue viva porque el token no ha caducado es
// justo lo que "retirar el acceso" no puede significar.
func (o Operador) PuedeEntrar() error {
	if !o.Activo {
		return ErrCredencialDePlataforma
	}
	return nil
}
