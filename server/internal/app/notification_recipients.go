package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Quién recibe los avisos de inventario por correo (spec 084, historia 7). Por PERMISO y no por nombre
// de rol (constitución): por omisión, quien puede configurar el almacén y tiene correo. La lista es
// propia, separada de la del cierre de caja, que lleva cifras de caja.

// RecipientKinds son los tipos de aviso que se eligen por destinatario.
var recipientKinds = []string{"stock", "not_found"}

// RecipientPerson es una persona que recibe por omisión, con lo que recibe de cada tipo.
type RecipientPerson struct {
	UserID   int64  `json:"userId"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	HasEmail bool   `json:"hasEmail"`
	Stock    bool   `json:"stock"`
	NotFound bool   `json:"notFound"`
}

// RecipientExtra es un correo que no es de ninguna persona del sistema.
type RecipientExtra struct {
	Email    string `json:"email"`
	Stock    bool   `json:"stock"`
	NotFound bool   `json:"notFound"`
}

// RecipientsView es la pantalla de destinatarios. Nunca con arreglos nil.
type RecipientsView struct {
	MorningAt string            `json:"morningAt"`
	People    []RecipientPerson `json:"people"`
	Extras    []RecipientExtra  `json:"extras"`
}

func recipientsState(ctx context.Context, q *db.Queries) (RecipientsView, error) {
	users, err := q.UsersWithPermission(ctx, string(domain.PermInventoryConfigure))
	if err != nil {
		return RecipientsView{}, err
	}
	rows, err := q.ListNotificationRecipients(ctx)
	if err != nil {
		return RecipientsView{}, err
	}
	at, err := q.NotificationsMorningAt(ctx)
	if err != nil {
		return RecipientsView{}, err
	}
	off := map[[2]string]bool{} // (usuario, tipo) quitado
	extras := map[string]*RecipientExtra{}
	var order []string
	for _, r := range rows {
		switch {
		case r.UserID != nil && !r.Enabled:
			off[[2]string{fmt.Sprint(*r.UserID), r.Kind}] = true
		case r.Email != "":
			e, ok := extras[r.Email]
			if !ok {
				e = &RecipientExtra{Email: r.Email}
				extras[r.Email] = e
				order = append(order, r.Email)
			}
			if r.Kind == "stock" {
				e.Stock = r.Enabled
			} else {
				e.NotFound = r.Enabled
			}
		}
	}
	v := RecipientsView{MorningAt: at, People: make([]RecipientPerson, 0, len(users)), Extras: make([]RecipientExtra, 0, len(order))}
	for _, u := range users {
		id := fmt.Sprint(u.ID)
		v.People = append(v.People, RecipientPerson{UserID: u.ID, Name: u.Name, Role: u.RoleName, Email: u.Email, HasEmail: u.Email != "",
			Stock: !off[[2]string{id, "stock"}], NotFound: !off[[2]string{id, "not_found"}]})
	}
	for _, e := range order {
		v.Extras = append(v.Extras, *extras[e])
	}
	return v, nil
}

// Recipients lee la pantalla de destinatarios.
func (s *NotificationsService) Recipients(ctx context.Context) (RecipientsView, error) {
	var v RecipientsView
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		v, err = recipientsState(ctx, q)
		return err
	})
	return v, err
}

// SetRecipients reemplaza la configuración completa: una persona marcada en un tipo vuelve a la
// omisión; desmarcada, se quita; los correos extra se normalizan (a lo más 10). Una persona sin el
// permiso no se puede agregar: recibir avisos sigue al permiso, no a esta lista.
func (s *NotificationsService) SetRecipients(ctx context.Context, in RecipientsView, actor int64) error {
	if err := domain.ValidMorningTime(in.MorningAt); err != nil {
		return err
	}
	emails := make([]string, len(in.Extras))
	for i, e := range in.Extras {
		emails[i] = e.Email
	}
	clean, err := domain.NormalizeSummaryEmails(emails)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockNotificationRecipients(ctx); err != nil {
			return err
		}
		before, err := recipientsState(ctx, q)
		if err != nil {
			return err
		}
		eligible := map[int64]bool{}
		for _, p := range before.People {
			eligible[p.UserID] = true
		}
		if err := q.ClearNotificationRecipients(ctx); err != nil {
			return err
		}
		for _, p := range in.People {
			if !eligible[p.UserID] {
				return fmt.Errorf("%w: esa persona no puede configurar el almacén; recibe quien tiene ese permiso", domain.ErrValidation)
			}
			for _, k := range recipientKinds {
				if (k == "stock" && p.Stock) || (k == "not_found" && p.NotFound) {
					continue
				}
				if err := q.InsertNotificationRecipient(ctx, db.InsertNotificationRecipientParams{UserID: &p.UserID, Kind: k, Enabled: false, UpdatedBy: actor}); err != nil {
					return err
				}
			}
		}
		for i, e := range in.Extras {
			email := clean[i]
			for _, k := range recipientKinds {
				on := (k == "stock" && e.Stock) || (k == "not_found" && e.NotFound)
				if err := q.InsertNotificationRecipient(ctx, db.InsertNotificationRecipientParams{Email: &email, Kind: k, Enabled: on, UpdatedBy: actor}); err != nil {
					return err
				}
			}
		}
		if err := q.SetNotificationsMorningAt(ctx, in.MorningAt); err != nil {
			return err
		}
		after := in
		after.Extras = slices.Clone(in.Extras)
		for i := range after.Extras {
			after.Extras[i].Email = clean[i]
		}
		return recordAudit(ctx, q, domain.AuditEvent{Action: domain.AuditNotificationRecipientsChanged, EntityType: "notification_recipients",
			ActorKind: domain.ActorUser, ActorUserID: &actor, Before: before, After: after})
	})
}

// RecipientsFor son los correos que reciben un tipo de aviso ahora: los de la omisión que no se
// quitaron y tienen correo, más los extra de ese tipo. Sin repetidos.
func (s *NotificationsService) RecipientsFor(ctx context.Context, kind string) ([]string, error) {
	var out []string
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		out, err = recipientsFor(ctx, q, kind)
		return err
	})
	return out, err
}

func recipientsFor(ctx context.Context, q *db.Queries, kind string) ([]string, error) {
	v, err := recipientsState(ctx, q)
	if err != nil {
		return nil, err
	}
	out := []string{}
	add := func(e string) {
		if e != "" && !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	for _, p := range v.People {
		if (kind == "stock" && p.Stock) || (kind == "not_found" && p.NotFound) {
			add(p.Email)
		}
	}
	for _, e := range v.Extras {
		if (kind == "stock" && e.Stock) || (kind == "not_found" && e.NotFound) {
			add(e.Email)
		}
	}
	return out, nil
}
