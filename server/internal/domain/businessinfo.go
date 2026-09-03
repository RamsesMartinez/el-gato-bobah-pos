package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Largos del encabezado del ticket, en CARACTERES. El ancho útil del papel de 80mm son ~32
// caracteres: los topes no son estéticos, son lo que evita que un texto largo desacomode todos los
// tickets que salgan después. Los mismos números están como check en la base.
const (
	MaxBusinessName  = 60
	MaxBusinessLine  = 120 // dirección: un renglón
	MaxBusinessPhone = 30
	// Los textos del ticket son BLOQUES, no renglones: ahí va el aviso de "sin valor fiscal" con
	// los datos de facturación, que son varias líneas. 400 caracteres son ~13 renglones de 32,
	// es decir unos 5 cm de papel por ticket — más que eso deja de ser un aviso y es un folleto.
	MaxTicketNote = 400
)

// BusinessInfo es la identidad editable que sale impresa en el ticket. Todos los campos salvo Name
// son opcionales: vacío significa "no imprimas ese renglón", no "imprime un hueco".
type BusinessInfo struct {
	Name       string
	Address    string
	Phone      string
	HeaderNote string
	FooterNote string
}

// Validate rechaza la información que no cabe en un ticket de 80mm.
func (b BusinessInfo) Validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return fmt.Errorf("%w: el nombre del negocio no puede ir vacío", ErrValidation)
	}
	limits := []struct {
		label string
		value string
		max   int
	}{
		{"el nombre del negocio", b.Name, MaxBusinessName},
		{"la dirección", b.Address, MaxBusinessLine},
		{"el teléfono", b.Phone, MaxBusinessPhone},
		{"el texto superior", b.HeaderNote, MaxTicketNote},
		{"el texto inferior", b.FooterNote, MaxTicketNote},
	}
	for _, l := range limits {
		// RuneCount y no len: "ñ" son dos bytes, y rechazar un nombre por sus acentos no tiene
		// nada que ver con cómo se ve en el papel.
		if utf8.RuneCountInString(l.value) > l.max {
			return fmt.Errorf("%w: %s no puede pasar de %d caracteres", ErrValidation, l.label, l.max)
		}
	}
	return nil
}

// PrintSettings agrupa lo que el negocio decide sobre la impresión.
//
// Van juntos y no como parámetros sueltos por dos razones. Una es de forma: cuatro booleanos en fila
// en una firma se confunden entre sí y el compilador no ayuda. La otra es de fondo: la intención
// declarada es que a medio plazo estos ajustes se vendan como paquetes, y un paquete es un
// conjunto con nombre. Tenerlo nombrado hoy es lo que evita desenredarlo después.
type PrintSettings struct {
	// AutoPrintOnClose: el ticket del cliente sale solo al cerrar el pedido.
	AutoPrintOnClose bool
	// PrintFreeModifiers: el ticket lista los adicionales que no cuestan.
	PrintFreeModifiers bool
	// KitchenCanCharge: si el tablero de Pedidos puede cobrar, además de preparar y entregar.
	// Apagado por default porque cobrar es del punto de venta, y una pantalla de cocina con botón de
	// cobrar le da acceso al dinero a quien solo tiene que preparar comida. Se enciende donde la
	// cocina y el mostrador son la misma persona en la misma máquina, que es el caso del local que
	// estrena el sistema.
	KitchenCanCharge bool

	// PrintKitchenTicket: al mandar el pedido sale una comanda SIN precios para cocina. Apagado por
	// default: donde la cocina está pegada al mostrador, sería papel que duplica lo que el cocinero
	// ya ve en la pantalla.
	PrintKitchenTicket bool

	// CorteDeVista: hasta cuándo se sigue viendo un pedido ya entregado. No decide de qué día es una
	// venta —eso lo hace el turno— y por eso vive aquí, entre los ajustes de pantalla, y no cerca de
	// nada que toque dinero.
	CorteDeVista string

	// FolioScheme: con qué se nombran los pedidos, `razas` (default) o `animales`. Vive con los
	// ajustes del papel porque es lo que sale impreso en el ticket y en la comanda, y porque es con
	// lo que cocina canta el pedido. Cambiarlo NO renombra nada ya vendido: el nombre se guarda en
	// el pedido al crearlo.
	FolioScheme string
}

// IdentitySettings: cómo se identifica quien opera una estación, y cada cuánto deja de estarlo.
//
// Los tres viven juntos porque se capturan en la misma pantalla y se guardan con el mismo UPDATE.
type IdentitySettings struct {
	// PinOnlyUnlock: desbloquear pide SOLO el PIN y el sistema deduce quién es. Apagado por
	// default — un dedazo que caiga en el PIN de otro atribuye la venta a quien no fue, en
	// silencio. Encenderlo exige PINs de 6 dígitos y únicos, y eso lo verifica el servicio.
	PinOnlyUnlock bool
	// LockAfterSeconds: segundos sin actividad antes de bloquear la pantalla. Cero = no bloquear,
	// que es una elección válida para una caja en una oficina cerrada.
	LockAfterSeconds int
	// SessionHours: horas que dura una sesión antes de exigir usuario y contraseña otra vez.
	SessionHours int
}

// Validate rechaza tiempos con los que la tableta quedaría inutilizable.
//
// Se rechazan en vez de ajustarse a un default: un bloqueo negativo o una sesión de cero horas son
// errores de captura, y "corregirlos" en silencio deja al negocio operando con un valor que nadie
// eligió — el mismo fallo que la constitución señala para los parámetros de frontera.
func (i IdentitySettings) Validate() error {
	if i.LockAfterSeconds < 0 {
		return fmt.Errorf("%w: el tiempo de bloqueo no puede ser negativo", ErrValidation)
	}
	// El tope alto no es defensivo: sin él, un dedazo de un cero de más deja la tableta bloqueada
	// para siempre y a nadie se le ocurriría buscar la causa en los ajustes.
	if i.LockAfterSeconds > maxLockSeconds {
		return fmt.Errorf("%w: el tiempo de bloqueo no puede pasar de una hora", ErrValidation)
	}
	// Una sesión de cero horas dejaría la tableta pidiendo credenciales a cada instante; el tope
	// alto (30 días) es el comportamiento que había antes de esta funcionalidad.
	if i.SessionHours < 1 || i.SessionHours > maxSessionHours {
		return fmt.Errorf("%w: la sesión debe durar entre 1 hora y 30 días", ErrValidation)
	}
	return nil
}

const (
	maxLockSeconds  = 3600
	maxSessionHours = 720
)

// DefaultIdentity son los valores con los que nace un negocio.
//
// Duplican los DEFAULT de la migración a propósito: los callers que no tocan estos ajustes —tests
// de otras cosas, o un alta de empresa— necesitan algo que pasar, y pasar el cero de Go dejaría la
// sesión en 0 horas. Si se cambian aquí, se cambian también en la migración.
func DefaultIdentity() IdentitySettings {
	return IdentitySettings{PinOnlyUnlock: false, LockAfterSeconds: 180, SessionHours: 8}
}
