package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// Notificaciones de inventario (spec 084). En el código todo se llama «notification»: «alert» ya es
// de los avisos de error del servidor (spec 070, paquete alerts).

// NotificationKind es el tipo de un aviso. Se guarda como dato: no se renombra.
type NotificationKind string

const (
	KindStockLow NotificationKind = "stock_low" // llegó a su mínimo
	KindStockOut NotificationKind = "stock_out" // se acabó (lo detectó el sistema o lo reportó alguien)
	KindNotFound NotificationKind = "not_found" // alguien buscó algo que no está en el sistema
)

// NotificationKinds es la lista de tipos; la base la repite en su check (prueba de paridad).
func NotificationKinds() []NotificationKind {
	return []NotificationKind{KindStockLow, KindStockOut, KindNotFound}
}

// NotificationReason es por qué se cerró un aviso. Se guarda como dato: no se renombra.
type NotificationReason string

const (
	ReasonAboveMin    NotificationReason = "above_min"    // volvió sobre el mínimo
	ReasonRestocked   NotificationReason = "restocked"    // volvió a haber (cualquier movimiento)
	ReasonCounted     NotificationReason = "counted"      // un conteo cerrado lo incluyó
	ReasonArrived     NotificationReason = "arrived"      // alguien dijo «ya llegó»
	ReasonUndone      NotificationReason = "undone"       // se deshizo el «se acabó»
	ReasonAlertOff    NotificationReason = "alert_off"    // se apagó la alerta o el evento
	ReasonArchived    NotificationReason = "archived"     // el artículo se archivó
	ReasonMerged      NotificationReason = "merged"       // el insumo se unió a otro
	ReasonUnitChanged NotificationReason = "unit_changed" // el artículo cambió de unidad
	ReasonDismissed   NotificationReason = "dismissed"    // alguien lo descartó
	ReasonCreated     NotificationReason = "created"      // «No lo encuentro» terminó en un alta
	ReasonAlias       NotificationReason = "alias"        // «No lo encuentro» era otro nombre de algo que existe
)

// NotificationReasons es la lista de motivos; la base la repite en su check (prueba de paridad).
func NotificationReasons() []NotificationReason {
	return []NotificationReason{ReasonAboveMin, ReasonRestocked, ReasonCounted, ReasonArrived, ReasonUndone,
		ReasonAlertOff, ReasonArchived, ReasonMerged, ReasonUnitChanged, ReasonDismissed, ReasonCreated, ReasonAlias}
}

var (
	// ErrReportLimit: el tope de reportes por hora y persona. Frena a quien toca sin parar, no a la
	// operación: 20 «se acabó» en una hora ya no es un turno normal.
	ErrReportLimit = errors.New("llegaste al límite de 20 reportes por hora; pide a quien compra que revise los avisos")
	// ErrUndoExpired: el deshacer de «se acabó» dura 90 segundos.
	ErrUndoExpired = errors.New("ya pasaron los 90 segundos para deshacer")
	// ErrUndoAfterMovement: después del reporte el artículo se movió (una venta, una compra, un
	// conteo): reponer el delta original pisaría ese movimiento.
	ErrUndoAfterMovement = errors.New("ya no se puede deshacer: el artículo se movió después del reporte")
	// ErrClientKeyReused: la llave de un reporte ya se usó para otro artículo, otro texto u otra
	// persona. Devolver el reporte guardado haría que la pantalla anunciara lo que no pasó.
	ErrClientKeyReused = errors.New("ese aviso ya se mandó con otro artículo; vuelve a intentar")
	// ErrAlreadyReportedByYou: la persona ya reportó ese artículo y el aviso sigue abierto. Contarlo
	// como confirmación suya engañaba a quien compra y no se podía deshacer el reporte original.
	ErrAlreadyReportedByYou = errors.New("ya lo reportaste tú; quien compra ya tiene el aviso")
	// ErrNotificationClosed: el aviso ya se resolvió o se descartó.
	ErrNotificationClosed = errors.New("ese aviso ya está cerrado")
)

// LowStockConfig es lo que el artículo dice de sus alertas. Min nulo o cero = sin mínimo definido.
type LowStockConfig struct {
	Enabled bool
	OnMin   bool
	OnOut   bool
	Min     *decimal.Decimal
}

// HasMin dice si el artículo tiene un mínimo definido. Cero es «sin mínimo» (decisión del dueño):
// avisar «llegó a 0 de mínimo 0» sería el mismo aviso que «se acabó» con otro nombre.
func (c LowStockConfig) HasMin() bool { return c.Min != nil && c.Min.IsPositive() }

// LowStockShould dice qué avisos del sistema deben estar abiertos con esa existencia. Es la tabla de
// verdad de evaluate_low_stock (la función de la base); la prueba de integración compara las dos.
func LowStockShould(c LowStockConfig, onHand decimal.Decimal) (low, out bool) {
	low = c.Enabled && c.OnMin && c.HasMin() && onHand.LessThanOrEqual(*c.Min)
	out = c.Enabled && c.OnOut && !onHand.IsPositive()
	return
}

// LowStockChange es lo que abre y cierra una evaluación. Nunca nil: viaja como JSON a la pantalla.
type LowStockChange struct {
	Opens  []NotificationKind `json:"opens"`
	Closes []NotificationKind `json:"closes"`
}

// LowStockEffect calcula qué se abre y qué se cierra con la existencia de hoy, dado lo que ya está
// abierto. manualOut dice si el «se acabó» abierto lo reportó una persona: ése no se cierra por
// apagar la alerta, solo cuando vuelve a haber.
func LowStockEffect(c LowStockConfig, onHand decimal.Decimal, open []NotificationKind, manualOut bool) LowStockChange {
	low, out := LowStockShould(c, onHand)
	isOpen := map[NotificationKind]bool{}
	for _, k := range open {
		isOpen[k] = true
	}
	e := LowStockChange{Opens: []NotificationKind{}, Closes: []NotificationKind{}}
	if low && !isOpen[KindStockLow] {
		e.Opens = append(e.Opens, KindStockLow)
	}
	if !low && isOpen[KindStockLow] {
		e.Closes = append(e.Closes, KindStockLow)
	}
	if out && !isOpen[KindStockOut] {
		e.Opens = append(e.Opens, KindStockOut)
	}
	keepManual := manualOut && !onHand.IsPositive()
	if !out && isOpen[KindStockOut] && !keepManual {
		e.Closes = append(e.Closes, KindStockOut)
	}
	return e
}

// countStaleAfter es cuánto vale un conteo para creerle a la existencia.
const countStaleAfter = 30 * 24 * time.Hour

// NeedsCount dice si un aviso debe decir «pedir conteo» en vez de «agotado»: con la existencia en
// negativo, o sin un conteo en los últimos 30 días, la cifra del sistema no es para comprar.
func NeedsCount(onHand decimal.Decimal, lastCounted *time.Time, now time.Time) bool {
	if onHand.IsNegative() || lastCounted == nil {
		return true
	}
	return now.Sub(*lastCounted) > countStaleAfter
}

// emailWindow es el bloque que junta los avisos en un solo correo (decisión del dueño).
const emailWindow = 5 * time.Minute

// EmailWindowStart es el inicio del bloque de 5 minutos de un aviso, en UTC: bloques fijos y no una
// ventana por aviso, para que nunca salgan dos correos de una empresa en el mismo bloque.
func EmailWindowStart(at time.Time) time.Time { return at.UTC().Truncate(emailWindow) }

// EmailWindowClosed dice si ya se puede mandar el correo del bloque.
func EmailWindowClosed(start, now time.Time) bool { return !now.Before(start.Add(emailWindow)) }

// EmailWindowKey es la llave del bloque en la tabla de envíos.
func EmailWindowKey(start time.Time) string { return start.UTC().Format("2006-01-02T15:04Z") }

// emailCooldown: el mismo artículo no vuelve a mandar correo en 12 h aunque su aviso se cierre y se
// reabra (un insumo que oscila alrededor del mínimo llenaría el buzón).
const emailCooldown = 12 * time.Hour

// InEmailCooldown dice si un artículo cuyo último correo salió en last todavía no puede mandar otro.
func InEmailCooldown(last *time.Time, now time.Time) bool {
	return last != nil && now.Sub(*last) < emailCooldown
}

var morningTime = regexp.MustCompile(`^([01][0-9]|2[0-3]):(00|15|30|45)$`)

// ValidMorningTime acepta la hora del resumen de la mañana en pasos de 15 minutos («08:00»). Una hora
// mal escrita se rechaza: caer a las 08:00 en silencio mandaría el resumen a una hora que nadie pidió.
func ValidMorningTime(s string) error {
	if !morningTime.MatchString(s) {
		return fmt.Errorf("%w: la hora del resumen va en pasos de 15 minutos, como 08:00", ErrValidation)
	}
	return nil
}

// MorningSummaryDue dice la fecha del negocio de hoy y si ya es hora del resumen. Que ya haya salido
// hoy lo decide la tabla de envíos, no esto: así un reinicio no lo duplica.
func MorningSummaryDue(now time.Time, loc *time.Location, at string) (string, bool, error) {
	if err := ValidMorningTime(at); err != nil {
		return "", false, err
	}
	local := now.In(loc)
	var h, m int
	if _, err := fmt.Sscanf(at, "%d:%d", &h, &m); err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	due := local.Hour()*60+local.Minute() >= h*60+m
	return local.Format("2006-01-02"), due, nil
}

// SearchKeys parte lo que se escribió en «Se acabó» en llaves de búsqueda: minúsculas, sin acentos y
// en singular (quesos → queso, platos → plato, bolsas grandes → bolsa grand), para buscar por inicio de
// palabra. Nunca nil.
func SearchKeys(q string) []string {
	out := []string{}
	for _, w := range strings.Fields(CatalogNameKey(q)) {
		switch {
		case len(w) > 4 && strings.HasSuffix(w, "es"):
			w = w[:len(w)-2]
		case len(w) > 3 && strings.HasSuffix(w, "s"):
			w = w[:len(w)-1]
		}
		out = append(out, w)
	}
	return out
}

// CleanNotFoundText deja el texto de «No lo encuentro» como se guarda.
func CleanNotFoundText(s string) (string, error) {
	t := strings.Join(strings.Fields(s), " ")
	if t == "" {
		return "", fmt.Errorf("%w: escribe qué se acabó", ErrValidation)
	}
	if utf8.RuneCountInString(t) > 80 {
		return "", fmt.Errorf("%w: el texto va hasta 80 caracteres", ErrValidation)
	}
	return t, nil
}

// reportsPerHour es el tope de reportes («se acabó» y «no lo encuentro») por persona y hora.
const reportsPerHour = 20

// ReportLimitReached dice si quien ya hizo n reportes en la última hora debe detenerse.
func ReportLimitReached(n int) bool { return n >= reportsPerHour }

// outReportUndo es cuánto dura el deshacer de «se acabó».
const outReportUndo = 90 * time.Second

// OutReportUndoWindow es la duración del deshacer, para la pantalla.
func OutReportUndoWindow() time.Duration { return outReportUndo }

// CanUndoOutReport dice si un reporte hecho en at se puede deshacer en now (los dos de la base).
func CanUndoOutReport(at, now time.Time) bool { return !now.After(at.Add(outReportUndo)) }
