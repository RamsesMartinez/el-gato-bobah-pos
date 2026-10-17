package cache

import "testing"

// El ETag es lo único que decide si la tableta se queda con su menú. Cada caso nombra el cambio que
// la tableta no vería si el ETag no lo distinguiera.
func TestMenuETagChangesWithWhatChangesTheMenu(t *testing.T) {
	base := MenuETag(2, 7, "20261010")
	cases := []struct {
		name string
		tag  string
	}{
		{"otra versión de catálogo (un cambio por SQL directo)", MenuETag(2, 8, "20261010")},
		{"otro día (productos con fecha de disponibilidad)", MenuETag(2, 7, "20261011")},
		{"otra empresa en la misma tableta con la misma versión", MenuETag(1, 7, "20261010")},
	}
	for _, c := range cases {
		if c.tag == base {
			t.Errorf("%s: el ETag no cambió (%s); la tableta recibiría 304 y se quedaría con el menú viejo", c.name, base)
		}
	}
	if MenuETag(2, 7, "20261010") != base {
		t.Error("el mismo menú dio dos ETag distintos: la tableta bajaría el catálogo completo cada minuto")
	}
	if len(base) < 2 || base[0] != '"' || base[len(base)-1] != '"' {
		t.Errorf("un ETag va entre comillas (RFC 9110): %s", base)
	}
}

// La llave del menú lleva la versión de la FORMA del documento. Sin ella, un deploy que agrega un
// campo sigue sirviendo el documento viejo hasta que venza el TTL de 24 horas, sin nada que lo
// delate: pasó al agregar las listas de precios por plataforma — el selector no aparecía en el POS
// y la base sí tenía las plataformas. Y lleva la versión de catálogo y el día por lo mismo que el
// ETag (2026-10-10).
func TestLaLlaveDelMenuLlevaVersionYEmpresa(t *testing.T) {
	k1 := menuKey(1, 7, "20261010")
	cases := map[string]string{
		"otra empresa":             menuKey(2, 7, "20261010"),
		"otra versión de catálogo": menuKey(1, 8, "20261010"),
		"otro día":                 menuKey(1, 7, "20261011"),
	}
	for name, k := range cases {
		if k == k1 {
			t.Errorf("%s comparte la llave del menú (%s)", name, k)
		}
	}
	if k1 != "pos:menu:"+menuSchema+":1:7:20261010" {
		t.Fatalf("forma inesperada de la llave: %s", k1)
	}
}
