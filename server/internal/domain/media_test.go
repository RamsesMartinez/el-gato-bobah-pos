package domain

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// riff arma un archivo WebP mínimo con un solo chunk: lo que el servidor lee para decidir.
func riff(fourcc string, payload []byte, total int) []byte {
	body := append([]byte(fourcc), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(body[4:], uint32(len(payload)))
	body = append(body, payload...)
	out := append([]byte("RIFF"), make([]byte, 4)...)
	out = append(out, []byte("WEBP")...)
	out = append(out, body...)
	if total > len(out) {
		out = append(out, make([]byte, total-len(out))...)
	}
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}

func vp8(w, h, total int) []byte {
	p := make([]byte, 10)
	p[0] = 0x10 // frame tag con bit de cuadro clave en 0 (sí es clave)
	p[3], p[4], p[5] = 0x9d, 0x01, 0x2a
	binary.LittleEndian.PutUint16(p[6:], uint16(w))
	binary.LittleEndian.PutUint16(p[8:], uint16(h))
	return riff("VP8 ", p, total)
}

func vp8l(w, h, total int) []byte {
	p := make([]byte, 5)
	p[0] = 0x2f
	bits := uint32(w-1) | uint32(h-1)<<14
	binary.LittleEndian.PutUint32(p[1:], bits)
	return riff("VP8L", p, total)
}

func vp8x(w, h int, flags byte, total int) []byte {
	p := make([]byte, 10)
	p[0] = flags
	put24 := func(b []byte, v int) { b[0], b[1], b[2] = byte(v), byte(v>>8), byte(v>>16) }
	put24(p[4:], w-1)
	put24(p[7:], h-1)
	return riff("VP8X", p, total)
}

func TestValidateWebP(t *testing.T) {
	lying := vp8(400, 400, 2000)
	binary.LittleEndian.PutUint32(lying[4:], 999999) // dice que pesa más de lo que llegó
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 200)...)
	noKey := vp8(400, 400, 2000)
	noKey[20] |= 0x01 // cuadro intermedio: un VP8 suelto que no es clave no trae dimensiones

	cases := []struct {
		name    string
		data    []byte
		side    int
		max     int
		wantErr error
		inMsg   string
	}{
		{"VP8 de 400 válido", vp8(400, 400, 5000), 400, MaxMediaSmallBytes, nil, ""},
		{"VP8L de 1200 válido", vp8l(1200, 1200, 5000), 1200, MaxMediaLargeBytes, nil, ""},
		{"VP8X sin animación válido", vp8x(1200, 1200, 0x10, 5000), 1200, MaxMediaLargeBytes, nil, ""},
		{"vacío", nil, 400, MaxMediaSmallBytes, ErrImageInvalid, "no llegó"},
		{"truncado a 20 bytes", vp8(400, 400, 0)[:20], 400, MaxMediaSmallBytes, ErrImageInvalid, "WebP"},
		{"PNG renombrado", png, 400, MaxMediaSmallBytes, ErrImageInvalid, "WebP"},
		{"RIFF con tamaño mentido", lying, 400, MaxMediaSmallBytes, ErrImageInvalid, "WebP"},
		{"chunk desconocido", riff("ALPH", make([]byte, 10), 100), 400, MaxMediaSmallBytes, ErrImageInvalid, "WebP"},
		{"VP8 que no es cuadro clave", noKey, 400, MaxMediaSmallBytes, ErrImageInvalid, "WebP"},
		{"VP8L sin firma", func() []byte { b := vp8l(400, 400, 100); b[20] = 0; return b }(), 400, MaxMediaSmallBytes, ErrImageInvalid, "WebP"},
		{"animado", vp8x(400, 400, 0x02, 500), 400, MaxMediaSmallBytes, ErrImageInvalid, "animada"},
		// La pantalla vuelve a codificar y quita la metadata; el MCP sube los bytes tal cual. Un EXIF
		// con GPS quedaría público un año en el dominio de medios.
		{"con EXIF", vp8x(400, 400, 0x08, 500), 400, MaxMediaSmallBytes, ErrImageInvalid, "metadatos"},
		{"con XMP", vp8x(400, 400, 0x04, 500), 400, MaxMediaSmallBytes, ErrImageInvalid, "metadatos"},
		{"VP8L de 1200x1199", vp8l(1200, 1199, 500), 1200, MaxMediaLargeBytes, ErrImageInvalid, "1200×1199"},
		{"lado de 800 en la de 1200", vp8(800, 600, 500), 1200, MaxMediaLargeBytes, ErrImageInvalid, "debe medir 1200×1200"},
		{"de 400 pesando de más", vp8(400, 400, MaxMediaSmallBytes+1), 400, MaxMediaSmallBytes, ErrImageTooLarge, "máximo es 100 KB"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateWebP(c.data, c.side, c.max)
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("debía aceptarse y se rechazó: %v", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, quería %v (otro error sale como 500 o con el código equivocado)", err, c.wantErr)
			}
			if !strings.Contains(err.Error(), c.inMsg) {
				t.Errorf("el mensaje %q no dice %q: quien sube no sabe qué corregir", err.Error(), c.inMsg)
			}
		})
	}
}

func TestMediaKeysAndURLs(t *testing.T) {
	const base = "https://media.example.com"
	hash := MediaHash([]byte("foto"))
	if len(hash) != 32 || strings.Trim(hash, "0123456789abcdef") != "" {
		t.Fatalf("hash = %q, quería 32 hex", hash)
	}
	key := MediaObjectKey(2, MediaProducts, 15, hash, MediaLargeSide)
	if want := "2/products/15/" + hash + "-1200.webp"; key != want {
		t.Fatalf("key = %q, quería %q", key, want)
	}
	url := MediaURL(base, key)
	if url != base+"/"+key {
		t.Fatalf("url = %q", url)
	}

	keys, ok := MediaKeysFromURL(base, url, 2, MediaProducts, 15)
	if !ok || len(keys) != 2 || keys[0] != "2/products/15/"+hash+"-400.webp" || keys[1] != key {
		t.Fatalf("keys = %v ok=%v; quería la de 400 derivada y la de 1200", keys, ok)
	}

	// La guarda que impide borrar lo que la API no armó: cada caso es una image_url que, de
	// aceptarse, borraría un archivo ajeno.
	foreign := []struct{ name, url string }{
		{"otra base", "https://evil.example.com/" + key},
		{"base como prefijo de otro host", "https://media.example.com.evil.com/" + key},
		{"otra empresa", MediaURL(base, MediaObjectKey(3, MediaProducts, 15, hash, 1200))},
		{"otro producto", MediaURL(base, MediaObjectKey(2, MediaProducts, 16, hash, 1200))},
		{"otra clase", MediaURL(base, MediaObjectKey(2, MediaCategories, 15, hash, 1200))},
		{"ruta con ..", base + "/2/products/15/../16/" + hash + "-1200.webp"},
		{"sin -1200", base + "/2/products/15/" + hash + "-400.webp"},
		{"hash corto", base + "/2/products/15/abc-1200.webp"},
		{"con querystring", url + "?x=1"},
		{"vacía", ""},
	}
	for _, f := range foreign {
		if _, ok := MediaKeysFromURL(base, f.url, 2, MediaProducts, 15); ok {
			t.Errorf("%s: %q se aceptó como propia; la API borraría un archivo que no armó", f.name, f.url)
		}
	}
}

func TestFoodFamily(t *testing.T) {
	want := []string{"cold_drink", "coffee", "tea", "frappe", "soda", "soup", "snack", "wings", "dessert", "breakfast", "dish"}
	if len(FoodFamilies) != len(want) {
		t.Fatalf("hay %d familias, quería %d: el enum de la 0117 y el sprite del front tienen %d", len(FoodFamilies), len(want), len(want))
	}
	for i, f := range FoodFamilies {
		if string(f) != want[i] {
			t.Errorf("familia %d = %q, quería %q", i, f, want[i])
		}
		if !ValidFoodFamily(string(f)) {
			t.Errorf("%q no es válida", f)
		}
	}
	for _, bad := range []string{"", "Coffee", "taco", " coffee"} {
		if ValidFoodFamily(bad) {
			t.Errorf("%q se aceptó como familia", bad)
		}
	}
}
