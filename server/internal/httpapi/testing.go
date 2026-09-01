package httpapi

import "context"

// ConUsuarioDePrueba inyecta la sesión del dispositivo en el contexto.
//
// Existe solo para los tests de integración, que viven en otro paquete y no pueden alcanzar la
// clave privada del contexto. No se usa en producción.
func ConUsuarioDePrueba(ctx context.Context, userID, companyID int64) context.Context {
	return context.WithValue(ctx, userCtxKey, AuthUser{ID: userID, CompanyID: companyID})
}
