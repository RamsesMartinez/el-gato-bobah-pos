//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// catKit es una empresa con categorías sembradas como owner y el servicio corriendo como
// gatobobah_app (RLS aplica), en la empresa de prueba.
type catKit struct {
	st, appSt *store.Store
	svc       *app.AdminService
	ctx       context.Context
	user      int64
}

func newCatKit(t *testing.T) *catKit {
	t.Helper()
	k := &catKit{st: newTestStore(t)}
	k.appSt = appRoleStore(t)
	k.svc = app.NewAdminService(k.appSt)
	k.user = makeUser(t, k.st, "cat_admin", "admin")
	ctx, release, err := k.appSt.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	k.ctx = ctx
	return k
}

// cat siembra una categoría como owner en la empresa dada.
func (k *catKit) cat(t *testing.T, company int64, name string, parent *int64, active bool) int64 {
	t.Helper()
	var id int64
	if err := k.st.Pool.QueryRow(context.Background(),
		`insert into categories (company_id, name, parent_id, is_active) values ($1, $2, $3, $4) returning id`,
		company, name, parent, active).Scan(&id); err != nil {
		t.Fatalf("categoría %s: %v", name, err)
	}
	return id
}

// product siembra un producto como owner en una categoría.
func (k *catKit) product(t *testing.T, company, category int64, name string, active bool) int64 {
	t.Helper()
	var id int64
	if err := k.st.Pool.QueryRow(context.Background(),
		`insert into products (company_id, name, category_id, price, is_active) values ($1, $2, $3, 10, $4) returning id`,
		company, name, category, active).Scan(&id); err != nil {
		t.Fatalf("producto %s: %v", name, err)
	}
	return id
}

// LA LISTA CON INACTIVAS (FR-017): sin parámetro sigue como la pantalla la conoce; con
// includeInactive trae las apagadas marcadas y la cuenta de productos inactivos.
func TestCategoryListIncludesInactiveOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	k := newCatKit(t)
	off := k.cat(t, defaultCompanyID, "Desayunos 099", nil, false)
	k.product(t, defaultCompanyID, off, "Molletes 099", false)
	k.product(t, defaultCompanyID, off, "Chilaquiles 099", false)

	active, err := k.svc.CategoryList(k.ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range active {
		if c.ID == off {
			t.Fatal("la lista de la pantalla trajo una categoría apagada")
		}
	}
	all, err := k.svc.CategoryList(k.ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range all {
		if c.ID == off {
			found = true
			if c.IsActive || c.ProductCount != 0 || c.InactiveProductCount != 2 {
				t.Errorf("la apagada salió como %+v; quiere inactiva, 0 activos y 2 inactivos", c)
			}
		}
	}
	if !found {
		t.Fatal("con includeInactive no salió la categoría apagada: no habría forma de reactivarla")
	}
}

// LA LISTA CON INACTIVAS NO VE CATEGORÍAS AJENAS en los tres casos donde RLS ya falló.
func TestCategoryListWithInactiveIsIsolated(t *testing.T) {
	t.Parallel()
	k := newCatKit(t)
	other := makeCompany(t, k.st, "cat-lista-ajena")
	mine := k.cat(t, defaultCompanyID, "Propia apagada", nil, false)
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		cats, err := app.NewAdminService(st).CategoryList(ctx, true)
		if err != nil {
			return
		}
		for _, c := range cats {
			if c.ID == mine {
				t.Error("se vio una categoría de otra empresa")
			}
		}
	})
}

func (k *catKit) catalogVersion(t *testing.T) int64 {
	t.Helper()
	var v int64
	if err := k.st.Pool.QueryRow(context.Background(),
		`select coalesce((select version from catalog_versions where company_id = $1), 0)`, defaultCompanyID).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (k *catKit) row(t *testing.T, id int64) (name string, parent *int64, active bool) {
	t.Helper()
	if err := k.st.Pool.QueryRow(context.Background(),
		`select name::text, parent_id, is_active from categories where id = $1`, id).Scan(&name, &parent, &active); err != nil {
		t.Fatal(err)
	}
	return name, parent, active
}

func catAuditCount(t *testing.T, st *store.Store, action string, entity int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from audit_events where action = $1 and entity_id = $2`, action, entity).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// APAGAR UNA CATEGORÍA SIN PRODUCTOS ACTIVOS (el caso real del dueño): deja de salir en el menú,
// sube la versión del catálogo y la bitácora dice qué cambió.
func TestUpdateCategoryTurnsOffAnEmptyCategory(t *testing.T) {
	t.Parallel()
	k := newCatKit(t)
	c := k.cat(t, defaultCompanyID, "Desayunos", nil, true)
	k.product(t, defaultCompanyID, c, "Molletes", false)
	v0 := k.catalogVersion(t)

	got, err := k.svc.UpdateCategory(k.ctx, c, app.CategoryPatch{Active: new(false)}, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsActive {
		t.Fatalf("respuesta = %+v; quiere inactiva", got)
	}
	menu, err := k.appSt.QC(k.ctx).MenuCategories(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range menu {
		if m.ID == c {
			t.Fatal("la categoría apagada sigue en el menú de la venta")
		}
	}
	if k.catalogVersion(t) <= v0 {
		t.Error("no subió la versión del catálogo: las tabletas seguirían pintando la categoría")
	}
	before, after := lastAudit(t, k.st, "category.updated", c)
	if !strings.Contains(before, `"isActive": true`) || !strings.Contains(after, `"isActive": false`) || strings.Contains(after, "name") {
		t.Fatalf("bitácora %s → %s; quiere solo isActive de true a false", before, after)
	}
}

func TestUpdateCategoryRules(t *testing.T) {
	t.Parallel()
	k := newCatKit(t)
	bebidas := k.cat(t, defaultCompanyID, "Bebidas", nil, true)
	frias := k.cat(t, defaultCompanyID, "Frías", &bebidas, true)
	calientes := k.cat(t, defaultCompanyID, "Calientes", &bebidas, true)
	k.product(t, defaultCompanyID, frias, "Limonada", true)
	apagada := k.cat(t, defaultCompanyID, "Apagada", nil, false)
	hijaApagada := k.cat(t, defaultCompanyID, "Hija apagada", &apagada, false)
	postres := k.cat(t, defaultCompanyID, "Postres", nil, true)

	cases := []struct {
		name  string
		id    int64
		patch app.CategoryPatch
		want  error
		msg   string
	}{
		{"con productos activos no se apaga", frias, app.CategoryPatch{Active: new(false)}, domain.ErrCategoryHasActiveProducts, "1 productos activos"},
		{"con subcategorías activas no se apaga", bebidas, app.CategoryPatch{Active: new(false)}, domain.ErrCategoryHasActiveChildren, "Calientes"},
		{"no se reactiva bajo un padre apagado", hijaApagada, app.CategoryPatch{Active: new(true)}, domain.ErrCategoryParentInvalid, "inactiva"},
		{"un nombre de hermana, con otra capitalización", calientes, app.CategoryPatch{Name: new("FRÍAS")}, domain.ErrCategoryNameTaken, "id "},
		{"una raíz con subcategorías no se mueve", bebidas, app.CategoryPatch{ParentID: &postres, SetParent: true}, domain.ErrCategoryParentInvalid, "subcategorías"},
		{"no bajo una subcategoría", postres, app.CategoryPatch{ParentID: &frias, SetParent: true}, domain.ErrCategoryParentInvalid, "raíz"},
		{"padre que no existe", postres, app.CategoryPatch{ParentID: new(int64(99999)), SetParent: true}, domain.ErrNotFound, ""},
		{"categoría que no existe", 99999, app.CategoryPatch{Active: new(false)}, domain.ErrNotFound, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := k.svc.UpdateCategory(k.ctx, c.id, c.patch, k.user)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v; quiere %v", err, c.want)
			}
			if c.msg != "" && !strings.Contains(err.Error(), c.msg) {
				t.Errorf("el mensaje %q no dice %q", err, c.msg)
			}
		})
	}
	if n := catAuditCount(t, k.st, "category.updated", bebidas) + catAuditCount(t, k.st, "category.updated", frias); n != 0 {
		t.Fatalf("los rechazos dejaron %d renglones en la bitácora", n)
	}

	t.Run("mover bajo una raíz activa y volver raíz", func(t *testing.T) {
		if _, err := k.svc.UpdateCategory(k.ctx, postres, app.CategoryPatch{ParentID: &bebidas, SetParent: true}, k.user); err != nil {
			t.Fatal(err)
		}
		if _, parent, _ := k.row(t, postres); parent == nil || *parent != bebidas {
			t.Fatalf("padre = %v; quiere %d", parent, bebidas)
		}
		if _, err := k.svc.UpdateCategory(k.ctx, postres, app.CategoryPatch{SetParent: true}, k.user); err != nil {
			t.Fatal(err)
		}
		if _, parent, _ := k.row(t, postres); parent != nil {
			t.Fatalf("padre = %v; quiere raíz", *parent)
		}
	})
	t.Run("renombrar a su mismo nombre no choca consigo misma ni deja bitácora", func(t *testing.T) {
		n0 := catAuditCount(t, k.st, "category.updated", calientes)
		if _, err := k.svc.UpdateCategory(k.ctx, calientes, app.CategoryPatch{Name: new("  Calientes ")}, k.user); err != nil {
			t.Fatal(err)
		}
		if catAuditCount(t, k.st, "category.updated", calientes) != n0 {
			t.Fatal("un cambio que no cambia nada dejó renglón en la bitácora")
		}
	})
	t.Run("reactivar el padre y luego la hija", func(t *testing.T) {
		if _, err := k.svc.UpdateCategory(k.ctx, apagada, app.CategoryPatch{Active: new(true)}, k.user); err != nil {
			t.Fatal(err)
		}
		if _, err := k.svc.UpdateCategory(k.ctx, hijaApagada, app.CategoryPatch{Active: new(true)}, k.user); err != nil {
			t.Fatal(err)
		}
	})
}

// DOS MOVIMIENTOS CRUZADOS a la vez (A bajo B y B bajo A): con candados en orden distinto daban
// deadlock (40P01) y el agente recibía un 500. Con el candado en orden de id, uno se aplica y el
// otro se rechaza por la regla.
func TestCrossedCategoryMovesDoNotDeadlock(t *testing.T) {
	t.Parallel()
	k := newCatKit(t)
	for i := range 5 {
		a := k.cat(t, defaultCompanyID, "A"+strings.Repeat("a", i), nil, true)
		b := k.cat(t, defaultCompanyID, "B"+strings.Repeat("b", i), nil, true)
		errs := make(chan error, 2)
		move := func(id, parent int64) {
			ctx, release, err := k.appSt.AcquireTenant(context.Background(), defaultCompanyID)
			if err != nil {
				errs <- err
				return
			}
			defer release()
			_, err = k.svc.UpdateCategory(ctx, id, app.CategoryPatch{ParentID: &parent, SetParent: true}, k.user)
			errs <- err
		}
		go move(a, b)
		go move(b, a)
		var ok, invalid int
		for range 2 {
			switch err := <-errs; {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrCategoryParentInvalid):
				invalid++
			default:
				t.Fatalf("vuelta %d: err = %v; quiere aplicado o padre inválido, nunca un deadlock", i, err)
			}
		}
		if ok != 1 || invalid != 1 {
			t.Fatalf("vuelta %d: %d aplicados y %d rechazados; quiere 1 y 1", i, ok, invalid)
		}
	}
}

// EDITAR NO ALCANZA CATEGORÍAS AJENAS, ni como blanco ni como padre, en los tres casos.
func TestUpdateCategoryIsIsolated(t *testing.T) {
	t.Parallel()
	k := newCatKit(t)
	other := makeCompany(t, k.st, "cat-edicion-ajena")
	mine := k.cat(t, defaultCompanyID, "Mía", nil, true)
	theirs := k.cat(t, other, "Ajena", nil, true)
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewAdminService(st)
		if _, err := svc.UpdateCategory(ctx, mine, app.CategoryPatch{Active: new(false)}, k.user); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("apagar una categoría ajena = %v; quiere ErrNotFound", err)
		}
	})
	// Como la dueña, colgar una propia de una ajena: la llave foránea de parent_id es de una columna
	// y la aceptaría; el servicio no.
	if _, err := k.svc.UpdateCategory(k.ctx, mine, app.CategoryPatch{ParentID: &theirs, SetParent: true}, k.user); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("padre de otra empresa = %v; quiere ErrNotFound", err)
	}
	if _, parent, active := k.row(t, mine); parent != nil || !active {
		t.Fatalf("la categoría propia cambió: padre %v, activa %v", parent, active)
	}
}

func TestCreateCategory(t *testing.T) {
	t.Parallel()
	k := newCatKit(t)
	bebidas := k.cat(t, defaultCompanyID, "Bebidas", nil, true)
	frias := k.cat(t, defaultCompanyID, "Frías", &bebidas, true)
	apagada := k.cat(t, defaultCompanyID, "Apagada", nil, false)
	viejo := k.cat(t, defaultCompanyID, "Tés", &bebidas, false)

	root, err := k.svc.CreateCategory(k.ctx, "  Postres  ", nil, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if name, parent, active := k.row(t, root.ID); name != "Postres" || parent != nil || !active {
		t.Fatalf("raíz = %q %v %v", name, parent, active)
	}
	if _, after := lastAudit(t, k.st, "category.created", root.ID); !strings.Contains(after, "Postres") {
		t.Fatalf("bitácora del alta = %s", after)
	}
	sub, err := k.svc.CreateCategory(k.ctx, "Smoothies", &bebidas, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, parent, _ := k.row(t, sub.ID); parent == nil || *parent != bebidas {
		t.Fatalf("padre = %v", parent)
	}

	cases := []struct {
		name   string
		cat    string
		parent *int64
		want   error
		msg    string
	}{
		{"bajo una subcategoría", "Con hielo", &frias, domain.ErrCategoryParentInvalid, "raíz"},
		{"bajo una raíz apagada", "Molletes", &apagada, domain.ErrCategoryParentInvalid, "inactiva"},
		{"nombre repetido, inactiva y con otra capitalización", "TÉS", &bebidas, domain.ErrCategoryNameTaken, "inactiva"},
		{"nombre vacío", "  ", nil, domain.ErrValidation, ""},
		{"padre que no existe", "X", new(int64(99999)), domain.ErrNotFound, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := k.svc.CreateCategory(k.ctx, c.cat, c.parent, k.user)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v; quiere %v", err, c.want)
			}
			if c.msg != "" && !strings.Contains(err.Error(), c.msg) {
				t.Errorf("el mensaje %q no dice %q", err, c.msg)
			}
		})
	}
	if !strings.Contains(func() string {
		_, err := k.svc.CreateCategory(k.ctx, "tés", &bebidas, k.user)
		return err.Error()
	}(), "id "+strconv.FormatInt(viejo, 10)) {
		t.Error("el rechazo por nombre no dice el id de la que ya existe")
	}
}

// DOS EMPRESAS PUEDEN TENER SU PROPIA «Bebidas» raíz, y un padre ajeno no existe para el alta.
func TestCreateCategoryIsIsolated(t *testing.T) {
	t.Parallel()
	k := newCatKit(t)
	other := makeCompany(t, k.st, "cat-alta-ajena")
	theirs := k.cat(t, other, "Bebidas", nil, true)
	if _, err := k.svc.CreateCategory(k.ctx, "Bebidas", nil, k.user); err != nil {
		t.Fatalf("una raíz con el mismo nombre que la de otra empresa = %v; deben convivir", err)
	}
	mine := k.cat(t, defaultCompanyID, "Raíz propia", nil, true)
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		if _, err := app.NewAdminService(st).CreateCategory(ctx, "Hija", &mine, k.user); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("alta bajo un padre ajeno = %v; quiere ErrNotFound", err)
		}
	})
	if _, err := k.svc.CreateCategory(k.ctx, "Hija", &theirs, k.user); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("alta bajo el padre de otra empresa = %v; quiere ErrNotFound", err)
	}
}
