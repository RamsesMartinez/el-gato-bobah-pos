package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// La clave del aparato es una pista de auditoría que genera el navegador: se guarda solo si tiene
// la forma exacta, y una clave mala NUNCA rechaza la acción (una venta no truena por un header).
func TestNormalizeOriginKeepsOnlyAWellFormedDeviceKey(t *testing.T) {
	cases := []struct {
		name, key string
		keep      bool
	}{
		{"bien formada", "0123456789abcdef0123456789abcdef", true},
		{"vacía", "", false},
		{"31 caracteres", "0123456789abcdef0123456789abcde", false},
		{"33 caracteres", "0123456789abcdef0123456789abcdef0", false},
		{"mayúsculas", "0123456789ABCDEF0123456789ABCDEF", false},
		{"inyección", "0123456789abcdef'; drop table x--", false},
		{"no hex", "zzzz456789abcdef0123456789abcdef", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := NormalizeOrigin("187.190.4.12", c.key, "")
			if (o.DeviceKey != nil) != c.keep {
				t.Fatalf("DeviceKey = %v, quiere guardada=%v", o.DeviceKey, c.keep)
			}
			if c.keep && *o.DeviceKey != c.key {
				t.Fatalf("DeviceKey = %q, quiere %q", *o.DeviceKey, c.key)
			}
		})
	}
}

// La descripción solo guarda las llaves conocidas y nunca pasa de 300 bytes: viene del navegador y
// se guarda en cada evento.
func TestNormalizeOriginTrimsTheDeviceDescription(t *testing.T) {
	info := `{"platform":"Windows","platformVersion":"15.0.0","model":"","browser":"Edge 141","screen":"1368x912","evil":"` + strings.Repeat("x", 500) + `"}`
	o := NormalizeOrigin("1.2.3.4", "", info)
	if len(o.Device) == 0 || len(o.Device) > 300 {
		t.Fatalf("la descripción mide %d bytes", len(o.Device))
	}
	var m map[string]string
	if err := json.Unmarshal(o.Device, &m); err != nil {
		t.Fatalf("la descripción guardada no es JSON: %v", err)
	}
	if _, ok := m["evil"]; ok {
		t.Fatal("se guardó una llave desconocida")
	}
	if m["browser"] != "Edge 141" || m["screen"] != "1368x912" {
		t.Fatalf("se perdió lo que sí describe al aparato: %v", m)
	}

	long := `{"browser":"` + strings.Repeat("y", 1000) + `"}`
	if o := NormalizeOrigin("", "", long); len(o.Device) > 300 {
		t.Fatalf("un valor largo dejó la descripción en %d bytes", len(o.Device))
	}
	for _, bad := range []string{"", "no es json", "[1,2]", "null"} {
		if o := NormalizeOrigin("", "", bad); o.Device != nil {
			t.Errorf("descripción %q guardó %s; debía quedar vacía", bad, o.Device)
		}
	}
}

func TestNormalizeOriginKeepsTheIPOnlyWhenItParses(t *testing.T) {
	if o := NormalizeOrigin("187.190.4.12", "", ""); o.IP == nil || o.IP.String() != "187.190.4.12" {
		t.Fatalf("IP = %v", o.IP)
	}
	if o := NormalizeOrigin("187.190.4.12:5555", "", ""); o.IP != nil {
		t.Fatalf("una IP con puerto se guardó como %v; el puerto lo quita quien la extrae", o.IP)
	}
	if o := NormalizeOrigin("", "", ""); o.IP != nil {
		t.Fatalf("sin IP guardó %v", o.IP)
	}
}
