package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Emparejamiento de la tienda conectada (spec 026). Lo de la 020 que sigue igual vive en
// menus_de_plataforma.go; aquí está lo que el rediseño agregó.

// guardarPareja escribe una pareja dentro de la transacción de quien llama.
func guardarPareja(ctx context.Context, q *db.Queries, in AltaDePareja) error {
	previa, err := q.GetItemLink(ctx, db.GetItemLinkParams{ConnectionID: in.ConexionID, ExternalID: in.ExternalID})
	hayPrevia := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("pareja previa en la conexión %d: %w", in.ConexionID, err)
	}
	product, option := targetOf(in.ClaseLocal, in.LocalID)
	mismoDestino := hayPrevia && linkTarget(previa.ProductID, previa.ModifierOptionID) == in.LocalID &&
		previa.LocalKind == string(in.ClaseLocal)
	// Una pareja CONFIRMADA no se pisa sin decirlo: es trabajo manual que no se reconstruye.
	if hayPrevia && previa.ConfirmedAt.Valid && !mismoDestino && !in.Reemplazar {
		return domain.ErrParejaOcupada
	}

	otras, err := q.ListLinksOfTarget(ctx, db.ListLinksOfTargetParams{
		ConnectionID: in.ConexionID, ProductID: product, ModifierOptionID: option,
	})
	if err != nil {
		return fmt.Errorf("parejas del mismo destino: %w", err)
	}
	hermanas := 0
	for _, o := range otras {
		if o.ExternalID != in.ExternalID {
			hermanas++
		}
	}
	captura := true
	if hermanas > 0 {
		// Varias de la plataforma al mismo producto: cuál da el precio de la captura a mano lo decide
		// quien configura, aquí, para que nadie lo decida al operar (constitución 1.15.0).
		if in.PrecioDeCaptura == nil {
			return domain.ErrCapturePriceRequired
		}
		captura = *in.PrecioDeCaptura
	}
	if captura {
		if err := q.ClearCapturePrice(ctx, db.ClearCapturePriceParams{
			ConnectionID: in.ConexionID, ProductID: product, ModifierOptionID: option,
		}); err != nil {
			return fmt.Errorf("quitar el precio de captura anterior: %w", err)
		}
	}

	// Sin usuario se guarda NULL, no cero: `confirmed_by` referencia `users(id)`.
	var confirmadaPor *int64
	if in.UsuarioID != 0 {
		confirmadaPor = &in.UsuarioID
	}
	err = q.UpsertItemLink(ctx, db.UpsertItemLinkParams{
		ConnectionID: in.ConexionID, ExternalID: in.ExternalID, Kind: db.PlatformItemKind(in.Clase),
		ProductID: product, ModifierOptionID: option, LocalKind: string(in.ClaseLocal),
		ConfirmedBy: confirmadaPor, IsCapturePrice: captura,
	})
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" {
			// Las FK compuestas con company_id son la barrera real contra emparejar el catálogo de
			// otra empresa: los chequeos de integridad saltan RLS.
			if strings.Contains(pg.ConstraintName, "producto_de_la_empresa") || strings.Contains(pg.ConstraintName, "option_of_company") {
				return fmt.Errorf("%w: eso no es del catálogo de este negocio", domain.ErrValidation)
			}
			return fmt.Errorf("%w: el emparejamiento apunta a algo que ya no existe", domain.ErrValidation)
		}
		return fmt.Errorf("guardar una pareja en la conexión %d: %w", in.ConexionID, err)
	}

	// Si la pareja cambió de destino y daba el precio de captura del anterior, ese precio pasa a otra.
	if hayPrevia && !mismoDestino && previa.IsCapturePrice {
		resto, err := q.ListLinksOfTarget(ctx, db.ListLinksOfTargetParams{
			ConnectionID: in.ConexionID, ProductID: previa.ProductID, ModifierOptionID: previa.ModifierOptionID,
		})
		if err != nil {
			return fmt.Errorf("parejas del destino anterior: %w", err)
		}
		if siguiente, ok := domain.CapturePriceAfterUnlink(linksOf(resto)); ok {
			if err := q.SetCapturePrice(ctx, db.SetCapturePriceParams{ConnectionID: in.ConexionID, ExternalID: siguiente}); err != nil {
				return err
			}
		}
	}

	// Emparejar deshace la decisión «solo existe en la plataforma» de ese renglón, en la misma
	// transacción: si quedaran las dos, la pantalla no sabría en qué grupo ponerlo.
	if _, err := q.DeleteItemExclusion(ctx, db.DeleteItemExclusionParams{ConnectionID: in.ConexionID, ExternalID: in.ExternalID}); err != nil {
		return fmt.Errorf("quitar la decisión sin pareja: %w", err)
	}
	return nil
}

func targetOf(kind domain.ClaseLocal, id int64) (product, option *int64) {
	if kind == domain.LocalOpcion {
		return nil, &id
	}
	return &id, nil
}

// --- La vista de la pantalla ---

type PairingLinkView struct {
	LocalKind    string     `json:"localKind"`
	LocalID      int64      `json:"localId"`
	LocalName    string     `json:"localName"`
	CapturePrice bool       `json:"isCapturePrice"`
	ConfirmedAt  *time.Time `json:"confirmedAt"`
}

type PairingProposalView struct {
	LocalKind string `json:"localKind"`
	LocalID   int64  `json:"localId"`
	LocalName string `json:"localName"`
}

type PairingRow struct {
	ExternalID string               `json:"externalId"`
	Kind       string               `json:"kind"`
	Name       string               `json:"name"`
	Price      string               `json:"price"`
	Available  bool                 `json:"available"`
	Group      domain.PairingGroup  `json:"group"`
	Link       *PairingLinkView     `json:"link"`
	Proposal   *PairingProposalView `json:"proposal"`
}

type PriceChangeView struct {
	Name string  `json:"name"`
	Old  *string `json:"old"`
	New  string  `json:"new"`
}

type PairingView struct {
	Platform string             `json:"platformName"`
	Store    string             `json:"storeLabel"`
	ReadAt   time.Time          `json:"readAt"`
	Counts   domain.GroupCounts `json:"counts"`
	Items    []PairingRow       `json:"items"`
	Excluded []string           `json:"excludedLocal"`
	Changes  []PriceChangeView  `json:"priceChanges"`
}

// Pairing arma la pantalla: platillos y opciones de la última lectura, cada uno en su grupo, y los
// conteos sacados de esos mismos grupos.
func (s *MenusDePlataformaService) Pairing(ctx context.Context, conexionID int64) (*PairingView, error) {
	lectura, err := s.ultimaLecturaValida(ctx, conexionID)
	if err != nil {
		return nil, err
	}
	links, err := s.store.QC(ctx).ListItemLinks(ctx, conexionID)
	if err != nil {
		return nil, fmt.Errorf("parejas de la conexión %d: %w", conexionID, err)
	}
	porItem := make(map[string]db.ListItemLinksRow, len(links))
	for _, l := range links {
		porItem[l.ExternalID] = l
	}
	excl, err := s.store.QC(ctx).ListItemExclusions(ctx, conexionID)
	if err != nil {
		return nil, fmt.Errorf("decisiones de la conexión %d: %w", conexionID, err)
	}
	excluido := make(map[string]bool, len(excl))
	for _, e := range excl {
		excluido[e.ExternalID] = true
	}

	con, err := s.store.QC(ctx).GetPlatformConnection(ctx, conexionID)
	if err != nil {
		return nil, fmt.Errorf("conexión %d: %w", conexionID, err)
	}
	out := &PairingView{
		Platform: con.PlatformName, Store: con.Label, ReadAt: lectura.StartedAt,
		Items: []PairingRow{}, Excluded: []string{}, Changes: []PriceChangeView{},
	}
	var grupos []domain.PairingGroup
	for _, clase := range []domain.ClaseDeItem{domain.ItemPlatillo, domain.ItemOpcion} {
		arriba, err := s.itemsDeLectura(ctx, lectura.ID, clase)
		if err != nil {
			return nil, err
		}
		abajo, err := s.catalogoLocal(ctx, conexionID, clase)
		if err != nil {
			return nil, err
		}
		nombre := make(map[int64]string, len(abajo))
		for _, p := range abajo {
			nombre[p.ID] = p.Nombre
		}
		yaEmparejado := map[string]bool{}
		for id, l := range porItem {
			if l.ConfirmedAt.Valid {
				yaEmparejado[id] = true
			}
		}
		propuestas := map[string]domain.Pareja{}
		for _, p := range domain.ProponerParejas(arriba, abajo, yaEmparejado) {
			propuestas[p.ExternalID] = p
		}
		for _, it := range arriba {
			fila := PairingRow{ExternalID: it.ID, Kind: string(it.Clase), Name: it.Nombre, Available: it.Activo, Price: centsText(it.Centavos)}
			l, hay := porItem[it.ID]
			confirmada := hay && l.ConfirmedAt.Valid
			if confirmada {
				id := linkTarget(l.ProductID, l.ModifierOptionID)
				c := l.ConfirmedAt.Time
				fila.Link = &PairingLinkView{LocalKind: l.LocalKind, LocalID: id, LocalName: nombre[id], CapturePrice: l.IsCapturePrice, ConfirmedAt: &c}
			}
			prop, hayProp := propuestas[it.ID]
			if hayProp && !confirmada {
				fila.Proposal = &PairingProposalView{LocalKind: string(prop.ClaseLocal), LocalID: prop.LocalID, LocalName: nombre[prop.LocalID]}
			}
			fila.Group = domain.GroupOf(confirmada, hayProp, excluido[it.ID])
			grupos = append(grupos, fila.Group)
			out.Items = append(out.Items, fila)
		}
	}
	out.Counts = domain.CountGroups(grupos)

	cambios, err := s.store.QC(ctx).ListPlatformPriceChanges(ctx, lectura.ID)
	if err != nil {
		return nil, fmt.Errorf("cambios de precio de la lectura %d: %w", lectura.ID, err)
	}
	for _, c := range cambios {
		v := PriceChangeView{Name: c.Name, New: c.NewPrice.StringFixed(2)}
		if c.OldPrice != nil {
			old := c.OldPrice.StringFixed(2)
			v.Old = &old
		}
		out.Changes = append(out.Changes, v)
	}
	return out, nil
}

func centsText(c int64) string {
	p, err := domain.PesosDeCentavos(c)
	if err != nil {
		return ""
	}
	return p.StringFixed(2)
}

// --- Candidatos ---

type CandidateView struct {
	LocalKind   string `json:"localKind"`
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	LinkedCount int    `json:"linkedCount"`
}

const maxCandidates = 20

// Candidates lista lo del POS con lo que se puede ligar un renglón: productos para un platillo y
// opciones para una opción, ordenados por parecido. Un producto ya ligado SIGUE apareciendo, con
// cuántos platillos lo usan: varios platillos al mismo producto es válido (spec 026, defecto 1).
func (s *MenusDePlataformaService) Candidates(ctx context.Context, conexionID int64, externalID, texto string) ([]CandidateView, error) {
	lectura, err := s.ultimaLecturaValida(ctx, conexionID)
	if err != nil {
		return nil, err
	}
	var item *domain.ItemDePlataforma
	for _, clase := range []domain.ClaseDeItem{domain.ItemPlatillo, domain.ItemOpcion} {
		arriba, err := s.itemsDeLectura(ctx, lectura.ID, clase)
		if err != nil {
			return nil, err
		}
		for i := range arriba {
			if arriba[i].ID == externalID {
				item = &arriba[i]
			}
		}
	}
	if item == nil {
		return nil, domain.ErrItemInexistente
	}
	local, ok := domain.ClaseLocalDe(item.Clase)
	if !ok {
		return nil, fmt.Errorf("%w: el nivel %q no se empareja", domain.ErrValidation, item.Clase)
	}
	abajo, err := s.catalogoLocal(ctx, conexionID, item.Clase)
	if err != nil {
		return nil, err
	}
	links, err := s.store.QC(ctx).ListItemLinks(ctx, conexionID)
	if err != nil {
		return nil, err
	}
	usados := map[int64]int{}
	for _, l := range links {
		if l.LocalKind == string(local) && l.ConfirmedAt.Valid {
			usados[linkTarget(l.ProductID, l.ModifierOptionID)]++
		}
	}
	criterio := item.Nombre
	if t := strings.TrimSpace(texto); t != "" {
		criterio = t
		filtrados := abajo[:0:0]
		for _, p := range abajo {
			if strings.Contains(strings.ToLower(p.Nombre), strings.ToLower(t)) {
				filtrados = append(filtrados, p)
			}
		}
		abajo = filtrados
	}
	ordenados := domain.RankCandidates(criterio, abajo)
	out := make([]CandidateView, 0, min(len(ordenados), maxCandidates))
	for _, p := range ordenados {
		if len(out) == maxCandidates {
			break
		}
		out = append(out, CandidateView{LocalKind: string(local), ID: p.ID, Name: p.Nombre, LinkedCount: usados[p.ID]})
	}
	return out, nil
}

// --- Lote ---

type BatchResult struct {
	Confirmed []string `json:"confirmed"`
	Skipped   []string `json:"skipped"`
}

// ConfirmBatch confirma las propuestas VIGENTES de esos renglones en una transacción. Lo que ya no
// tiene propuesta (otra persona lo cambió) se reporta como omitido, sin tumbar el lote.
//
// Si el producto ya tiene pareja en la tienda, la nueva NO toma el precio de captura: lo conserva
// la que lo tenía. Así el lote no necesita preguntar nada.
func (s *MenusDePlataformaService) ConfirmBatch(ctx context.Context, conexionID, usuarioID int64, externalIDs []string) (*BatchResult, error) {
	vista, err := s.Pairing(ctx, conexionID)
	if err != nil {
		return nil, err
	}
	porID := make(map[string]PairingRow, len(vista.Items))
	for _, it := range vista.Items {
		porID[it.ExternalID] = it
	}
	res := &BatchResult{Confirmed: []string{}, Skipped: []string{}}
	var confirmadaPor *int64
	if usuarioID != 0 {
		confirmadaPor = &usuarioID
	}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		for _, id := range externalIDs {
			fila, ok := porID[id]
			if !ok || fila.Proposal == nil || fila.Group != domain.GroupToReview {
				res.Skipped = append(res.Skipped, id)
				continue
			}
			kind := domain.ClaseLocal(fila.Proposal.LocalKind)
			product, option := targetOf(kind, fila.Proposal.LocalID)
			otras, err := q.ListLinksOfTarget(ctx, db.ListLinksOfTargetParams{ConnectionID: conexionID, ProductID: product, ModifierOptionID: option})
			if err != nil {
				return err
			}
			n, err := q.ConfirmItemLinkProposal(ctx, db.ConfirmItemLinkProposalParams{
				ConnectionID: conexionID, ExternalID: id, Kind: db.PlatformItemKind(fila.Kind),
				ProductID: product, ModifierOptionID: option, LocalKind: string(kind),
				ConfirmedBy: confirmadaPor, IsCapturePrice: len(otras) == 0,
			})
			if err != nil {
				return fmt.Errorf("confirmar %q: %w", id, err)
			}
			if n == 0 {
				// Ya había pareja (un id repetido en el lote, u otra persona primero): no la escribió
				// este lote y no se reporta como suya.
				res.Skipped = append(res.Skipped, id)
				continue
			}
			res.Confirmed = append(res.Confirmed, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// --- Decisiones sin pareja ---

// ExcludeItem guarda «solo existe en la plataforma» para un renglón de la tienda.
func (s *MenusDePlataformaService) ExcludeItem(ctx context.Context, conexionID, usuarioID int64, externalID string) error {
	lectura, err := s.ultimaLecturaValida(ctx, conexionID)
	if err != nil {
		return err
	}
	for _, clase := range []domain.ClaseDeItem{domain.ItemPlatillo, domain.ItemOpcion} {
		existe, err := s.store.QC(ctx).MenuItemExistsInRead(ctx, db.MenuItemExistsInReadParams{
			ReadID: lectura.ID, ExternalID: externalID, Kind: db.PlatformItemKind(clase),
		})
		if err != nil {
			return err
		}
		if existe {
			return s.store.QC(ctx).InsertItemExclusion(ctx, db.InsertItemExclusionParams{
				ConnectionID: conexionID, ExternalID: externalID, Kind: db.PlatformItemKind(clase), DecidedBy: usuarioID,
			})
		}
	}
	return domain.ErrItemInexistente
}

func (s *MenusDePlataformaService) UnexcludeItem(ctx context.Context, conexionID int64, externalID string) error {
	n, err := s.store.QC(ctx).DeleteItemExclusion(ctx, db.DeleteItemExclusionParams{ConnectionID: conexionID, ExternalID: externalID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ExcludeLocal guarda «no se vende en la plataforma» para un producto u opción del POS.
func (s *MenusDePlataformaService) ExcludeLocal(ctx context.Context, conexionID, usuarioID int64, kind domain.ClaseLocal, localID int64) error {
	if kind != domain.LocalProducto && kind != domain.LocalOpcion {
		return fmt.Errorf("%w: tipo local desconocido %q", domain.ErrValidation, kind)
	}
	product, option := targetOf(kind, localID)
	err := s.store.QC(ctx).InsertLocalExclusion(ctx, db.InsertLocalExclusionParams{
		ConnectionID: conexionID, ProductID: product, ModifierOptionID: option, DecidedBy: usuarioID,
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23503" {
		return fmt.Errorf("%w: eso no es del catálogo de este negocio", domain.ErrValidation)
	}
	return err
}

func (s *MenusDePlataformaService) UnexcludeLocal(ctx context.Context, conexionID int64, kind domain.ClaseLocal, localID int64) error {
	// Mismo rechazo que ExcludeLocal: un tipo mal escrito NO se lee como «producto» (principio V).
	if kind != domain.LocalProducto && kind != domain.LocalOpcion {
		return fmt.Errorf("%w: tipo local desconocido %q", domain.ErrValidation, kind)
	}
	product, option := targetOf(kind, localID)
	n, err := s.store.QC(ctx).DeleteLocalExclusion(ctx, db.DeleteLocalExclusionParams{ConnectionID: conexionID, ProductID: product, ModifierOptionID: option})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- Sincronización de precios ---

// syncPrices copia al POS los precios de lo emparejado después de una lectura buena. Corre con la
// empresa fijada (WithTenant): la goroutine de la lectura nace sin empresa y RLS no la dejaría
// escribir. Si falla, la lectura sigue siendo buena; los precios se quedan como estaban.
func (s *MenusDePlataformaService) syncPrices(ctx context.Context, companyID, conexionID, lecturaID, usuarioID int64, items []domain.ItemDePlataforma) {
	err := s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		con, err := q.GetPlatformConnection(ctx, conexionID)
		if err != nil {
			return err
		}
		tiendas, err := q.CountConnectionsOfPlatform(ctx, con.DeliveryPlatformID)
		if err != nil {
			return err
		}
		filas, err := q.ListItemLinks(ctx, conexionID)
		if err != nil {
			return err
		}
		links := make([]domain.PairingLink, 0, len(filas))
		for _, f := range filas {
			links = append(links, domain.PairingLink{
				ExternalID: f.ExternalID, Confirmed: f.ConfirmedAt.Valid, CapturePrice: f.IsCapturePrice, CreatedAt: f.CreatedAt,
				Target: domain.PriceTarget{Kind: domain.ClaseLocal(f.LocalKind), ID: linkTarget(f.ProductID, f.ModifierOptionID)},
			})
		}
		actuales := map[domain.PriceTarget]domain.CurrentPrice{}
		prod, err := q.ListProductPlatformPricesOfPlatform(ctx, con.DeliveryPlatformID)
		if err != nil {
			return err
		}
		for _, p := range prod {
			actuales[domain.PriceTarget{Kind: domain.LocalProducto, ID: p.ProductID}] = domain.CurrentPrice{Price: p.Price, FromPlatform: p.Source == "platform"}
		}
		opts, err := q.ListOptionPlatformPricesOfPlatform(ctx, con.DeliveryPlatformID)
		if err != nil {
			return err
		}
		for _, o := range opts {
			actuales[domain.PriceTarget{Kind: domain.LocalOpcion, ID: o.OptionID}] = domain.CurrentPrice{Price: o.PriceDelta, FromPlatform: o.Source == "platform"}
		}

		writes, err := domain.PriceSync(items, links, actuales, int(tiendas), s.ahora())
		if err != nil {
			return err
		}
		synced := pgtype.Timestamptz{Time: s.ahora(), Valid: true}
		for _, w := range writes {
			switch w.Target.Kind {
			case domain.LocalProducto:
				err = q.SyncProductPlatformPrice(ctx, db.SyncProductPlatformPriceParams{
					ProductID: w.Target.ID, PlatformID: con.DeliveryPlatformID, Price: w.Price, UpdatedBy: usuarioID, SyncedAt: synced,
				})
			case domain.LocalOpcion:
				err = q.SyncOptionPlatformPrice(ctx, db.SyncOptionPlatformPriceParams{
					OptionID: w.Target.ID, PlatformID: con.DeliveryPlatformID, PriceDelta: w.Price, UpdatedBy: usuarioID, SyncedAt: synced,
				})
			}
			if err != nil {
				return fmt.Errorf("copiar el precio de %q: %w", w.ExternalID, err)
			}
			if !w.Changed {
				continue
			}
			product, option := targetOf(w.Target.Kind, w.Target.ID)
			if err := q.InsertPlatformPriceChange(ctx, db.InsertPlatformPriceChangeParams{
				ReadID: lecturaID, ExternalID: w.ExternalID, Name: w.Name,
				ProductID: product, ModifierOptionID: option, OldPrice: w.Old, NewPrice: w.Price,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil && s.onPricesSynced != nil {
		s.onPricesSynced(ctx, companyID)
	}
	if err != nil {
		// Clave estable y sin el mensaje: puede traer datos del catálogo.
		clase := "error"
		if errors.Is(err, domain.ErrSeveralStoresSamePlatform) {
			clase = "several_stores_same_platform"
		}
		logging.SecurityEvent(ctx, "platform_price_sync_failed", "connection_id", conexionID, "reason", clase)
	}
}
