package app

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// TransferInput es un traspaso como lo manda la pantalla (o el MCP).
type TransferInput struct {
	ClientKey       uuid.UUID
	FromWarehouseID int64
	ToWarehouseID   int64
	Note            string
	ConfirmNegative bool
	Lines           []domain.TransferLine
}

// MaxTransferNote es el largo de la nota del traspaso, como la columna.
const MaxTransferNote = 200

// TransferResult dice qué traspaso quedó y si es uno que ya existía (un reintento).
type TransferResult struct {
	ID     int64
	Replay bool
}

// CreateTransfer mueve mercancía entre dos almacenes activos de la sucursal de la sesión (spec 094).
// Cada renglón deja dos movimientos `traspaso` ligados al documento, con el costo vigente del
// artículo. Un reintento con la misma clave y el mismo cuerpo devuelve el traspaso que ya existe; con
// otro cuerpo, ErrTransferKeyReused.
func (s *BackofficeService) CreateTransfer(ctx context.Context, in TransferInput, userID int64) (TransferResult, error) {
	lines, err := domain.ValidateTransfer(in.FromWarehouseID, in.ToWarehouseID, in.Lines)
	if err != nil {
		return TransferResult{}, err
	}
	note := strings.TrimSpace(in.Note)
	if len([]rune(note)) > MaxTransferNote {
		return TransferResult{}, fmt.Errorf("%w: la nota lleva hasta %d letras", domain.ErrValidation, MaxTransferNote)
	}
	if in.ClientKey == (uuid.UUID{}) {
		return TransferResult{}, fmt.Errorf("%w: falta la clave del traspaso", domain.ErrValidation)
	}
	// Orden fijo de renglones: dos traspasos con los mismos artículos toman sus filas en el mismo orden.
	slices.SortFunc(lines, func(a, b domain.TransferLine) int {
		if a.ItemType != b.ItemType {
			return strings.Compare(a.ItemType, b.ItemType)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	hash := transferHash(in.FromWarehouseID, in.ToWarehouseID, note, lines)

	var out TransferResult
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// Un insumo unido va a su destino (hasta 16 saltos, como el trigger de la 0104). La huella ya se
		// tomó sobre lo que mandó la tableta: un reintento después de la unión sigue siendo el mismo.
		if err := redirectMerged(ctx, q, lines); err != nil {
			return err
		}
		// Si ya existe, es un reintento: se responde sin volver a revisar nada.
		if prev, err := q.TransferByClientKey(ctx, in.ClientKey); err == nil {
			if prev.BodyHash != hash {
				return domain.ErrTransferKeyReused
			}
			out = TransferResult{ID: prev.ID, Replay: true}
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// Los dos almacenes for share (archivar los toma for update): ninguno se archiva a la mitad.
		for _, id := range []int64{in.FromWarehouseID, in.ToWarehouseID} {
			if _, err := ResolveWarehouseID(ctx, q, domain.WarehouseSel{ID: id}); err != nil {
				return err
			}
		}
		var ings, prods []int64
		for _, l := range lines {
			if l.ItemType == "ingrediente" {
				ings = append(ings, l.ID)
			} else {
				prods = append(prods, l.ID)
			}
		}
		// Los insumos sin esperar, antes de escribir, como la compra: una venta en curso responde 409
		// «vuelve a intentar» en vez de una espera que puede cruzarse.
		slices.Sort(ings)
		if err := lockIngredients(ctx, q, ings); err != nil {
			return err
		}
		// Los productos igual: sin esto el traspaso esperaba a una venta del mismo producto.
		slices.Sort(prods)
		if len(prods) > 0 {
			if _, err := q.LockProductStockNowait(ctx, prods); err != nil {
				return lockErr(err)
			}
		}
		arts, err := q.TransferArticles(ctx, db.TransferArticlesParams{IngredientIds: ings, ProductIds: prods})
		if err != nil {
			return err
		}
		// Después de seguir las uniones, A y su destino B son el mismo renglón: se dice así en vez de
		// «no existe».
		if err := domain.TransferDuplicates(lines); err != nil {
			return err
		}
		cost := map[domain.StockKey]decimal.Decimal{}
		for _, a := range arts {
			if !a.IsActive {
				return fmt.Errorf("%w: un artículo del traspaso está archivado", domain.ErrValidation)
			}
			cost[domain.StockKey{ItemType: string(a.ItemType), ID: a.ID}] = a.CurrentCost
		}
		if len(cost) != len(lines) {
			return fmt.Errorf("%w: un artículo del traspaso no existe o no lleva existencia", domain.ErrValidation)
		}
		levels, err := q.WarehouseOnHand(ctx, db.WarehouseOnHandParams{WarehouseID: in.FromWarehouseID, IngredientIds: ings, ProductIds: prods})
		if err != nil {
			return err
		}
		onHand := map[domain.StockKey]decimal.Decimal{}
		for _, l := range levels {
			onHand[salesStockKey(string(l.ItemType), l.IngredientID, l.ProductID)] = l.OnHand
		}
		if err := domain.CheckTransferStock(lines, onHand, in.ConfirmNegative); err != nil {
			return err
		}

		var notePtr *string
		if note != "" {
			notePtr = &note
		}
		id, err := q.InsertTransfer(ctx, db.InsertTransferParams{FromWarehouseID: in.FromWarehouseID, ToWarehouseID: in.ToWarehouseID,
			Note: notePtr, ClientKey: in.ClientKey, BodyHash: hash, CreatedBy: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			// Otra petición con la misma clave ganó entre la lectura y el insert: es su reintento.
			prev, perr := q.TransferByClientKey(ctx, in.ClientKey)
			if perr != nil {
				return perr
			}
			if prev.BodyHash != hash {
				return domain.ErrTransferKeyReused
			}
			out = TransferResult{ID: prev.ID, Replay: true}
			return nil
		}
		if err != nil {
			return err
		}
		reason := "traspaso"
		for i, l := range lines {
			var ing, prod *int64
			if l.ItemType == "ingrediente" {
				ing = &l.ID
			} else {
				prod = &l.ID
			}
			if err := q.InsertTransferLine(ctx, db.InsertTransferLineParams{TransferID: id, LineNo: int16(i + 1),
				ItemType: db.StockItemType(l.ItemType), IngredientID: ing, ProductID: prod, QtyBase: l.Qty}); err != nil {
				return err
			}
			c := cost[l.Key()]
			for _, leg := range []struct {
				wh  int64
				qty decimal.Decimal
			}{{in.FromWarehouseID, l.Qty.Neg()}, {in.ToWarehouseID, l.Qty}} {
				if err := q.InsertStockTransferMovement(ctx, db.InsertStockTransferMovementParams{WarehouseID: leg.wh,
					ItemType: db.StockItemType(l.ItemType), IngredientID: ing, ProductID: prod, Quantity: leg.qty,
					UnitCost: &c, TransferID: &id, UserID: &userID, Reason: &reason, Note: notePtr}); err != nil {
					return err
				}
			}
		}
		branch, err := q.CurrentBranchID(ctx)
		if err != nil {
			return err
		}
		after := map[string]any{"from": in.FromWarehouseID, "to": in.ToWarehouseID, "lines": len(lines)}
		if err := auditEntity(ctx, q, domain.AuditTransferCreated, "transfer", id, &branch, &userID, nil, after); err != nil {
			return err
		}
		out = TransferResult{ID: id}
		return nil
	})
	return out, err
}

// redirectMerged cambia, en su lugar, cada insumo unido por el destino de la unión.
func redirectMerged(ctx context.Context, q *db.Queries, lines []domain.TransferLine) error {
	for range 16 {
		var ids []int64
		for _, l := range lines {
			if l.ItemType == "ingrediente" {
				ids = append(ids, l.ID)
			}
		}
		if len(ids) == 0 {
			return nil
		}
		rows, err := q.MergeTargets(ctx, ids)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		to := map[int64]int64{}
		for _, r := range rows {
			to[r.OldIngredientID] = r.NewIngredientID
		}
		for i := range lines {
			if n, ok := to[lines[i].ID]; ok && lines[i].ItemType == "ingrediente" {
				lines[i].ID = n
			}
		}
	}
	return nil
}

// transferHash es la huella del cuerpo que decide si un reintento es el mismo traspaso. Va sobre los
// renglones ya ordenados y redondeados, para que el orden en que la tableta los mandó no la cambie.
func transferHash(from, to int64, note string, lines []domain.TransferLine) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%s", from, to, note)
	for _, l := range lines {
		fmt.Fprintf(&b, "|%s:%d:%s", l.ItemType, l.ID, l.Qty.String())
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// TransferView es un traspaso para la lista.
type TransferView struct {
	ID              int64              `json:"id"`
	FromWarehouseID int64              `json:"fromWarehouseId"`
	FromName        string             `json:"fromName"`
	ToWarehouseID   int64              `json:"toWarehouseId"`
	ToName          string             `json:"toName"`
	Note            *string            `json:"note"`
	CreatedAt       time.Time          `json:"createdAt"`
	CreatedBy       string             `json:"createdBy"`
	Lines           []TransferLineView `json:"lines"`
}

// TransferLineView es un renglón de un traspaso.
type TransferLineView struct {
	ItemType     string          `json:"itemType"`
	IngredientID *int64          `json:"ingredientId"`
	ProductID    *int64          `json:"productId"`
	Name         string          `json:"name"`
	Qty          decimal.Decimal `json:"qty"`
	UnitCode     string          `json:"unitCode"`
}

// ListTransfers son los traspasos recientes de la sucursal, con sus renglones. Siempre arreglos.
func (s *BackofficeService) ListTransfers(ctx context.Context, limit int32) ([]TransferView, error) {
	q := s.store.QC(ctx)
	rows, err := q.ListTransfers(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TransferView, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	idx := map[int64]int{}
	for _, r := range rows {
		idx[r.ID] = len(out)
		ids = append(ids, r.ID)
		out = append(out, TransferView{ID: r.ID, FromWarehouseID: r.FromWarehouseID, FromName: r.FromName,
			ToWarehouseID: r.ToWarehouseID, ToName: r.ToName, Note: r.Note, CreatedAt: r.CreatedAt,
			CreatedBy: r.CreatedByName, Lines: []TransferLineView{}})
	}
	if len(ids) == 0 {
		return out, nil
	}
	lines, err := q.ListTransferLines(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		i := idx[l.TransferID]
		out[i].Lines = append(out[i].Lines, TransferLineView{ItemType: string(l.ItemType), IngredientID: l.IngredientID,
			ProductID: l.ProductID, Name: l.Name, Qty: l.QtyBase, UnitCode: l.UnitCode})
	}
	return out, nil
}
