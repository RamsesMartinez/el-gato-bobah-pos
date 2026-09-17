package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// Sentinels de los pedidos que llegan de una plataforma. El mapeo a HTTP vive solo en
// httpapi.Error, vía errors.Is.
var (
	// ErrFirmaInvalida es el ÚNICO error que sale cuando algo de la autenticación no cuadra: firma
	// ausente, mal formada, incorrecta, o de una tienda que no reconocemos. Distinguirlos en la
	// respuesta le diría a quien prueba a ciegas cuándo va acertando.
	ErrFirmaInvalida = errors.New("aviso de plataforma no autenticado")
	// ErrAvisoRepetido: la plataforma reintenta lo que no se le confirma y no garantiza el orden.
	ErrAvisoRepetido = errors.New("ese aviso ya se procesó")
	// ErrPedidoYaDecidido: aceptar o rechazar dos veces se rechaza DICIÉNDOLO, no en silencio.
	ErrPedidoYaDecidido = errors.New("ese pedido ya fue decidido")
	// ErrAmbienteEquivocado: procesar un aviso de producción contra el ambiente de pruebas, o al
	// revés, mezclaría un pedido real con datos de prueba.
	ErrAmbienteEquivocado = errors.New("el aviso viene de otro ambiente")
	// ErrMotivoDesconocido: un motivo de rechazo fuera de la lista de la plataforma.
	ErrMotivoDesconocido = errors.New("ese motivo de rechazo no existe")
)

// LlavesDeFirma son las llaves con las que se verifica un aviso de una empresa. Son dos para poder
// cambiar la llave sin dejar de recibir pedidos: durante la rotación las dos validan.
type LlavesDeFirma struct {
	Primaria   string
	Secundaria string
}

// Verifican dice si firma es el HMAC-SHA256 de cuerpo bajo alguna de las llaves.
//
// La comparación va con hmac.Equal y NUNCA con ==: el == de Go sale en cuanto encuentra un byte
// distinto, y esa diferencia de tiempo deja adivinar la firma correcta byte por byte. Es el mismo
// motivo por el que las ramas de login corren CheckDummySecret.
//
// El cuerpo es el CRUDO, tal como llegó. Deserializar y volver a serializar reordena llaves y la
// firma deja de cuadrar; por eso también se guarda como text y no como jsonb.
func (l LlavesDeFirma) Verifican(cuerpo []byte, firma string) bool {
	if firma == "" {
		return false
	}
	recibida, err := hex.DecodeString(firma)
	if err != nil {
		return false
	}
	// Se prueban las dos SIEMPRE, sin cortocircuito: salir antes cuando la primaria acierta hace
	// que una firma buena tarde menos que una mala, y eso es exactamente lo que hmac.Equal evita
	// dentro de cada comparación.
	ok := false
	for _, llave := range []string{l.Primaria, l.Secundaria} {
		if llave == "" {
			continue
		}
		if hmac.Equal(recibida, firmar(cuerpo, llave)) {
			ok = true
		}
	}
	return ok
}

func firmar(cuerpo []byte, llave string) []byte {
	mac := hmac.New(sha256.New, []byte(llave))
	mac.Write(cuerpo)
	return mac.Sum(nil)
}

// FirmarParaPrueba produce la firma que mandaría la plataforma. Existe porque la feature TIENE que
// poder validarse disparando avisos firmados contra el ambiente de pruebas: la plataforma todavía
// no documenta cómo generar un pedido de prueba, y esperar esa respuesta dejaría la feature sin
// forma de comprobarse.
func FirmarParaPrueba(cuerpo []byte, llave string) string {
	return hex.EncodeToString(firmar(cuerpo, llave))
}

// ClaseDeAviso agrupa los tipos de evento de la plataforma en lo que el sistema hace con ellos.
type ClaseDeAviso int

const (
	// AvisoDesconocido se confirma y NO se procesa. No confirmarlo haría que la plataforma lo
	// reintente para siempre; procesarlo a ciegas es peor.
	AvisoDesconocido ClaseDeAviso = iota
	AvisoPedidoNuevo
	AvisoPedidoCancelado
	AvisoTiendaConectada
	AvisoTiendaDesconectada
	AvisoEstadoDeTienda
)

// ClasificarAviso traduce el `event_type` de la plataforma.
//
// Llegan TODOS los tipos a la misma URL: la plataforma no ofrece filtro, así que clasificar es
// nuestro. `orders.failure` es la cancelación de las tiendas en la versión vieja de su API y
// `orders.cancel` la de las nuevas; se tratan igual porque significan lo mismo.
func ClasificarAviso(tipo string) ClaseDeAviso {
	switch tipo {
	case "orders.notification", "orders.scheduled.notification":
		return AvisoPedidoNuevo
	case "orders.cancel", "orders.failure":
		return AvisoPedidoCancelado
	case "store.provisioned":
		return AvisoTiendaConectada
	case "store.deprovisioned":
		return AvisoTiendaDesconectada
	case "store.status.changed":
		return AvisoEstadoDeTienda
	default:
		return AvisoDesconocido
	}
}

// motivosDeRechazo es la lista CERRADA que admite la plataforma. Va como constante y no como tabla
// de catálogo porque son valores que define un tercero y que no cambian con el negocio.
var motivosDeRechazo = map[string]struct{}{
	"STORE_CLOSED": {}, "POS_NOT_READY": {}, "POS_OFFLINE": {}, "ITEM_AVAILABILITY": {},
	"MISSING_ITEM": {}, "MISSING_INFO": {}, "PRICING": {}, "CAPACITY": {}, "ADDRESS": {},
	"SPECIAL_INSTRUCTIONS": {}, "OTHER": {},
}

// MotivoDeRechazoValido rechaza lo que no esté en la lista, en vez de caer a OTHER.
//
// Caer a un default mandaría a la plataforma un motivo que nadie eligió, y el cliente recibiría una
// explicación falsa de por qué no le hicieron su pedido.
func MotivoDeRechazoValido(codigo string) bool {
	_, ok := motivosDeRechazo[codigo]
	return ok
}
