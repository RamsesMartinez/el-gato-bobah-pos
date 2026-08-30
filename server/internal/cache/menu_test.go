package cache

import "testing"

// La llave del menú lleva la versión de la FORMA del documento. Sin ella, un deploy que agrega un
// campo sigue sirviendo el documento viejo hasta que venza el TTL de 24 horas, sin nada que lo
// delate: pasó al agregar las listas de precios por plataforma — el selector no aparecía en el POS
// y la base sí tenía las plataformas.
func TestLaLlaveDelMenuLlevaVersionYEmpresa(t *testing.T) {
	k1 := menuKey(1)
	k2 := menuKey(2)
	if k1 == k2 {
		t.Fatal("dos empresas no pueden compartir la llave del menú")
	}
	for _, k := range []string{k1, k2} {
		if !contiene(k, menuSchema) {
			t.Fatalf("la llave %q debe llevar la versión del documento (%s)", k, menuSchema)
		}
	}
	if k1 != "pos:menu:"+menuSchema+":1" {
		t.Fatalf("forma inesperada de la llave: %s", k1)
	}
}

func contiene(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
