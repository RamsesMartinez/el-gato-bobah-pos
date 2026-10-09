package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// DraftsService es la cuenta en captura (spec 030): lo que se está capturando, guardado en el
// servidor desde el primer producto y visible en todas las tabletas. Nunca es un pedido (D-1); se
// convierte en uno al mandarse a cocina (Send).
type DraftsService struct {
	store  *store.Store
	orders *OrdersService
}

func NewDraftsService(s *store.Store, orders *OrdersService) *DraftsService {
	return &DraftsService{store: s, orders: orders}
}

// Field es un campo de un PATCH: ausente (Set=false) no cambia; presente con Value nil lo borra.
type Field[T any] struct {
	Set   bool
	Value *T
}

// DraftDiscount es el descuento tal como se capturó: monto o porcentaje, excluyentes.
type DraftDiscount struct {
	Amount  *decimal.Decimal `json:"amount,omitempty"`
	Percent *decimal.Decimal `json:"percent,omitempty"`
}

// DraftHeaderPatch es la cabecera de una cuenta: lo que se crea con ella o lo que se cambia después.
type DraftHeaderPatch struct {
	ExpectedVersion  int32
	ServiceType      *string
	CustomerName     Field[string]
	PlatformID       Field[int16]
	PlatformOrderRef Field[string]
	DeliveryFee      *decimal.Decimal
	Discount         Field[DraftDiscount]
}

// DraftLineCmd es un «agregar»: un producto nuevo, o el «+» de un renglón (IntoLineID).
type DraftLineCmd struct {
	OpID       uuid.UUID
	IntoLineID *uuid.UUID
	ProductID  int64
	Qty        decimal.Decimal
	Modifiers  []domain.DraftModifier
	Notes      string
}

type CreateDraftCmd struct {
	ID        uuid.UUID
	OrderID   *int64
	FolioName string
	Header    *DraftHeaderPatch
	Lines     []DraftLineCmd
	Actor     int64
}

type ChangeDraftLineCmd struct {
	DraftID         uuid.UUID
	LineID          uuid.UUID
	ExpectedVersion int32
	Qty             *decimal.Decimal
	Modifiers       *[]domain.DraftModifier
	Notes           *string
}

// DraftView es la cuenta como la pinta la tableta, con precios calculados en cada lectura.
type DraftView struct {
	ID               uuid.UUID       `json:"id"`
	OrderID          *int64          `json:"orderId"`
	FolioName        *string         `json:"folioName"`
	Status           string          `json:"status"`
	HeaderVersion    int32           `json:"headerVersion"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	CreatedAt        time.Time       `json:"createdAt"`
	OpenedBy         string          `json:"openedBy"`
	ServiceType      string          `json:"serviceType"`
	CustomerName     *string         `json:"customerName"`
	PlatformID       *int16          `json:"platformId"`
	PlatformOrderRef *string         `json:"platformOrderRef"`
	DeliveryFee      decimal.Decimal `json:"deliveryFee"`
	Discount         *DraftDiscount  `json:"discount"`
	Lines            []DraftLineView `json:"lines"`
	Subtotal         decimal.Decimal `json:"subtotal"`
	DiscountTotal    decimal.Decimal `json:"discountTotal"`
	Total            decimal.Decimal `json:"total"`
	Unavailable      []string        `json:"unavailable"`
}

type DraftLineView struct {
	ID          uuid.UUID       `json:"id"`
	Version     int32           `json:"version"`
	ProductID   int64           `json:"productId"`
	ProductName string          `json:"productName"`
	Qty         decimal.Decimal `json:"qty"`
	UnitPrice   decimal.Decimal `json:"unitPrice"`
	Modifiers   []DraftModView  `json:"modifiers"`
	Notes       string          `json:"notes"`
	LineTotal   decimal.Decimal `json:"lineTotal"`
	Available   bool            `json:"available"`
}

type DraftModView struct {
	OptionID   int64           `json:"optionId"`
	Name       string          `json:"name"`
	Qty        int             `json:"qty"`
	PriceDelta decimal.Decimal `json:"priceDelta"`
	Portion    string          `json:"portion"`
}

// PatchDraftResult dice si cambió el descuento, para el evento de seguridad.
type PatchDraftResult struct {
	View            *DraftView
	DiscountChanged bool
	DiscountBefore  string
	DiscountAfter   string
	// RefChanged y RefBefore: el folio de plataforma cambió, y cuál era. Viajan para el evento de
	// seguridad, como en `PATCH /orders/{id}/platform-ref`: el cambio es en sitio y sin historia.
	RefChanged bool
	RefBefore  string
}

// draftCreateRetries: cuántas veces se reintenta crear cuando otra tableta se llevó el nombre (o abrió
// la «Nuevo» del mismo pedido) en el mismo instante. Cada vuelta relee los nombres vivos, así que la
// segunda ya no propone el perdido.
const draftCreateRetries = 3

// maxCustomerName es el largo del cliente en letras; el check de la tabla dice lo mismo.
const maxCustomerName = 60

// Create abre una cuenta con su primer producto, o le agrega a la «Nuevo» de un pedido ya enviado.
//
// Devuelve `created=false` en dos casos que NO son error: el reintento con el mismo id (no reaplica
// los renglones: sus opId ya existen), y una «Nuevo» que otra tableta ya había abierto para ese
// pedido (se le aplican los renglones a ésa y se devuelve con SU id).
func (s *DraftsService) Create(ctx context.Context, cmd CreateDraftCmd) (*DraftView, bool, error) {
	if cmd.ID == uuid.Nil() || len(cmd.Lines) == 0 {
		return nil, false, fmt.Errorf("%w: la cuenta nace con su primer producto", domain.ErrValidation)
	}
	if len(cmd.Lines) > domain.MaxDraftLines {
		return nil, false, tooManyLines()
	}
	if cmd.OrderID != nil && cmd.Header != nil {
		return nil, false, fmt.Errorf("%w: lo nuevo de un pedido no lleva datos propios", domain.ErrValidation)
	}
	for i := range cmd.Lines {
		if err := validLineCmd(&cmd.Lines[i]); err != nil {
			return nil, false, err
		}
		if cmd.Lines[i].IntoLineID != nil {
			return nil, false, fmt.Errorf("%w: una cuenta nueva no tiene renglones a los cuales sumar", domain.ErrValidation)
		}
	}
	// El barrido va en SU transacción, antes: dentro de la de crear tomaría cuentas y luego, al abrir
	// una «Nuevo», el pedido — el orden inverso al de enviar, y los dos se interbloqueaban.
	if err := s.store.WithTx(ctx, func(q *db.Queries) error { return sweepDrafts(ctx, q) }); err != nil {
		return nil, false, err
	}
	var id uuid.UUID
	var created bool
	var err error
	for range draftCreateRetries {
		err = s.store.WithTx(ctx, func(q *db.Queries) error {
			if d, err := q.GetDraft(ctx, cmd.ID); err == nil {
				// Reintento: la misma cuenta, sin volver a aplicar nada.
				if err := draftStatusErr(d.Status); err != nil {
					return err
				}
				id, created = d.ID, false
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if cmd.OrderID != nil {
				id, created, err = s.createNewOfOrder(ctx, q, cmd)
			} else {
				id, created, err = s.createAccount(ctx, q, cmd)
			}
			return err
		})
		if !isDraftRace(err) {
			break
		}
	}
	if isDraftRace(err) {
		return nil, false, fmt.Errorf("%w: otra tableta abrió una cuenta al mismo tiempo; vuelve a intentarlo", domain.ErrConflict)
	}
	if err != nil {
		return nil, false, err
	}
	v, err := draftView(ctx, s.store.QC(ctx), id)
	return v, created, err
}

// isDraftRace dice si un 23505 es la carrera de dos tabletas por el mismo nombre o la misma «Nuevo»,
// que se resuelve reintentando. Cualquier otro choque no es una carrera y no se reintenta.
func isDraftRace(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		(pgErr.ConstraintName == "order_drafts_live_name" || pgErr.ConstraintName == "order_drafts_live_per_order")
}

func tooManyLines() error {
	return fmt.Errorf("%w: una cuenta lleva a lo más %d productos distintos", domain.ErrValidation, domain.MaxDraftLines)
}

// createAccount abre una cuenta nueva: amarra su nombre (D-2) y le aplica los renglones.
func (s *DraftsService) createAccount(ctx context.Context, q *db.Queries, cmd CreateDraftCmd) (uuid.UUID, bool, error) {
	h, err := newHeader(ctx, q, cmd.Header, cmd.Actor)
	if err != nil {
		return uuid.UUID{}, false, err
	}
	nombre, esquema, err := bindDraftName(ctx, q, cmd.FolioName)
	if err != nil {
		return uuid.UUID{}, false, err
	}
	n, err := q.InsertDraft(ctx, db.InsertDraftParams{
		ID: cmd.ID, FolioName: &nombre, FolioScheme: &esquema,
		ServiceType: db.ServiceType(h.serviceType), CustomerName: h.customerName,
		DeliveryPlatformID: h.platformID, PlatformOrderRef: h.platformRef, DeliveryFee: h.deliveryFee,
		DiscountAmount: h.discountAmount, DiscountPercent: h.discountPercent,
		DiscountSetBy: h.discountSetBy, PlatformRefSetBy: h.platformRefSetBy, OpenedBy: cmd.Actor,
	})
	if err != nil {
		return uuid.UUID{}, false, err
	}
	if n == 0 {
		// El id ya existe y no se ve: es de otra empresa (RLS). No es un reintento nuestro.
		return uuid.UUID{}, false, fmt.Errorf("%w: esa cuenta no se puede crear", domain.ErrConflict)
	}
	for _, l := range cmd.Lines {
		if err := addDraftLineInTx(ctx, q, cmd.ID, l); err != nil {
			return uuid.UUID{}, false, err
		}
	}
	if h.discountAmount != nil || h.discountPercent != nil {
		if err := checkDraftDiscount(ctx, q, cmd.ID); err != nil {
			return uuid.UUID{}, false, err
		}
	}
	return cmd.ID, true, nil
}

// createNewOfOrder abre (o reusa) la «Nuevo» de un pedido ya enviado (D-3).
func (s *DraftsService) createNewOfOrder(ctx context.Context, q *db.Queries, cmd CreateDraftCmd) (uuid.UUID, bool, error) {
	o, err := q.GetOrderForUpdate(ctx, *cmd.OrderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, false, fmt.Errorf("%w: ese pedido no existe", domain.ErrNotFound)
		}
		return uuid.UUID{}, false, err
	}
	pagado, err := q.SumOrderPayments(ctx, o.ID)
	if err != nil {
		return uuid.UUID{}, false, err
	}
	if err := domain.CanReceiveLines(domain.OrderForAdd{
		Status: string(o.Status), Paid: pagado.Pagado, Total: o.Total, PlatformID: o.DeliveryPlatformID,
	}); err != nil {
		return uuid.UUID{}, false, err
	}
	// Otra tableta ya abrió la «Nuevo» de este pedido: lo de ésta se le suma a aquélla (R-9).
	if existing, err := q.GetLiveDraftOfOrder(ctx, &o.ID); err == nil {
		// Se bloquea y se REVISA viva: otra tableta pudo descartarla o mandarla entre la lectura y el
		// candado, y agregarle a una cuenta muerta perdería el producto respondiendo 200. Si murió, se
		// abre una «Nuevo» nueva (el único parcial ya no la cuenta).
		if _, err := lockLiveDraft(ctx, q, existing); err == nil {
			for _, l := range cmd.Lines {
				if err := addDraftLineInTx(ctx, q, existing, l); err != nil {
					return uuid.UUID{}, false, err
				}
			}
			return existing, false, nil
		} else if !errors.Is(err, domain.ErrDraftDiscarded) && !errors.Is(err, domain.ErrDraftAlreadySent) {
			return uuid.UUID{}, false, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, false, err
	}
	n, err := q.InsertDraft(ctx, db.InsertDraftParams{
		ID: cmd.ID, OrderID: &o.ID, ServiceType: o.ServiceType, DeliveryFee: decimal.Zero, OpenedBy: cmd.Actor,
	})
	if err != nil {
		return uuid.UUID{}, false, err
	}
	if n == 0 {
		return uuid.UUID{}, false, fmt.Errorf("%w: esa cuenta no se puede crear", domain.ErrConflict)
	}
	for _, l := range cmd.Lines {
		if err := addDraftLineInTx(ctx, q, cmd.ID, l); err != nil {
			return uuid.UUID{}, false, err
		}
	}
	return cmd.ID, true, nil
}

// bindDraftName saca el nombre de la bolsa para una cuenta que nace (research R-3).
//
// Es el MISMO predicado que la pantalla y que el pedido que se crea por otro camino
// (domain.AvailableNames): lo usado en el turno abierto, si hay, más los nombres de las cuentas
// vivas, leídos aquí, dentro de la transacción. Gana el propuesto por la pantalla si está libre.
// Si otra tableta se lo lleva en el mismo instante, el único parcial de la tabla lo rechaza con
// 23505 y Create reintenta todo con los vivos ya actualizados.
func bindDraftName(ctx context.Context, q *db.Queries, propuesto string) (string, db.FolioScheme, error) {
	var usados []string
	if sess, err := q.GetOpenPrimarySession(ctx); err == nil {
		if usados, err = folioNamesUsedInSession(ctx, q, sess.ID); err != nil {
			return "", "", err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	esquema, err := esquemaDeFolio(ctx, q)
	if err != nil {
		return "", "", err
	}
	consumidos, err := q.FolioNamesConsumidos(ctx, db.FolioScheme(esquema))
	if err != nil {
		return "", "", err
	}
	vivos, err := q.ListLiveDraftNames(ctx)
	if err != nil {
		return "", "", err
	}
	lista := domain.NombresDelEsquema(esquema)
	opciones, vaciar := domain.AvailableNames(lista, consumidos, usados, vivos)
	if len(opciones) == 0 {
		// Todos los nombres están en cuentas vivas. Responder «no hay nombres» dejaría a TODAS las
		// tabletas sin poder vender hasta que alguien descarte (o pasen 12 horas), y bastan unas
		// decenas de cuentas para provocarlo. Un nombre con número es mejor que no vender; no se marca
		// en la bolsa porque no es de la bolsa.
		base := lista[rand.IntN(len(lista))] //nolint:gosec // G404: el nombre de un pedido se canta en voz alta; no es secreto
		nombre := domain.SiguienteFolioLibre(base, append(append([]string(nil), vivos...), usados...))
		if nombre == "" {
			return "", "", fmt.Errorf("%w: no quedan nombres libres; cierra o manda alguna cuenta", domain.ErrConflict)
		}
		return nombre, db.FolioScheme(esquema), nil
	}
	nombre := opciones[rand.IntN(len(opciones))] //nolint:gosec // G404: el nombre de un pedido se canta en voz alta; no es secreto
	if base := domain.SanitizarFolio(propuesto); base != "" && contiene(opciones, base) {
		nombre = base
	}
	if vaciar {
		if err := q.VaciarBolsaDeFolios(ctx, db.FolioScheme(esquema)); err != nil {
			return "", "", err
		}
	}
	if err := q.MarcarFolioConsumido(ctx, db.MarcarFolioConsumidoParams{Scheme: db.FolioScheme(esquema), Name: nombre}); err != nil {
		return "", "", err
	}
	// Pasado el largo de la lista en el turno, la bolsa ofrece animales ya cantados hoy. La cuenta
	// nace ya con su número de vuelta («Persa 2») y no al mandarla: el nombre de la ficha y del
	// ticket es el que se le dice al cliente, y el pedido tiene que llevar ese mismo (D-2).
	numerado := domain.SiguienteFolioLibre(nombre, append(append([]string(nil), usados...), vivos...))
	if numerado == "" {
		return "", "", fmt.Errorf("%w: se acabaron los nombres del día", domain.ErrConflict)
	}
	return numerado, db.FolioScheme(esquema), nil
}

// Get devuelve la cuenta, también si ya se envió o se descartó: la pantalla necesita saber que otra
// tableta la mandó a cocina o la cerró.
func (s *DraftsService) Get(ctx context.Context, id uuid.UUID) (*DraftView, error) {
	return draftView(ctx, s.store.QC(ctx), id)
}

// AddLine agrega a una cuenta viva: un producto, o el «+» de un renglón. Idempotente por OpID.
func (s *DraftsService) AddLine(ctx context.Context, draftID uuid.UUID, cmd DraftLineCmd) (*DraftView, error) {
	if err := validLineCmd(&cmd); err != nil {
		return nil, err
	}
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := lockLiveDraft(ctx, q, draftID); err != nil {
			return err
		}
		return addDraftLineInTx(ctx, q, draftID, cmd)
	})
	if err != nil {
		return nil, err
	}
	return draftView(ctx, s.store.QC(ctx), draftID)
}

// ChangeLine cambia cantidad, modificadores o nota de un renglón, con la versión que la tableta vio.
func (s *DraftsService) ChangeLine(ctx context.Context, cmd ChangeDraftLineCmd) (*DraftView, error) {
	if cmd.Qty == nil && cmd.Modifiers == nil && cmd.Notes == nil {
		return nil, fmt.Errorf("%w: no hay nada que cambiar", domain.ErrValidation)
	}
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := lockLiveDraft(ctx, q, cmd.DraftID); err != nil {
			return err
		}
		lines, err := q.ListDraftLines(ctx, cmd.DraftID)
		if err != nil {
			return err
		}
		var cur *db.OrderDraftLine
		for i := range lines {
			if lines[i].ID == cmd.LineID {
				cur = &lines[i]
			}
		}
		if cur == nil {
			// La otra tableta lo quitó: lo mismo que una versión vieja, la pantalla recarga.
			return domain.ErrDraftChanged
		}
		in := domain.DraftLineInput{ProductID: cur.ProductID, Qty: cur.Qty, Modifiers: modsOf(cur.Modifiers), Notes: derefStr(cur.Notes)}
		if cmd.Qty != nil {
			in.Qty = *cmd.Qty
		}
		if cmd.Modifiers != nil {
			in.Modifiers = *cmd.Modifiers
		}
		if cmd.Notes != nil {
			in.Notes = strings.TrimSpace(*cmd.Notes)
		}
		in, err = domain.ValidateDraftLine(in)
		if err != nil {
			return err
		}
		if cmd.Modifiers != nil {
			if err := checkOptions(ctx, q, in.Modifiers); err != nil {
				return err
			}
		}
		n, err := q.ChangeDraftLine(ctx, db.ChangeDraftLineParams{
			Qty: in.Qty, Modifiers: modsJSON(in.Modifiers), Notes: notesPtr(in.Notes),
			ID: cmd.LineID, DraftID: cmd.DraftID, ExpectedVersion: cmd.ExpectedVersion,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrDraftChanged
		}
		return q.TouchDraft(ctx, cmd.DraftID)
	})
	if err != nil {
		return nil, err
	}
	return draftView(ctx, s.store.QC(ctx), cmd.DraftID)
}

// RemoveLine quita un renglón con la versión que la tableta vio. Quitar el último deja la cuenta
// vacía y viva, con su nombre: cerrarla es otra decisión (D-7).
func (s *DraftsService) RemoveLine(ctx context.Context, draftID, lineID uuid.UUID, expectedVersion int32) (*DraftView, error) {
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := lockLiveDraft(ctx, q, draftID); err != nil {
			return err
		}
		n, err := q.DeleteDraftLine(ctx, db.DeleteDraftLineParams{ID: lineID, DraftID: draftID, ExpectedVersion: expectedVersion})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrDraftChanged
		}
		return q.TouchDraft(ctx, draftID)
	})
	if err != nil {
		return nil, err
	}
	return draftView(ctx, s.store.QC(ctx), draftID)
}

// PatchHeader cambia la cabecera con la versión que la tableta vio. Los campos ausentes no cambian.
//
// Quien pone el descuento o teclea el folio de plataforma queda como su autor —no quien abrió la
// cuenta— y pasa así al pedido: es el mismo rastro que exige `PUT /orders/{id}/discount`, y un camino
// nuevo para poner un descuento no nace sin él.
func (s *DraftsService) PatchHeader(ctx context.Context, id uuid.UUID, p DraftHeaderPatch, actor int64) (PatchDraftResult, error) {
	var res PatchDraftResult
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		d, err := lockLiveDraft(ctx, q, id)
		if err != nil {
			return err
		}
		if d.FolioName == nil {
			return domain.ErrDraftHasOrderHeader
		}
		if d.HeaderVersion != p.ExpectedVersion {
			return domain.ErrDraftChanged
		}
		before := headerOf(d)
		h, err := applyHeader(ctx, q, before, p, actor)
		if err != nil {
			return err
		}
		n, err := q.UpdateDraftHeader(ctx, db.UpdateDraftHeaderParams{
			ServiceType: db.ServiceType(h.serviceType), CustomerName: h.customerName,
			DeliveryPlatformID: h.platformID, PlatformOrderRef: h.platformRef, DeliveryFee: h.deliveryFee,
			DiscountAmount: h.discountAmount, DiscountPercent: h.discountPercent,
			DiscountSetBy: h.discountSetBy, PlatformRefSetBy: h.platformRefSetBy,
			ID: id, ExpectedVersion: p.ExpectedVersion,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrDraftChanged
		}
		if !sameStr(before.platformRef, h.platformRef) {
			res.RefChanged, res.RefBefore = true, derefStr(before.platformRef)
		}
		if p.Discount.Set {
			if err := checkDraftDiscount(ctx, q, id); err != nil {
				return err
			}
			res.DiscountBefore, res.DiscountAfter = before.discountText(), h.discountText()
			res.DiscountChanged = res.DiscountBefore != res.DiscountAfter
		}
		return nil
	})
	if err != nil {
		return PatchDraftResult{}, err
	}
	res.View, err = draftView(ctx, s.store.QC(ctx), id)
	return res, err
}

// --- piezas compartidas -------------------------------------------------------------------------

// draftStatusErr traduce un estado terminal al rechazo que la pantalla sabe leer.
func draftStatusErr(status string) error {
	switch status {
	case domain.DraftSent:
		return domain.ErrDraftAlreadySent
	case domain.DraftDiscarded:
		return domain.ErrDraftDiscarded
	}
	return nil
}

// lockLiveDraft bloquea la cuenta y exige que siga capturándose. Toda escritura pasa por aquí
// PRIMERO: serializa por cuenta, así dos tabletas no fusionan ni numeran renglones sobre el mismo
// estado viejo.
func lockLiveDraft(ctx context.Context, q *db.Queries, id uuid.UUID) (db.OrderDraft, error) {
	d, err := q.LockDraft(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return d, fmt.Errorf("%w: esa cuenta ya no existe", domain.ErrNotFound)
		}
		return d, err
	}
	return d, draftStatusErr(d.Status)
}

// validLineCmd valida un «agregar» en la frontera y deja la cantidad redondeada.
func validLineCmd(c *DraftLineCmd) error {
	if c.OpID == uuid.Nil() {
		return fmt.Errorf("%w: falta la llave del toque", domain.ErrValidation)
	}
	if c.IntoLineID != nil {
		qty, err := domain.ValidateDraftQty(c.Qty)
		if err != nil {
			return err
		}
		c.Qty = qty
		return nil
	}
	c.Notes = strings.TrimSpace(c.Notes)
	in, err := domain.ValidateDraftLine(domain.DraftLineInput{ProductID: c.ProductID, Qty: c.Qty, Modifiers: c.Modifiers, Notes: c.Notes})
	if err != nil {
		return err
	}
	c.Qty = in.Qty
	return nil
}

// addDraftLineInTx aplica un «agregar» a una cuenta YA BLOQUEADA por quien llama.
//
// La idempotencia es por toque (OpID), no por cuenta: agregar se suma (D-5), así que un reintento con
// la misma llave no hace nada y uno con llave nueva siempre suma.
func addDraftLineInTx(ctx context.Context, q *db.Queries, draftID uuid.UUID, c DraftLineCmd) error {
	if owner, err := q.GetDraftAdd(ctx, c.OpID); err == nil {
		if owner != draftID {
			// Misma llave, OTRA cuenta: no es un reintento, es un toque mal dirigido. Aplicarlo le
			// cargaría el producto a la mesa equivocada.
			return fmt.Errorf("%w: ese producto ya se agregó a otra cuenta", domain.ErrConflict)
		}
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var lineID uuid.UUID
	switch {
	case c.IntoLineID != nil:
		rows, err := q.ListDraftLines(ctx, draftID)
		if err != nil {
			return err
		}
		cur, ok := findLine(rows, *c.IntoLineID)
		if !ok {
			return fmt.Errorf("%w: ese producto ya no está en la cuenta", domain.ErrNotFound)
		}
		if err := checkSum(cur.Qty, c.Qty); err != nil {
			return err
		}
		if _, err := q.AddToDraftLine(ctx, db.AddToDraftLineParams{Qty: c.Qty, ID: *c.IntoLineID, DraftID: draftID}); err != nil {
			return err
		}
		lineID = *c.IntoLineID
	default:
		if err := checkProduct(ctx, q, c.ProductID); err != nil {
			return err
		}
		if err := checkOptions(ctx, q, c.Modifiers); err != nil {
			return err
		}
		rows, err := q.ListDraftLines(ctx, draftID)
		if err != nil {
			return err
		}
		lines := make([]domain.DraftLine, len(rows))
		for i, r := range rows {
			lines[i] = domain.DraftLine{ID: r.ID, ProductID: r.ProductID, Qty: r.Qty, Modifiers: modsOf(r.Modifiers), Notes: derefStr(r.Notes)}
		}
		in := domain.DraftLineInput{ProductID: c.ProductID, Qty: c.Qty, Modifiers: c.Modifiers, Notes: c.Notes}
		if target, ok := domain.MergeTarget(lines, in); ok {
			cur, _ := findLine(rows, target)
			if err := checkSum(cur.Qty, c.Qty); err != nil {
				return err
			}
			if _, err := q.AddToDraftLine(ctx, db.AddToDraftLineParams{Qty: c.Qty, ID: target, DraftID: draftID}); err != nil {
				return err
			}
			lineID = target
			break
		}
		if len(rows) >= domain.MaxDraftLines {
			return fmt.Errorf("%w: la cuenta ya tiene %d productos distintos", domain.ErrValidation, domain.MaxDraftLines)
		}
		if err := q.InsertDraftLine(ctx, db.InsertDraftLineParams{
			ID: c.OpID, DraftID: draftID, ProductID: c.ProductID, Qty: c.Qty,
			Modifiers: modsJSON(c.Modifiers), Notes: notesPtr(c.Notes),
		}); err != nil {
			return err
		}
		lineID = c.OpID
	}
	if n, err := q.InsertDraftAdd(ctx, db.InsertDraftAddParams{OpID: c.OpID, DraftID: draftID, LineID: lineID}); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("%w: ese producto ya se agregó a otra cuenta", domain.ErrConflict)
	}
	return q.TouchDraft(ctx, draftID)
}

func findLine(rows []db.OrderDraftLine, id uuid.UUID) (db.OrderDraftLine, bool) {
	for _, r := range rows {
		if r.ID == id {
			return r, true
		}
	}
	return db.OrderDraftLine{}, false
}

// checkSum topa la cantidad ACUMULADA de un renglón, no solo el toque: cien «+» de 10 000 desbordan
// la columna (500) y, antes, guardan una cantidad que el pedido rechazaría al enviar.
func checkSum(cur, add decimal.Decimal) error {
	if !domain.ValidQty(cur.Add(add), domain.MaxOrderQty, false) {
		return fmt.Errorf("%w: ese producto ya llegó al máximo de piezas", domain.ErrValidation)
	}
	return nil
}

// checkProduct rechaza el producto que no está en el menú de ESTA empresa (bajo RLS: el de otra
// no se ve) o que ya no se vende. Sin esto el renglón entraría y la cuenta se descubriría
// imposible de enviar hasta el final.
func checkProduct(ctx context.Context, q *db.Queries, productID int64) error {
	rows, err := q.GetPricedProducts(ctx, []int64{productID})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return domain.ProductUnavailable{ProductID: productID}
	}
	if !rows[0].IsActive {
		return domain.ProductUnavailable{ProductID: productID, Name: rows[0].Name}
	}
	return nil
}

func checkOptions(ctx context.Context, q *db.Queries, mods []domain.DraftModifier) error {
	if len(mods) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(mods))
	for _, m := range mods {
		ids = append(ids, m.OptionID)
	}
	rows, err := q.GetPricedOptions(ctx, ids)
	if err != nil {
		return err
	}
	found := make(map[int64]bool, len(rows))
	for _, r := range rows {
		found[r.ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			return fmt.Errorf("%w (id %d)", domain.ErrOptionNotFound, id)
		}
	}
	return nil
}

// checkDraftDiscount valida el descuento de la cuenta contra el subtotal que calcula el servidor,
// dentro de la transacción que lo escribió.
func checkDraftDiscount(ctx context.Context, q *db.Queries, id uuid.UUID) error {
	d, err := q.GetDraft(ctx, id)
	if err != nil {
		return err
	}
	v, err := buildDraftView(ctx, q, d)
	if err != nil {
		return err
	}
	_, err = domain.ResolverDescuento(v.Subtotal, d.DiscountAmount, d.DiscountPercent)
	return err
}

func modsOf(raw []byte) []domain.DraftModifier {
	var out []domain.DraftModifier
	_ = json.Unmarshal(raw, &out) // el check de la tabla garantiza un arreglo; lo validó ValidateDraftLine
	return out
}

func modsJSON(mods []domain.DraftModifier) []byte {
	if len(mods) == 0 {
		return []byte("[]")
	}
	b, _ := json.Marshal(mods)
	return b
}

func notesPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- cabecera -------------------------------------------------------------------------------------

// draftHeader es la cabecera ya resuelta: lo que se escribe tal cual.
type draftHeader struct {
	serviceType      string
	customerName     *string
	platformID       *int16
	platformRef      *string
	deliveryFee      decimal.Decimal
	discountAmount   *decimal.Decimal
	discountPercent  *decimal.Decimal
	discountSetBy    *int64
	platformRefSetBy *int64
}

func headerOf(d db.OrderDraft) draftHeader {
	return draftHeader{
		serviceType: string(d.ServiceType), customerName: d.CustomerName, platformID: d.DeliveryPlatformID,
		platformRef: d.PlatformOrderRef, deliveryFee: d.DeliveryFee, discountAmount: d.DiscountAmount,
		discountPercent: d.DiscountPercent, discountSetBy: d.DiscountSetBy, platformRefSetBy: d.PlatformRefSetBy,
	}
}

func newHeader(ctx context.Context, q *db.Queries, p *DraftHeaderPatch, actor int64) (draftHeader, error) {
	h := draftHeader{serviceType: "mostrador", deliveryFee: decimal.Zero}
	if p == nil {
		return h, nil
	}
	return applyHeader(ctx, q, h, *p, actor)
}

// applyHeader aplica un PATCH sobre la cabecera actual y valida cada campo en la frontera.
func applyHeader(ctx context.Context, q *db.Queries, h draftHeader, p DraftHeaderPatch, actor int64) (draftHeader, error) {
	if p.ServiceType != nil {
		if !validServiceType(*p.ServiceType) {
			return h, fmt.Errorf("%w: ese tipo de servicio no existe", domain.ErrValidation)
		}
		h.serviceType = *p.ServiceType
	}
	if p.CustomerName.Set {
		h.customerName = nil
		if p.CustomerName.Value != nil {
			name := strings.TrimSpace(*p.CustomerName.Value)
			if utf8.RuneCountInString(name) > maxCustomerName {
				return h, fmt.Errorf("%w: el nombre del cliente es demasiado largo", domain.ErrValidation)
			}
			if name != "" {
				h.customerName = &name
			}
		}
	}
	if p.PlatformID.Set && !sameInt16(h.platformID, p.PlatformID.Value) {
		if p.PlatformID.Value != nil {
			// Bajo RLS y con rechazo explícito: la FK salta RLS, y caer a margen 0 cobraría a precio de
			// mostrador en la plataforma.
			if _, err := listaDePreciosQ(ctx, q, p.PlatformID.Value); err != nil {
				return h, err
			}
		}
		h.platformID = p.PlatformID.Value
		// Cambiar de plataforma tira el folio: era el de la otra.
		h.platformRef, h.platformRefSetBy = nil, nil
	}
	if p.PlatformOrderRef.Set {
		ref, err := domain.PlatformRefDelPedido(p.PlatformOrderRef.Value, h.platformID)
		if err != nil {
			return h, err
		}
		if !sameStr(ref, h.platformRef) {
			h.platformRef = ref
			h.platformRefSetBy = nil
			if ref != nil {
				h.platformRefSetBy = &actor
			}
		}
	}
	if p.DeliveryFee != nil {
		fee := domain.Round2(*p.DeliveryFee)
		if !domain.ValidMoney(fee, true) {
			return h, fmt.Errorf("%w: el envío no es un monto válido", domain.ErrValidation)
		}
		h.deliveryFee = fee
	}
	if p.Discount.Set {
		h.discountAmount, h.discountPercent, h.discountSetBy = nil, nil, nil
		if dd := p.Discount.Value; dd != nil {
			switch {
			case dd.Amount != nil && dd.Percent != nil:
				return h, fmt.Errorf("%w: el descuento viene como monto y como porcentaje a la vez", domain.ErrValidation)
			case dd.Amount != nil:
				a := domain.Round2(*dd.Amount)
				h.discountAmount = &a
			case dd.Percent != nil:
				p := domain.Round2(*dd.Percent)
				h.discountPercent = &p
			}
			if h.discountAmount != nil || h.discountPercent != nil {
				// La forma se valida aquí con un subtotal enorme; el tope real (no más que la venta) se
				// mide contra el subtotal del servidor después de escribir (checkDraftDiscount).
				if _, err := domain.ResolverDescuento(domain.MaxMoney, h.discountAmount, h.discountPercent); err != nil {
					return h, err
				}
				h.discountSetBy = &actor
			}
		}
	}
	return h, nil
}

// discountText describe el descuento para el evento de seguridad: dinero y quién, nunca PII.
func (h draftHeader) discountText() string {
	return discountText(h.discountAmount, h.discountPercent)
}

// DraftDiscountText describe el descuento con que nace una cuenta (crear o importar), con el mismo
// formato que el evento del PATCH.
func DraftDiscountText(d *DraftDiscount) string {
	if d == nil {
		return "0.00"
	}
	return discountText(d.Amount, d.Percent)
}

func discountText(amount, percent *decimal.Decimal) string {
	switch {
	case amount != nil:
		return domain.Round2(*amount).StringFixed(2)
	case percent != nil:
		return domain.Round2(*percent).StringFixed(2) + "%"
	}
	return "0.00"
}

func sameInt16(a, b *int16) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// --- vista ----------------------------------------------------------------------------------------

func draftView(ctx context.Context, q *db.Queries, id uuid.UUID) (*DraftView, error) {
	d, err := q.GetDraft(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: esa cuenta ya no existe", domain.ErrNotFound)
		}
		return nil, err
	}
	return buildDraftView(ctx, q, d)
}

// buildDraftView arma la cuenta con precios calculados AHORA, con la lista de la cuenta (mostrador o
// la plataforma). No se guardan precios (R-16): se reprecia en cada lectura y el pedido vuelve a
// calcular todo al nacer, así que un snapshot aquí no protegería nada.
//
// Un renglón que ya no se puede vender —producto desactivado, opción borrada— no tumba la lectura:
// sale con `available=false`, no suma, y su nombre va en `unavailable` para que la pantalla avise
// antes de mandar.
func buildDraftView(ctx context.Context, q *db.Queries, d db.GetDraftRow) (*DraftView, error) {
	rows, err := q.ListDraftLines(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	lista, err := listaDePreciosQ(ctx, q, d.DeliveryPlatformID)
	if err != nil {
		return nil, err
	}
	products, options, err := pricedCatalog(ctx, q, lista, rows)
	if err != nil {
		return nil, err
	}
	v := &DraftView{
		ID: d.ID, OrderID: d.OrderID, FolioName: d.FolioName, Status: d.Status, HeaderVersion: d.HeaderVersion,
		UpdatedAt: d.UpdatedAt, CreatedAt: d.CreatedAt, OpenedBy: d.OpenedByName, ServiceType: string(d.ServiceType),
		CustomerName: d.CustomerName, PlatformID: d.DeliveryPlatformID, PlatformOrderRef: d.PlatformOrderRef,
		DeliveryFee: d.DeliveryFee, Lines: make([]DraftLineView, 0, len(rows)), Unavailable: []string{},
	}
	if d.DiscountAmount != nil {
		v.Discount = &DraftDiscount{Amount: d.DiscountAmount}
	} else if d.DiscountPercent != nil {
		v.Discount = &DraftDiscount{Percent: d.DiscountPercent}
	}
	subtotal := decimal.Zero
	for _, r := range rows {
		lv := draftLineView(r, products, options)
		if lv.Available {
			subtotal = subtotal.Add(lv.LineTotal)
		} else if !contiene(v.Unavailable, lv.ProductName) {
			v.Unavailable = append(v.Unavailable, lv.ProductName)
		}
		v.Lines = append(v.Lines, lv)
	}
	v.Subtotal = domain.Round2(subtotal)
	descuento, err := domain.ResolverDescuento(v.Subtotal, d.DiscountAmount, d.DiscountPercent)
	if errors.Is(err, domain.ErrDescuentoMayorQueLaVenta) {
		// Se quitaron productos después de poner el descuento: la vista no se cae; enviar lo rechaza
		// con el máximo, que es donde el operador lo puede corregir.
		descuento = v.Subtotal
	} else if err != nil {
		return nil, err
	}
	v.DiscountTotal = descuento
	v.Total = domain.Round2(v.Subtotal.Sub(descuento))
	if v.Total.IsNegative() {
		v.Total = decimal.Zero
	}
	if v.ServiceType == "domicilio" && v.PlatformID == nil {
		v.Total = domain.Round2(v.Total.Add(v.DeliveryFee))
	}
	return v, nil
}

func draftLineView(r db.OrderDraftLine, products map[int64]domain.PricedProduct, options map[int64]domain.PricedOption) DraftLineView {
	mods := modsOf(r.Modifiers)
	lv := DraftLineView{
		ID: r.ID, Version: r.Version, ProductID: r.ProductID, Qty: r.Qty, Notes: derefStr(r.Notes),
		Modifiers: make([]DraftModView, 0, len(mods)), LineTotal: decimal.Zero,
	}
	p, known := products[r.ProductID]
	if known {
		lv.ProductName, lv.UnitPrice = p.Name, p.Price
	} else {
		lv.ProductName = "Producto que ya no está en el menú"
	}
	for _, m := range mods {
		mv := DraftModView{OptionID: m.OptionID, Qty: m.Qty, Portion: m.Portion}
		if o, ok := options[m.OptionID]; ok {
			mv.Name, mv.PriceDelta = o.Name, o.PriceDelta
		}
		lv.Modifiers = append(lv.Modifiers, mv)
	}
	built, err := domain.BuildOrder([]domain.OrderLineInput{orderLineOf(r.ProductID, r.Qty, mods, lv.Notes)}, products, options)
	if err != nil {
		return lv
	}
	lv.Available = true
	lv.LineTotal = built.Lines[0].LineTotal
	return lv
}

// pricedCatalog trae los productos y opciones que usan los renglones, con la lista de precios dada.
func pricedCatalog(ctx context.Context, q *db.Queries, lista listaDePrecios, rows []db.OrderDraftLine) (
	map[int64]domain.PricedProduct, map[int64]domain.PricedOption, error) {
	var prodIDs, optIDs []int64
	for _, r := range rows {
		prodIDs = append(prodIDs, r.ProductID)
		for _, m := range modsOf(r.Modifiers) {
			optIDs = append(optIDs, m.OptionID)
		}
	}
	products := map[int64]domain.PricedProduct{}
	options := map[int64]domain.PricedOption{}
	if len(prodIDs) == 0 {
		return products, options, nil
	}
	prodRows, err := q.GetPricedProducts(ctx, prodIDs)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range prodRows {
		products[p.ID] = domain.PricedProduct{
			ID: p.ID, Name: p.Name, Cost: p.CurrentCost, Active: p.IsActive,
			Price: domain.PlatformPrice(p.Price, lista.margen, lista.producto[p.ID]),
		}
	}
	if len(optIDs) > 0 {
		optRows, err := q.GetPricedOptions(ctx, optIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, o := range optRows {
			options[o.ID] = domain.PricedOption{
				ID: o.ID, Name: o.Name, Cost: o.CurrentCost, GroupTitle: o.GroupTitle, MaxPerLine: int(o.MaxPerLine),
				PriceDelta: domain.PlatformPrice(o.PriceDelta, lista.margen, lista.opcion[o.ID]),
			}
		}
	}
	return products, options, nil
}

func orderLineOf(productID int64, qty decimal.Decimal, mods []domain.DraftModifier, notes string) domain.OrderLineInput {
	in := domain.OrderLineInput{ProductID: productID, Qty: qty, Notes: notes}
	for _, m := range mods {
		in.Modifiers = append(in.Modifiers, domain.OrderModInput(m))
	}
	return in
}

// --- barrido --------------------------------------------------------------------------------------

// sweepDrafts descarta lo que ya no tiene caso tener vivo (research R-8): las cuentas sin tocar en
// domain.DraftIdleLimit (D-8) y lo «Nuevo» de pedidos que se cancelaron o reembolsaron. Suelta sus
// nombres.
//
// Perezoso y no un job: corre dentro de la transacción de quien lista o crea, que es justo cuando
// alguien va a mirar la fila. Un job sería otra pieza que vigilar para un efecto que nadie ve hasta
// que mira. Corre bajo RLS: solo barre la empresa de la sesión, y eso basta — nadie más ve su fila.
func sweepDrafts(ctx context.Context, q *db.Queries) error {
	rows, err := q.ListDraftsToSweep(ctx)
	if err != nil {
		return err
	}
	expired := domain.DiscardExpired
	closed := domain.DiscardOrderClosed
	for _, r := range rows {
		p := db.DiscardDraftParams{ID: r.ID}
		switch {
		case r.OrderVoided:
			p.Reason = &closed
		case domain.DraftExpired(r.UpdatedAt, r.DbNow):
			// Con el `updated_at` que se vio: si alguien la tocó entretanto, ya no venció.
			p.Reason, p.Seen = &expired, pgtype.Timestamptz{Time: r.UpdatedAt, Valid: true}
		default:
			continue
		}
		if err := discardAndRelease(ctx, q, p); err != nil {
			return err
		}
	}
	return nil
}

// discardAndRelease descarta una cuenta viva y devuelve su nombre a la bolsa. Una cuenta que ya no
// estaba viva (otra tableta la mandó o la descartó) no es error: no hay nada que hacer.
func discardAndRelease(ctx context.Context, q *db.Queries, p db.DiscardDraftParams) error {
	res, err := q.DiscardDraft(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if res.FolioName == nil || res.FolioScheme == nil {
		return nil // lo «Nuevo» de un pedido no tiene nombre propio
	}
	return q.ReleaseFolioName(ctx, db.ReleaseFolioNameParams{
		// La bolsa guarda animales: «Persa 2» suelta a «Persa».
		Scheme: *res.FolioScheme, Name: domain.FolioAnimal(*res.FolioName), TakenBefore: res.CreatedAt,
	})
}

// ImportAccount es una pestaña de la versión anterior (`egb:ticket:v2`) que sube al servidor (D-12).
type ImportAccount struct {
	ID        uuid.UUID
	FolioName string
	Header    *DraftHeaderPatch
	Lines     []DraftLineCmd
}

// ImportResult dice qué pasó con cada pestaña. La tableta borra su almacenamiento local solo cuando
// todas regresan con resultado.
type ImportResult struct {
	ID      uuid.UUID  `json:"id"`
	Outcome string     `json:"outcome"`
	DraftID *uuid.UUID `json:"draftId"`
	OrderID *int64     `json:"orderId"`
	// Reason dice por qué una pestaña se rechazó (`outcome: "rejected"`), en palabras de quien opera.
	Reason string `json:"reason,omitempty"`
}

// maxImportAccounts acota una subida: una tableta real trae dos o tres pestañas. Sin tope, una sola
// petición podría abrir cientos de cuentas con nombre y vaciar la bolsa de la empresa.
const maxImportAccounts = 20

// Import sube las pestañas de la versión anterior con el id de cada pestaña como id de la cuenta y
// un opId estable por renglón (los calcula la tableta): reintentar es inocuo.
//
// Una pestaña que ya se había mandado a cocina —su id ya es el `client_uuid` de un pedido o de un
// lote de renglones— NO vuelve como cuenta: la red se cayó después de que el servidor confirmó, y
// volver a abrirla mandaría la comida dos veces (R-10).
//
// Un producto que ya no está en el menú se queda fuera de la cuenta en vez de tumbar la pestaña
// entera: lo demás que se capturó sí sube. Si no queda nada, la pestaña se reporta vacía.
func (s *DraftsService) Import(ctx context.Context, accounts []ImportAccount, actor int64) ([]ImportResult, error) {
	if len(accounts) > maxImportAccounts {
		return nil, fmt.Errorf("%w: se suben a lo más %d cuentas a la vez", domain.ErrValidation, maxImportAccounts)
	}
	for _, a := range accounts {
		if a.ID == uuid.Nil() {
			return nil, fmt.Errorf("%w: una cuenta sin id", domain.ErrValidation)
		}
		if len(a.Lines) > domain.MaxDraftLines {
			return nil, tooManyLines()
		}
	}
	out := make([]ImportResult, 0, len(accounts))
	q := s.store.QC(ctx)
	for _, a := range accounts {
		r := ImportResult{ID: a.ID}
		if orderID, err := q.GetOrderIDByClientUUID(ctx, a.ID); err == nil {
			r.Outcome, r.OrderID = "already_sent", &orderID
			out = append(out, r)
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if orderID, err := q.GetLoteDeRenglones(ctx, a.ID); err == nil {
			r.Outcome, r.OrderID = "already_sent", &orderID
			out = append(out, r)
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		lines, err := sellableLines(ctx, q, a.Lines)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			r.Outcome = "skipped_empty"
			out = append(out, r)
			continue
		}
		v, created, err := s.Create(ctx, CreateDraftCmd{ID: a.ID, FolioName: a.FolioName, Header: a.Header, Lines: lines, Actor: actor})
		switch {
		case errors.Is(err, domain.ErrDraftDiscarded):
			// Ya subió y alguien la cerró en otra tableta: para esta tableta es «ya está», y puede
			// borrar su copia. Volver a abrirla resucitaría una cuenta que alguien decidió cerrar.
			id := a.ID
			r.Outcome, r.DraftID = "exists", &id
		case isRejection(err):
			// Una pestaña inservible (opción borrada, cabecera que ya no vale) se reporta sola: si tumbara
			// la subida, la tableta no borraría su copia y reintentaría para siempre con las demás.
			r.Outcome, r.Reason = "rejected", operatorText(err)
		case err != nil:
			return nil, err
		default:
			r.Outcome, r.DraftID = "created", &v.ID
			if !created {
				r.Outcome = "exists"
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// sellableLines deja fuera, en UNA lectura, los renglones cuyo producto ya no está en el menú: lo
// demás que se capturó sí sube. Antes se reintentaba la creación entera por cada producto faltante.
func sellableLines(ctx context.Context, q *db.Queries, lines []DraftLineCmd) ([]DraftLineCmd, error) {
	ids := make([]int64, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.ProductID)
	}
	rows, err := q.GetPricedProducts(ctx, ids)
	if err != nil {
		return nil, err
	}
	active := make(map[int64]bool, len(rows))
	for _, p := range rows {
		active[p.ID] = p.IsActive
	}
	out := make([]DraftLineCmd, 0, len(lines))
	for _, l := range lines {
		if active[l.ProductID] {
			out = append(out, l)
		}
	}
	return out, nil
}

// isRejection dice si un error es de LA PESTAÑA (lo que mandó no vale) y no de la base o del servidor.
func isRejection(err error) bool {
	for _, e := range []error{domain.ErrValidation, domain.ErrOptionNotFound, domain.ErrOptionOverMax,
		domain.ErrProductNotSell, domain.ErrPlatformNotFound, domain.ErrDescuentoMayorQueLaVenta, domain.ErrDraftAlreadySent} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// operatorText quita del mensaje el nombre del sentinel base, como hace httpapi con las respuestas.
func operatorText(err error) string {
	msg := err.Error()
	for _, p := range []string{domain.ErrValidation.Error() + ": ", domain.ErrConflict.Error() + ": "} {
		msg = strings.TrimPrefix(msg, p)
	}
	return msg
}

// SendResult es lo que vuelve de mandar una cuenta a cocina.
type SendResult struct {
	Order *OrderView `json:"order"`
	// PrintLineIDs son los renglones que la comanda imprime: todos si el pedido nació, solo lo nuevo
	// si se agregó, y ninguno en el reintento (cocina no vuelve a preparar lo que ya preparó).
	PrintLineIDs []int64 `json:"printLineIds"`
	// Created dice si nació el pedido o se agregó a uno que ya existía.
	Created bool `json:"created"`
}

// draftSendRetries: cuántas veces se vuelve a preparar el envío si otra tableta cambió la cuenta entre
// la lectura y la transacción. El reintento no vuelve a sortear el nombre: está amarrado.
const draftSendRetries = 3

// Send manda la cuenta a cocina (research R-4).
//
// Una cuenta nueva se convierte en pedido por `createInTx`; lo «Nuevo» de un pedido se le agrega por
// `addLinesInTx`. Lo que se lee (precios, composición) se prepara FUERA; la escritura del pedido y la
// marca de enviada van en UNA transacción: con dos, habría una ventana donde el pedido existe y la
// cuenta sigue viva, y otra tableta la vería y la mandaría otra vez.
//
// Idempotente por la cuenta: su id es el `client_uuid` del pedido o del lote. Si ya se envió, devuelve
// el pedido sin escribir nada y sin renglones para imprimir.
func (s *DraftsService) Send(ctx context.Context, id uuid.UUID, actor int64) (*SendResult, error) {
	for range draftSendRetries {
		res, retry, err := s.trySend(ctx, id, actor)
		if !retry {
			return res, err
		}
	}
	return nil, fmt.Errorf("%w: la cuenta está cambiando en otra tableta; vuelve a intentarlo", domain.ErrDraftChanged)
}

// trySend hace un intento; `retry` dice que la cuenta cambió entre la lectura y el candado.
func (s *DraftsService) trySend(ctx context.Context, id uuid.UUID, actor int64) (*SendResult, bool, error) {
	q := s.store.QC(ctx)
	d, err := q.GetDraft(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, fmt.Errorf("%w: esa cuenta ya no existe", domain.ErrNotFound)
		}
		return nil, false, err
	}
	switch d.Status {
	case domain.DraftSent:
		res, err := s.alreadySent(ctx, d)
		return res, false, err
	case domain.DraftDiscarded:
		return nil, false, domain.ErrDraftDiscarded
	}
	rows, err := q.ListDraftLines(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, fmt.Errorf("%w: la cuenta no tiene productos", domain.ErrValidation)
	}
	lines, err := orderLinesOf(ctx, q, rows)
	if err != nil {
		return nil, false, err
	}
	if d.OrderID != nil {
		return s.sendNew(ctx, d, lines, actor)
	}
	return s.sendAccount(ctx, d, lines)
}

// sendAccount convierte una cuenta nueva en pedido.
func (s *DraftsService) sendAccount(ctx context.Context, d db.GetDraftRow, lines []domain.OrderLineInput) (*SendResult, bool, error) {
	cmd := CreateOrderCmd{
		ClientUUID: d.ID, ServiceType: domain.OrderServiceType(string(d.ServiceType), d.DeliveryPlatformID), DeliveryPlatformID: d.DeliveryPlatformID,
		CustomerName: d.CustomerName, PlatformOrderRef: d.PlatformOrderRef, OpenedBy: d.OpenedBy,
		DeliveryFee: d.DeliveryFee, DiscountAmount: d.DiscountAmount, DiscountPercent: d.DiscountPercent,
		Lines: lines, BoundFolioName: derefStr(d.FolioName),
		DiscountSetBy: d.DiscountSetBy, PlatformRefSetBy: d.PlatformRefSetBy,
	}
	sess, err := s.store.QC(ctx).GetOpenPrimarySession(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, domain.ErrNoOpenRegister
		}
		return nil, false, err
	}
	plan, err := s.orders.prepareCreate(ctx, cmd)
	if err != nil {
		return nil, false, err
	}
	var orderID int64
	var retry, created bool
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// El candado de la cuenta va ANTES de mirar si el pedido ya existe: dos tabletas que la mandan a
		// la vez se serializan aquí, y la segunda la ve enviada.
		locked, err := q.LockDraft(ctx, d.ID)
		if err != nil {
			return err
		}
		if err := draftStatusErr(locked.Status); err != nil {
			if errors.Is(err, domain.ErrDraftAlreadySent) {
				retry = true // la otra tableta ganó: el siguiente intento devuelve su pedido
				return nil
			}
			return err
		}
		if !locked.UpdatedAt.Equal(d.UpdatedAt) {
			retry = true
			return nil
		}
		// Una cuenta importada de la versión anterior que SÍ se había enviado (R-10): se marca contra
		// ese pedido en vez de crear otro.
		if existing, err := q.GetOrderIDByClientUUID(ctx, d.ID); err == nil {
			orderID = existing
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		} else {
			if orderID, err = s.orders.createInTx(ctx, q, plan, sess.ID); err != nil {
				return err
			}
			created = true
		}
		return markSent(ctx, q, d.ID, orderID)
	})
	if err != nil {
		return nil, false, s.orders.traduceFolioRepetido(ctx, err, plan.folio, cmd.DeliveryPlatformID)
	}
	if retry {
		return nil, true, nil
	}
	o, err := s.orders.load(ctx, orderID)
	if err != nil {
		return nil, false, err
	}
	res := &SendResult{Order: o, PrintLineIDs: []int64{}, Created: created}
	if created {
		res.PrintLineIDs = liveLineIDs(o)
	}
	return res, false, nil
}

// sendNew le agrega lo «Nuevo» a su pedido (D-3). Cocina recibe solo eso.
func (s *DraftsService) sendNew(ctx context.Context, d db.GetDraftRow, lines []domain.OrderLineInput, actor int64) (*SendResult, bool, error) {
	plan, err := s.orders.prepareAddLines(ctx, *d.OrderID, lines)
	if err != nil {
		return nil, false, err
	}
	var agregados []int64
	var retry bool
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// Pedido y luego cuenta, en el mismo orden que crear una «Nuevo»: al revés, una tableta que
		// manda y otra que agrega a la vez se interbloquean.
		if _, err := q.GetOrderForUpdate(ctx, *d.OrderID); err != nil {
			return err
		}
		locked, err := q.LockDraft(ctx, d.ID)
		if err != nil {
			return err
		}
		if err := draftStatusErr(locked.Status); err != nil {
			if errors.Is(err, domain.ErrDraftAlreadySent) {
				retry = true
				return nil
			}
			return err
		}
		if !locked.UpdatedAt.Equal(d.UpdatedAt) {
			retry = true
			return nil
		}
		if agregados, err = addLinesInTx(ctx, q, *d.OrderID, plan, actor, d.ID); err != nil {
			return err
		}
		return markSent(ctx, q, d.ID, *d.OrderID)
	})
	if err != nil || retry {
		return nil, retry, err
	}
	o, err := s.orders.load(ctx, *d.OrderID)
	if err != nil {
		return nil, false, err
	}
	if agregados == nil {
		agregados = []int64{}
	}
	return &SendResult{Order: o, PrintLineIDs: agregados, Created: false}, false, nil
}

// markSent marca la cuenta enviada y borra sus renglones, que ya viven en el pedido.
func markSent(ctx context.Context, q *db.Queries, id uuid.UUID, orderID int64) error {
	n, err := q.MarkDraftSent(ctx, db.MarkDraftSentParams{OrderID: &orderID, ID: id})
	if err != nil {
		return err
	}
	if n == 0 {
		// Imposible con la cuenta bloqueada y revisada viva; si pasa, mejor abortar que dejar un pedido
		// con su cuenta todavía viva (otra tableta la volvería a mandar).
		return domain.ErrDraftChanged
	}
	return q.DeleteDraftLinesOf(ctx, id)
}

// alreadySent responde el reintento de una cuenta ya enviada: el mismo pedido, nada que imprimir.
func (s *DraftsService) alreadySent(ctx context.Context, d db.GetDraftRow) (*SendResult, error) {
	o, err := s.orders.load(ctx, *d.OrderID)
	if err != nil {
		return nil, err
	}
	return &SendResult{Order: o, PrintLineIDs: []int64{}, Created: d.FolioName != nil}, nil
}

// orderLinesOf traduce los renglones de la cuenta a la entrada de un pedido. Una opción de
// modificador que se borró mientras la cuenta esperaba se rechaza NOMBRANDO el producto: el
// operador tiene la cuenta enfrente y un id no le dice qué renglón corregir.
func orderLinesOf(ctx context.Context, q *db.Queries, rows []db.OrderDraftLine) ([]domain.OrderLineInput, error) {
	lista, err := listaDePreciosQ(ctx, q, nil)
	if err != nil {
		return nil, err
	}
	products, options, err := pricedCatalog(ctx, q, lista, rows)
	if err != nil {
		return nil, err
	}
	out := make([]domain.OrderLineInput, 0, len(rows))
	for _, r := range rows {
		mods := modsOf(r.Modifiers)
		for _, m := range mods {
			if _, ok := options[m.OptionID]; !ok {
				name := "un producto"
				if p, ok := products[r.ProductID]; ok {
					name = p.Name
				}
				return nil, fmt.Errorf("%w en %s: quítalo o cámbialo", domain.ErrOptionNotFound, name)
			}
		}
		out = append(out, orderLineOf(r.ProductID, r.Qty, mods, derefStr(r.Notes)))
	}
	return out, nil
}

func liveLineIDs(o *OrderView) []int64 {
	out := make([]int64, 0, len(o.Lines))
	for _, l := range o.Lines {
		if !l.Cancelled {
			out = append(out, l.ID)
		}
	}
	return out
}

// Discard cierra una cuenta que no se mandó a cocina: queda «descartada» como rastro —no es venta
// cancelada, no gasta folio— y su nombre vuelve a la bolsa (D-7). Descartarla dos veces no es error.
// Una ya enviada se rechaza: ya es un pedido y se quita cancelándolo, con motivo y permiso.
func (s *DraftsService) Discard(ctx context.Context, id uuid.UUID, actor int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		d, err := q.LockDraft(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: esa cuenta ya no existe", domain.ErrNotFound)
			}
			return err
		}
		switch d.Status {
		case domain.DraftDiscarded:
			return nil
		case domain.DraftSent:
			return domain.ErrDraftAlreadySent
		}
		n, err := q.CountDraftLines(ctx, id)
		if err != nil {
			return err
		}
		reason := domain.DiscardManual
		if n == 0 {
			reason = domain.DiscardEmpty
		}
		return discardAndRelease(ctx, q, db.DiscardDraftParams{ID: id, DiscardedBy: &actor, Reason: &reason})
	})
}
