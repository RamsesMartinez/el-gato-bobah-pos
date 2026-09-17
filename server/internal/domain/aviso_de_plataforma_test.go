package domain

import "testing"

// LA FIRMA ES LA ÚNICA PUERTA. Esta ruta no tiene sesión: quien llama es la plataforma, no una
// persona. Si la verificación se puede evadir, cualquiera que conozca la URL mete pedidos en la
// cocina — y la URL no es un secreto ni el diseño supone que lo sea.
func TestVerificarFirma(t *testing.T) {
	cuerpo := []byte(`{"event_type":"orders.notification","event_id":"abc"}`)
	const primaria = "una-llave-de-al-menos-16"
	const secundaria = "la-otra-llave-de-rotacion"

	// Calculadas con el mismo algoritmo que documenta la plataforma: HMAC-SHA256 del cuerpo crudo,
	// hexadecimal en minúsculas.
	buena := FirmarParaPrueba(cuerpo, primaria)
	buenaSecundaria := FirmarParaPrueba(cuerpo, secundaria)

	casos := []struct {
		nombre string
		firma  string
		llaves LlavesDeFirma
		quiere bool
	}{
		{"la primaria firma", buena, LlavesDeFirma{Primaria: primaria}, true},
		{"la secundaria también, para poder rotar sin dejar de recibir",
			buenaSecundaria, LlavesDeFirma{Primaria: primaria, Secundaria: secundaria}, true},
		{"la primaria sigue valiendo durante la rotación",
			buena, LlavesDeFirma{Primaria: primaria, Secundaria: secundaria}, true},
		{"sin secundaria configurada, la de rotación no vale",
			buenaSecundaria, LlavesDeFirma{Primaria: primaria}, false},
		{"firma vacía", "", LlavesDeFirma{Primaria: primaria}, false},
		{"firma incorrecta", "00", LlavesDeFirma{Primaria: primaria}, false},
		// En MAYÚSCULAS también vale, y es correcto que valga: el hexadecimal en mayúsculas
		// decodifica a los MISMOS bytes. Rechazarlo no cerraría nada —quien produce una forma
		// produce la otra— y solo agregaría una manera de que un cliente legítimo falle.
		{"mayúsculas: mismo valor, otra escritura", upper(buena), LlavesDeFirma{Primaria: primaria}, true},
		{"del largo correcto pero equivocada", ceros(len(buena)), LlavesDeFirma{Primaria: primaria}, false},
		{"no es hexadecimal", "zz" + buena[2:], LlavesDeFirma{Primaria: primaria}, false},
		{"sin llave configurada nada valida", buena, LlavesDeFirma{}, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := c.llaves.Verifican(cuerpo, c.firma); got != c.quiere {
				t.Fatalf("Verifican() = %v, se esperaba %v", got, c.quiere)
			}
		})
	}
}

// Un cuerpo distinto NO puede validar con la misma firma: si validara, la firma no estaría atada al
// contenido y bastaría capturar una firma buena para mandar cualquier pedido.
func TestLaFirmaEstaAtadaAlCuerpo(t *testing.T) {
	const llave = "una-llave-de-al-menos-16"
	firma := FirmarParaPrueba([]byte(`{"total":100}`), llave)
	llaves := LlavesDeFirma{Primaria: llave}
	if llaves.Verifican([]byte(`{"total":999}`), firma) {
		t.Fatal("un cuerpo distinto validó con la firma del original")
	}
}

func upper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

func ceros(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}

// LOS TIPOS DE AVISO. La plataforma manda TODOS a la misma URL: no hay filtro del lado de allá, así
// que clasificar es nuestro. Un tipo desconocido se confirma y no se procesa — no confirmarlo haría
// que la plataforma lo reintente para siempre.
func TestClasificarAviso(t *testing.T) {
	casos := []struct {
		tipo   string
		quiere ClaseDeAviso
	}{
		{"orders.notification", AvisoPedidoNuevo},
		{"orders.scheduled.notification", AvisoPedidoNuevo},
		{"orders.cancel", AvisoPedidoCancelado},
		{"orders.failure", AvisoPedidoCancelado},
		{"store.provisioned", AvisoTiendaConectada},
		{"store.deprovisioned", AvisoTiendaDesconectada},
		{"store.status.changed", AvisoEstadoDeTienda},
		{"orders.release", AvisoDesconocido},
		{"lo.que.sea", AvisoDesconocido},
		{"", AvisoDesconocido},
	}
	for _, c := range casos {
		t.Run(c.tipo, func(t *testing.T) {
			if got := ClasificarAviso(c.tipo); got != c.quiere {
				t.Fatalf("ClasificarAviso(%q) = %v, se esperaba %v", c.tipo, got, c.quiere)
			}
		})
	}
}

// EL MOTIVO DE RECHAZO VA CONTRA UNA LISTA CERRADA, y un código desconocido se RECHAZA en vez de
// caer a «otro». Un parámetro presente y malformado que cae a un default manda a la plataforma un
// motivo que nadie eligió, y el cliente recibe una explicación falsa.
func TestMotivoDeRechazo(t *testing.T) {
	for _, bueno := range []string{
		"STORE_CLOSED", "POS_NOT_READY", "POS_OFFLINE", "ITEM_AVAILABILITY", "MISSING_ITEM",
		"MISSING_INFO", "PRICING", "CAPACITY", "ADDRESS", "SPECIAL_INSTRUCTIONS", "OTHER",
	} {
		if !MotivoDeRechazoValido(bueno) {
			t.Errorf("%s debería ser un motivo válido", bueno)
		}
	}
	for _, malo := range []string{"", "otro", "item_availability", "SE_ACABO", "OTHER ", "OTHER;"} {
		if MotivoDeRechazoValido(malo) {
			t.Errorf("%q NO debería pasar: un motivo inventado tiene que rebotar, no caer a OTHER", malo)
		}
	}
}
