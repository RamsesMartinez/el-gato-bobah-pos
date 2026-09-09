package domain

import (
	"errors"
	"strings"
	"testing"
)

// El folio con el que la plataforma nombra al pedido.
//
// Los casos están escritos contra los BORDES, no contra el camino feliz: lo que decide si un test
// aquí vale es qué defecto concreto atrapa. Los cuatro primeros vienen de la lista de bordes del
// spec, enumerada antes de escribir una línea de código.
func TestNormalizePlatformRef(t *testing.T) {
	casos := []struct {
		nombre   string
		entrada  string
		esperado string
		malo     bool
		queRompe string
	}{
		{
			nombre: "recorta solo los extremos", entrada: "  4B2E9A10-77C3  ", esperado: "4B2E9A10-77C3",
			queRompe: "pegar del reporte deja espacios, y sin recortarlos el mismo folio no se parece a sí mismo",
		},
		{
			nombre: "no toca mayúsculas ni guiones", entrada: "4b2e-9A10", esperado: "4b2e-9A10",
			queRompe: "cualquier transformación destruye el valor que sirve para conciliar contra el documento",
		},
		{
			nombre: "no toca los espacios de EN MEDIO", entrada: "ABC 123", esperado: "ABC 123",
			queRompe: "recortar de más cambia el identificador; el recorte es solo de los extremos",
		},
		{
			nombre: "cadena vacía", entrada: "", malo: true,
			queRompe: `"" no es "sin folio": guardarlo hace que dos pedidos sin folio choquen contra la unicidad`,
		},
		{
			nombre: "solo espacios", entrada: "     ", malo: true,
			queRompe: "es la cadena vacía por otro camino, y el caso típico de un campo que se tocó sin escribir",
		},
		{
			nombre: "solo tabuladores y saltos", entrada: "\t\n ", malo: true,
			queRompe: "TrimSpace recorta espacio Unicode, así que esto también queda vacío",
		},
		{
			nombre: "justo en el tope", entrada: strings.Repeat("x", MaxPlatformRefLen),
			esperado: strings.Repeat("x", MaxPlatformRefLen),
			queRompe: "el tope es inclusivo; rechazarlo tiraría un folio legítimo",
		},
		{
			nombre: "un carácter más que el tope", entrada: strings.Repeat("x", MaxPlatformRefLen+1), malo: true,
			queRompe: "sin cota, un pegado accidental de media pantalla entra a la columna",
		},
		{
			nombre: "el tope se mide DESPUÉS de recortar", entrada: "  " + strings.Repeat("x", MaxPlatformRefLen) + "  ",
			esperado: strings.Repeat("x", MaxPlatformRefLen),
			queRompe: "medir antes rechazaría un folio que cabe, solo por venir pegado con espacios",
		},
		{
			nombre: "el UUID de Uber cabe", entrada: "4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44",
			esperado: "4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44",
			queRompe: "son 36 caracteres y es el formato más largo que se conoce hoy",
		},
		{
			nombre: "el entero de 19 dígitos de DiDi cabe", entrada: "1234567890123456789",
			esperado: "1234567890123456789",
			queRompe: "no se guarda como número: ningún entero cubre a las tres plataformas",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := NormalizePlatformRef(c.entrada)
			if c.malo {
				if err == nil {
					t.Fatalf("se aceptó %q y debía rechazarse: %s", c.entrada, c.queRompe)
				}
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("el rechazo no es ErrValidation, así que la frontera lo mapearía a 500 en vez de 400: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("se rechazó %q y es válido (%s): %v", c.entrada, c.queRompe, err)
			}
			if got != c.esperado {
				t.Fatalf("%q → %q, se esperaba %q: %s", c.entrada, got, c.esperado, c.queRompe)
			}
		})
	}
}

// Quién puede llevar folio, probado SIN base de datos.
func TestPlatformRefDelPedido(t *testing.T) {
	uber := int16(6)
	folio := "  UBER-1  "
	vacio := "   "

	t.Run("sin folio no es error: es la salida explícita", func(t *testing.T) {
		got, err := PlatformRefDelPedido(nil, &uber)
		if err != nil || got != nil {
			t.Fatalf("mandar sin folio tiene que pasar y quedar en nil (got=%v err=%v)", got, err)
		}
	})
	t.Run("con plataforma se recorta", func(t *testing.T) {
		got, err := PlatformRefDelPedido(&folio, &uber)
		if err != nil {
			t.Fatalf("un folio válido se rechazó: %v", err)
		}
		if *got != "UBER-1" {
			t.Fatalf("quedó %q y debía recortarse a %q", *got, "UBER-1")
		}
	})
	t.Run("sin plataforma se rechaza", func(t *testing.T) {
		// La pantalla no ofrece el campo en mostrador, pero un cliente de API sí puede mandarlo y
		// el servidor es la única barrera.
		_, err := PlatformRefDelPedido(&folio, nil)
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("un pedido de mostrador aceptó folio de plataforma: %v", err)
		}
	})
	t.Run("vacío se rechaza aunque haya plataforma", func(t *testing.T) {
		_, err := PlatformRefDelPedido(&vacio, &uber)
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("se aceptó un folio de puros espacios: %v", err)
		}
	})
}

// Un carácter de control se rechaza como ENTRADA INVÁLIDA, no como error del servidor.
//
// El byte NUL pasa TrimSpace, no deja la cadena vacía y cuenta como una runa, así que llegaba hasta
// el driver de Postgres: `invalid byte sequence for encoding "UTF8"` (22021), que el mapeo no
// reconoce y sale como 500. Un 500 dice "el servidor se rompió" y manda a revisar logs por un dato
// que el cliente mandó mal.
func TestUnCaracterDeControlEnElFolioEs400YNo500(t *testing.T) {
	for _, malo := range []string{"AB\x00C", "UBER\x01", "\x7f", "linea1\nlinea2"} {
		_, err := NormalizePlatformRef(malo)
		if err == nil {
			t.Fatalf("se aceptó %q: llega hasta Postgres y sale como 500", malo)
		}
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("%q se rechazó pero no como ErrValidation: %v", malo, err)
		}
	}
}
