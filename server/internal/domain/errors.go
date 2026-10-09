package domain

import (
	"errors"
	"fmt"
)

// Sentinel domain errors. The HTTP layer maps these to status codes + error codes.
var (
	ErrNotFound           = errors.New("no encontrado")
	ErrUnauthorized       = errors.New("no autenticado")
	ErrForbidden          = errors.New("sin permisos")
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrValidation         = errors.New("datos inválidos")
	ErrConflict           = errors.New("conflicto")
	// ErrDuplicateName: ya existe un producto (por empresa) con ese nombre. Envuelve ErrConflict
	// para heredar el 409 y a la vez dar un mensaje accionable en el alta/edición/duplicado.
	ErrDuplicateName   = fmt.Errorf("ya existe un producto con ese nombre (%w)", ErrConflict)
	ErrTooManyRequests = errors.New("demasiados intentos, espera un momento")
	// ErrWeakPassword: la contraseña no cumple la política. Se envuelve con el motivo concreto
	// (longitud / común / filtrada) para que el mensaje llegue al usuario (422).
	ErrWeakPassword = errors.New("contraseña insegura")
	// ErrResetInvalid: el enlace/token de recuperación no sirve (inexistente, ya usado o vencido).
	// Distinto de ErrInvalidCredentials para dar un mensaje accionable en la pantalla de reset.
	ErrResetInvalid = errors.New("el enlace de recuperación es inválido o expiró; solicita uno nuevo")
	// ErrPaymentNeedsRegister: se pagó con un método que mueve el cajón sin decir de qué caja
	// salió. Es obligatorio y no opcional: efectivo que sale sin movimiento de caja descuadra el
	// corte, y el descuadre se descubre horas después sin saber de dónde vino.
	ErrPaymentNeedsRegister = fmt.Errorf("%w: un pago en efectivo debe indicar la caja de la que sale", ErrValidation)
	// ErrPaymentsBelowAmount: se intentó dar por pagado un gasto con pagos que no cubren el
	// importe. Envuelve ErrValidation para llegar como 4xx con mensaje accionable.
	ErrPaymentsBelowAmount = fmt.Errorf("%w: los pagos registrados no cubren el importe del gasto", ErrValidation)
	// ErrNoOpenRegister: se quiso cobrar sin la caja principal abierta. No es una validación de
	// forma sino una regla del negocio: una venta cobrada fuera de un arqueo es dinero que el corte
	// no ve, y el faltante se descubre al cerrar sin manera de reconstruir de dónde salió.
	// Solo la caja PRINCIPAL habilita el cobro — las secundarias existen para traspasos y gastos.
	ErrNoOpenRegister = errors.New("no hay una caja abierta: abre el turno antes de cobrar")
	// ErrBranchAmbiguous: la operación necesita saber en qué sucursal ocurre y no hay cómo
	// saberlo — la empresa tiene más de una activa y todavía no existe el selector (0076). Se
	// rechaza en vez de escoger la matriz: adivinar mezclaría las ventas de dos sucursales y el
	// corte de cada una dejaría de cuadrar sin que nada lo avise.
	ErrBranchAmbiguous = errors.New("este negocio tiene más de una sucursal y falta elegir en cuál trabajar")
	// ErrInvalidTimezone: la zona horaria capturada no es un nombre IANA real. Se rechaza al
	// GUARDAR y no al usar: donde se usa está el camino de una venta, que cae a UTC antes que
	// tumbar un cobro, y sin este rechazo ese fallback correría las fechas en silencio.
	ErrInvalidTimezone = fmt.Errorf("%w: la zona horaria no existe (usa un nombre como America/Mexico_City)", ErrValidation)
	// ErrPlatformNotFound: la plataforma de reparto que mandó el cliente no es de esta empresa.
	// Se resuelve bajo RLS y se rechaza: los chequeos de llave foránea de Postgres saltan RLS, así
	// que un id ajeno pasaría y —si el código cayera a margen 0— la venta se cobraría a precio de
	// mostrador en Uber, con el ticket bien impreso y el descuadre apareciendo semanas después al
	// conciliar el depósito.
	ErrPlatformNotFound = errors.New("esa plataforma de reparto no existe en este negocio")
	// ErrPaymentMethodPlatform: se cobró un pedido con un método que no corresponde a su
	// plataforma — el de otra plataforma, uno de plataforma en una venta de mostrador, o el
	// efectivo de mostrador en un pedido de plataforma.
	//
	// Es un error de DINERO, no de forma: el corte agrupa por método y el método decide si el
	// importe entra al cajón. Un pedido de Uber cobrado con el efectivo de mostrador hace que el
	// sistema espere billetes que la plataforma pagó por transferencia, y el turno cierra con un
	// faltante por el monto exacto sin nada que lo explique.
	ErrPaymentMethodPlatform = errors.New("ese método de pago no corresponde a la plataforma del pedido")
	// ErrPlatformRefTaken: el folio que la plataforma le dio a un pedido ya está en OTRO pedido de
	// la misma empresa y la misma plataforma.
	//
	// Es el caso normal, no la excepción: el folio se teclea a mano con la tablet de la plataforma
	// enfrente, y el dedazo es de todos los días. Por eso quien lo envuelve NOMBRA el pedido que ya
	// lo tiene — un aviso genérico de duplicado manda al operador a buscar a ciegas entre las
	// ventas del día, con el repartidor esperando.
	//
	// Envuelve ErrConflict (409) y no ErrValidation: el dato está bien formado, lo que no se puede
	// es que dos pedidos compartan el identificador con el que se concilia un depósito.
	ErrPlatformRefTaken = fmt.Errorf("ese folio ya está en otro pedido de la misma plataforma (%w)", ErrConflict)
	// ErrDescuentoMayorQueLaVenta: se quiso descontar más de lo que el pedido vendió. Es 422 y no
	// 400 porque el dato está bien formado —es un monto válido— y lo que falla es la regla: un
	// total negativo devolvería dinero que nadie autorizó. El mensaje lleva el máximo, para que el
	// operador no vuelva a teclear a ciegas con el cliente enfrente.
	ErrDescuentoMayorQueLaVenta = errors.New("el descuento no puede ser mayor que la venta")
	// ErrOptionOverMax: se pidió una opción de modificador más veces de las que el negocio permite
	// en una línea (`modifier_options.max_per_line`). Envuelve el nombre y los dos números para
	// que el mensaje diga qué corregir y no solo que algo está mal.
	ErrOptionOverMax = errors.New("esa opción no se puede repetir tantas veces en una línea")
	// ErrOpenOrders: se quiso cerrar la caja con pedidos sin terminar. No es un error de lo que
	// mandó el cliente sino del estado del negocio, y llega envuelto con los folios pendientes
	// para que el operador sepa cuáles resolver — un error que no dice cuáles no se puede accionar.
	ErrOpenOrders = errors.New("hay pedidos sin terminar")
)

// Dividir la cuenta (spec 027). Cada uno envuelve al sentinel base que decide el status; el texto
// es el que lee quien opera, porque httpapi.Error quita el nombre del sentinel base del mensaje.
var (
	ErrPieceAlreadyPaid           = fmt.Errorf("%w: Ese producto ya se pagó", ErrConflict)
	ErrSplitPartAlreadyCharged    = fmt.Errorf("%w: Esa parte ya se cobró", ErrConflict)
	ErrChargeKeyMismatch          = fmt.Errorf("%w: Ese cobro ya se hizo con otros productos. Vuelve a intentarlo", ErrConflict)
	ErrPaymentVoidedKey           = fmt.Errorf("%w: Ese pago ya se devolvió. Vuelve a cobrar", ErrConflict)
	ErrPaymentAlreadyVoided       = fmt.Errorf("%w: Ese pago ya se devolvió", ErrConflict)
	ErrPaymentFromClosedShift     = fmt.Errorf("%w: Ese pago es de un turno cerrado: devuélvelo desde Pedidos entregados", ErrConflict)
	ErrOrderFromClosedShift       = fmt.Errorf("%w: Ese pedido es de un turno cerrado; no se divide", ErrConflict)
	ErrPlatformOrderNotSplittable = fmt.Errorf("%w: Los pedidos de plataforma no se dividen", ErrConflict)
	ErrOrderWouldBeOverpaid       = fmt.Errorf("%w: Ya se cobró más de lo que quedaría. Primero hay que devolver un pago", ErrConflict)
	ErrMixedDeliveredPieces       = fmt.Errorf("%w: Ese producto tiene piezas entregadas y otras sin entregar. Pásalas todas juntas", ErrConflict)
	ErrAlreadyItsOwnOrder         = fmt.Errorf("%w: Ya es su propio pedido; no hace falta pasarlo", ErrConflict)
	ErrMoveKeyMismatch            = fmt.Errorf("%w: Esto ya se pasó a otro pedido", ErrConflict)
	ErrOrderHasPayments           = fmt.Errorf("%w: Tiene pagos: hay que devolverlos primero", ErrConflict)
	ErrNoProducts                 = fmt.Errorf("%w: Este pedido ya no tiene productos: ciérralo", ErrConflict)
	ErrDiscountWithPayments       = fmt.Errorf("%w: Ya hay pagos; el descuento se pone antes de cobrar", ErrConflict)
	ErrOneChargeShape             = fmt.Errorf("%w: Elige una sola forma de cobrar", ErrValidation)
	ErrEmptySelection             = fmt.Errorf("%w: Elige qué productos paga", ErrValidation)
	ErrTooManyPieces              = fmt.Errorf("%w: No hay tantas piezas por quitar", ErrValidation)
	ErrMoveWithDiscount           = fmt.Errorf("%w: Quita el descuento antes de pasar productos", ErrConflict)
	ErrMoveTargetClosed           = fmt.Errorf("%w: Ese pedido ya no recibe productos", ErrConflict)
	ErrMoveTargetOtherShift       = fmt.Errorf("%w: Ese pedido es de otro turno", ErrConflict)
	ErrMergeWithShipping          = fmt.Errorf("%w: Ese pedido tiene envío; cóbralo o quítalo antes de juntarlo", ErrConflict)
	ErrMoveRefundedLine           = fmt.Errorf("%w: Ese producto tiene una devolución; no se puede pasar", ErrConflict)
	ErrMoveLegacyLine             = fmt.Errorf("%w: Ese producto es de un pedido viejo; no se puede pasar", ErrConflict)
	ErrOrderClosedForVoid         = fmt.Errorf("%w: Ese pedido ya se cerró; no se le pueden devolver pagos", ErrConflict)

	// Variantes por operación: el mismo rechazo dice qué hacer según desde dónde se intentó.
	ErrPieceAlreadyPaidToMove     = Reword(ErrPieceAlreadyPaid, "Ese producto ya se pagó; no se puede pasar")
	ErrPieceAlreadyPaidToRemove   = Reword(ErrPieceAlreadyPaid, "Ese producto ya se pagó. Primero hay que devolver el pago")
	ErrOrderFromClosedShiftToMove = Reword(ErrOrderFromClosedShift, "Ese pedido es de un turno cerrado; no se puede pasar")
)

// reworded conserva el sentinel para errors.Is y cambia el texto entero.
type reworded struct {
	base error
	text string
}

func (e reworded) Error() string { return e.text }
func (e reworded) Unwrap() error { return e.base }

// Reword devuelve un error que es base para errors.Is pero se lee como text.
//
// Existe porque `%w: texto` siempre arrastra el texto del sentinel delante, y hay rechazos que
// reusan un sentinel (y su status) con una frase propia que no es continuación de la de él: «Ya se
// cobraron $X sin elegir productos…» es ErrCobroExcede, pero no empieza con «no puedes cobrar…».
func Reword(base error, text string) error {
	return reworded{base: base, text: text}
}

// Una sola puerta para cobrar (spec 030). El texto es el que lee quien opera: sin «borrador»,
// «versión» ni códigos (constitución, Restricciones del producto).
var (
	// ErrDraftChanged: se quiso cambiar o quitar un renglón, o la cabecera, con una versión que ya no
	// es la de la base — otra tableta lo cambió antes (D-5). Nada se aplicó; la pantalla recarga.
	ErrDraftChanged = fmt.Errorf("%w: La cuenta cambió en otra tableta", ErrConflict)
	// ErrDraftDiscarded: la cuenta ya se descartó (a mano en otra tableta o por las 12 horas).
	ErrDraftDiscarded = fmt.Errorf("%w: Esa cuenta ya se descartó", ErrConflict)
	// ErrDraftAlreadySent: la cuenta ya es un pedido. Quien lo envuelve agrega el pedido.
	ErrDraftAlreadySent = fmt.Errorf("%w: Ya se mandó a cocina; para quitarla hay que cancelar el pedido", ErrConflict)
	// ErrOrderClosed: el pedido está pagado y entregado (D-9). La pantalla ofrece cuenta nueva.
	ErrOrderClosed = fmt.Errorf("%w: Esa cuenta ya se pagó y se entregó; lo que pidan va en una cuenta nueva", ErrConflict)
)

// Sin envolver ErrConflict ni ErrValidation a propósito: son 422 con código propio y no deben caer en
// el 409 o el 400 genérico si alguien los mueve de lugar en httpapi.Error.
var (
	// ErrPlatformOrderNoLines: a un pedido de plataforma no se le agregan productos (D-11).
	ErrPlatformOrderNoLines = errors.New("a los pedidos de plataforma no se les agregan productos")
	// ErrDraftHasOrderHeader: lo nuevo de un pedido ya enviado no tiene cliente, canal ni descuento
	// propios: son los del pedido, y se cambian ahí.
	ErrDraftHasOrderHeader = errors.New("esos datos son del pedido; se cambian en el pedido")
)
