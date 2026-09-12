package auth

import (
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// EL TEST QUE JUSTIFICA TODO EL DISEÑO.
//
// Un token de la consola no vale en el negocio y uno del negocio no vale en la consola. Con un
// solo secreto y un claim que los distinga, ese ataque se rechaza con un `if` —y un `if` se olvida
// al agregar la siguiente ruta—. Con dos secretos no se puede construir: no hay código que
// escribir mal.
func TestUnTokenDeUnaSuperficieNoValeEnLaOtra(t *testing.T) {
	const secretoDelNegocio = "0123456789abcdef0123456789abcdef"
	const secretoDeLaConsola = "fedcba9876543210fedcba9876543210"

	negocio := NewManager(secretoDelNegocio, nil)
	consola := NewManagerDePlataforma(secretoDeLaConsola, nil)

	delNegocio, err := negocio.Issue(domain.User{ID: 7, Name: "Ana", Role: domain.RoleAdmin, CompanyID: 1})
	if err != nil {
		t.Fatalf("emitir token del negocio: %v", err)
	}
	deLaConsola, err := consola.Issue(domain.Operador{ID: 1, Username: "soporte", Name: "Soporte", Activo: true})
	if err != nil {
		t.Fatalf("emitir token de la consola: %v", err)
	}

	if _, err := consola.Parse(delNegocio); err == nil {
		t.Fatal("la consola aceptó un token del negocio: el admin de cualquier cliente entraría a la consola de plataforma")
	}
	if _, err := negocio.Parse(deLaConsola); err == nil {
		t.Fatal("el negocio aceptó un token de la consola: un operador de plataforma entraría al POS de un cliente")
	}

	// Cada uno sí valida el suyo, o el test de arriba pasaría con dos managers rotos.
	if _, err := consola.Parse(deLaConsola); err != nil {
		t.Fatalf("la consola no validó su propio token: %v", err)
	}
	if _, err := negocio.Parse(delNegocio); err != nil {
		t.Fatalf("el negocio no validó su propio token: %v", err)
	}
}

// Y por qué `config.Validate` se niega a arrancar con los dos secretos iguales.
//
// Esto no es una hipótesis: con el MISMO secreto, un token del negocio valida en la consola sin
// que nada falle. La separación entera se apoya en que las dos firmas no coincidan, así que la
// comprobación de configuración no es celo — es la única cosa que sostiene lo de arriba.
func TestConElMismoSecretoLaSeparacionDesaparece(t *testing.T) {
	const unoSolo = "0123456789abcdef0123456789abcdef"

	delNegocio, err := NewManager(unoSolo, nil).Issue(domain.User{ID: 7, Name: "Ana", Role: domain.RoleAdmin, CompanyID: 1})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if _, err := NewManagerDePlataforma(unoSolo, nil).Parse(delNegocio); err != nil {
		t.Fatalf("la consola rechazó el token del negocio pese a compartir secreto (%v). Si es por una barrera nueva —un claim que se verifique, por ejemplo—, este test ya no describe la realidad: bórralo y revisa si `config.Validate` todavía tiene que exigir secretos distintos", err)
	}
}

// El operador que sale del token es el que entró, con su nombre y sin empresa: la consola no
// tiene tenant y un claim de empresa ahí sería el primer paso para mezclarla con el negocio.
func TestElTokenDeLaConsolaNoLlevaEmpresa(t *testing.T) {
	consola := NewManagerDePlataforma("fedcba9876543210fedcba9876543210", nil)
	tok, err := consola.Issue(domain.Operador{ID: 42, Username: "soporte", Name: "Soporte", Activo: true})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	c, err := consola.Parse(tok)
	if err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if c.OperadorID != 42 || c.Username != "soporte" {
		t.Fatalf("claims = %+v, quiere el operador 42/soporte", c)
	}
}
