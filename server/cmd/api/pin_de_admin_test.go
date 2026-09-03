package main

import (
	"os"
	"regexp"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// EL PIN QUE PONE LA LÍNEA DE COMANDOS TIENE QUE NACER IGUAL QUE EL DE LA PANTALLA.
//
// `UsersService.Create` y `SetPIN` pasan por `prepararPin`, que valida el largo contra el modo del
// negocio y calcula la huella de búsqueda. Los tres caminos de arranque —provisionar empresa,
// crear el admin inicial, reiniciarlo— hacían `auth.HashSecret(pin)` a pelo: sin validar y SIN
// huella. Consecuencias: el índice único que impide dos PINs iguales no ve esas filas, y con el
// modo de solo-PIN encendido el admin queda con un PIN que no puede desbloquear, en silencio.
//
// Es el mismo defecto que ya se corrigió en `Create` y que quedó vivo en los hermanos que no se
// movieron.
func TestElPinDeAdminNaceConSuHuella(t *testing.T) {
	const pepper = "pepper-de-prueba"

	hash, lookup, err := pinDeAdmin("482715", pepper)
	if err != nil {
		t.Fatalf("pinDeAdmin: %v", err)
	}
	if hash == nil || !auth.CheckSecret(*hash, "482715") {
		t.Fatal("el hash no verifica el PIN")
	}
	if lookup == nil || *lookup != domain.PinLookup("482715", pepper) {
		t.Error("el PIN nace sin huella: el índice único que impide dos PINs iguales no lo ve")
	}

	// Sin secreto no hay huella, y eso está bien: un HMAC con clave vacía sería invertible por
	// cualquiera. Lo que no puede es fallar — el negocio funciona sin el modo de solo-PIN.
	if _, l, err := pinDeAdmin("482715", ""); err != nil || l != nil {
		t.Errorf("sin pepper: lookup=%v err=%v; quiere nil, nil", l, err)
	}

	// Y un PIN que la pantalla rechazaría, aquí también.
	if _, _, err := pinDeAdmin("1234", pepper); err == nil {
		t.Error("la línea de comandos acepta un PIN que la pantalla rechaza")
	}

	// Sin PIN no es un error: el admin entra con usuario y contraseña.
	if h, l, err := pinDeAdmin("", pepper); err != nil || h != nil || l != nil {
		t.Errorf("sin PIN: hash=%v lookup=%v err=%v; quiere nil, nil, nil", h, l, err)
	}
}

// Y que ningún camino se vuelva a saltar el helper: los hermanos que no se movieron son el modo de
// fallo de este archivo.
func TestNingunCaminoHasheaElPinPorSuCuenta(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("leer main.go: %v", err)
	}
	// Se recorta el propio helper: ahí el hash es su trabajo.
	helper := regexp.MustCompile(`(?s)func pinDeAdmin\(.*?
}
`)
	fuera := helper.ReplaceAll(src, nil)

	suelto := regexp.MustCompile(`auth\.HashSecret\(pin\)`)
	if m := suelto.Find(fuera); m != nil {
		t.Errorf("hay un camino que hashea el PIN sin pasar por pinDeAdmin: %s", m)
	}
}
