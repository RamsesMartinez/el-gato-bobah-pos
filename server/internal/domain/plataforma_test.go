package domain

import (
	"errors"
	"strings"
	"testing"
)

// Un operador desactivado se rechaza IGUAL que uno que no existe.
//
// No es una preferencia de redacción: la consola vive en un subdominio público y el único que
// entra ahí somos nosotros, así que "ese usuario existe pero está apagado" le regala a quien toca
// la puerta la mitad del trabajo. Que el dominio tenga UN SOLO error para los dos casos hace que la
// fuga no dependa de que cada handler se acuerde de no distinguirlos.
func TestElOperadorInactivoSeRechazaComoSiNoExistiera(t *testing.T) {
	casos := []struct {
		nombre  string
		op      Operador
		quiere  error
		entrada bool
	}{
		{nombre: "activo", op: Operador{ID: 1, Username: "soporte", Activo: true}, entrada: true},
		{nombre: "desactivado", op: Operador{ID: 1, Username: "soporte"}, quiere: ErrCredencialDePlataforma},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := c.op.PuedeEntrar()
			if c.entrada && err != nil {
				t.Fatalf("PuedeEntrar() = %v, quiere nil", err)
			}
			if !c.entrada && !errors.Is(err, c.quiere) {
				t.Fatalf("PuedeEntrar() = %v, quiere %v", err, c.quiere)
			}
		})
	}

	// Y el error tiene que ser el MISMO valor que el de una credencial equivocada. Si algún día se
	// parte en dos sentinels, la consola empieza a decir cuáles usuarios existen sin que nadie lo
	// note: los dos caminos siguen respondiendo 401.
	if !errors.Is(Operador{}.PuedeEntrar(), ErrCredencialDePlataforma) {
		t.Fatal("el rechazo por inactivo dejó de ser el mismo error que el de credencial inválida: la consola filtraría qué usuarios existen")
	}
}

func TestUsuarioValido(t *testing.T) {
	casos := []struct {
		nombre  string
		usuario string
		valido  bool
	}{
		{"normal", "soporte", true},
		{"con guion y dígitos", "soporte-2", true},
		{"vacío", "", false},
		{"solo espacios", "   ", false},

		// El '@' es el separador de usuario@empresa del login del negocio. Un operador que se
		// llame así se puede teclear en la pantalla equivocada y confunde a quien lea una bitácora
		// de accesos fallidos, que es donde se mira cuando algo va mal.
		{"parece un login de negocio", "soporte@gatobobah", false},

		{"con espacio dentro", "de soporte", false},
		{"demasiado largo", strings.Repeat("a", 65), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := UsuarioValido(c.usuario); got != c.valido {
				t.Fatalf("UsuarioValido(%q) = %v, quiere %v", c.usuario, got, c.valido)
			}
		})
	}
}

// El usuario se normaliza ANTES de guardarlo y antes de buscarlo, o "soporte " y "soporte" son dos
// operadores distintos en una tabla cuyo índice único dice que no pueden serlo.
func TestNormalizarUsuario(t *testing.T) {
	for entrada, quiere := range map[string]string{
		"  soporte  ": "soporte",
		"Soporte":     "soporte",
		"soporte":     "soporte",
	} {
		if got := NormalizarUsuario(entrada); got != quiere {
			t.Errorf("NormalizarUsuario(%q) = %q, quiere %q", entrada, got, quiere)
		}
	}
}
