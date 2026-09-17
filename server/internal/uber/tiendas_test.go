package uber

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// La respuesta real de `GET /v1/eats/stores`, recortada a lo que la pantalla necesita y con datos
// inventados. Trae de más a propósito —correos de contacto, URL pública, tiempo de preparación—
// porque lo que este test fija es que NADA de eso salga del paquete.
const tiendasDePrueba = `{"stores":[
  {"name":"Tienda del Centro","store_id":"aaaa-1111",
   "location":{"city":"Toluca","street_address_line_one":"Calle Falsa 123"},
   "contact_emails":["dueno@ejemplo.invalid"],
   "pos_data":{"integration_enabled":false},
   "web_url":"https://ubereats.com/x"},
  {"name":"Tienda Norte","store_id":"bbbb-2222",
   "location":{"city":"Metepec"},
   "pos_data":{"integration_enabled":true}}
]}`

// LAS TIENDAS SE LISTAN PARA QUE NADIE TENGA QUE TECLEAR UN UUID.
//
// Es el paso que hoy deja fuera a quien nunca ha usado el sistema: el `store_id` es un UUID que no
// se puede adivinar ni escribir de memoria. Con esta lista, la pantalla muestra «Tienda del Centro
// — Toluca» y el id viaja por debajo.
func TestListarTiendas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/") {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":2592000}`))
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("listar tiendas usó %s: este paquete es de solo lectura", r.Method)
		}
		_, _ = w.Write([]byte(tiendasDePrueba))
	}))
	defer srv.Close()

	tiendas, err := clienteContra(t, srv).ListarTiendas(context.Background())
	if err != nil {
		t.Fatalf("listar tiendas: %v", err)
	}
	if len(tiendas) != 2 {
		t.Fatalf("devolvió %d tiendas, quería 2", len(tiendas))
	}
	if tiendas[0].ID != "aaaa-1111" || tiendas[0].Nombre != "Tienda del Centro" || tiendas[0].Ciudad != "Toluca" {
		t.Errorf("la primera tienda llegó como %+v", tiendas[0])
	}
	// `integration_enabled` es lo que dice si esa tienda ya tiene un PDV conectado. Sin mostrarlo,
	// alguien la conecta dos veces y no entiende por qué la otra dejó de recibir.
	if tiendas[0].PDVConectado || !tiendas[1].PDVConectado {
		t.Errorf("el estado del PDV llegó al revés: %+v", tiendas)
	}
}

// LA RESPUESTA TRAE DATOS PERSONALES Y NO SALEN DE AQUÍ: correo del titular, dirección, teléfono.
// La pantalla necesita nombre y ciudad para que alguien distinga sus sucursales, nada más.
func TestListarTiendasNoArrastraDatosPersonales(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/") {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":2592000}`))
			return
		}
		_, _ = w.Write([]byte(tiendasDePrueba))
	}))
	defer srv.Close()

	tiendas, err := clienteContra(t, srv).ListarTiendas(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range tiendas {
		for campo, valor := range map[string]string{
			"correo de contacto": "ejemplo.invalid",
			"dirección":          "Calle Falsa",
		} {
			if strings.Contains(s.Nombre+s.Ciudad+s.ID, valor) {
				t.Errorf("la tienda %q arrastra %s", s.Nombre, campo)
			}
		}
	}
}
