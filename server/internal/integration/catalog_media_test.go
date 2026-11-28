//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/objstore"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

const mediaBase = "https://media.test"

// fakeS3 es un bucket en memoria detrás de HTTP. Lo que aquí se prueba es la orquestación (qué se
// sube, qué se borra y cuándo); la firma la prueban objstore y el S3 real de `make start`.
type fakeS3 struct {
	mu         sync.Mutex
	objects    map[string][]byte
	headers    map[string]http.Header
	failPut    bool
	failLarge  bool // falla solo la variante de 1200: la de 400 ya entró
	failDelete bool
	// onDelete corre al recibir un DELETE: simula a quien cierra la tableta justo después del commit.
	onDelete func()
}

func newFakeS3(t *testing.T) (*fakeS3, *objstore.Client) {
	t.Helper()
	f := &fakeS3{objects: map[string][]byte{}, headers: map[string]http.Header{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		key := strings.TrimPrefix(r.URL.Path, "/media/")
		switch r.Method {
		case http.MethodPut:
			if f.failPut || (f.failLarge && strings.HasSuffix(key, "-1200.webp")) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			b, _ := io.ReadAll(r.Body)
			f.objects[key] = b
			f.headers[key] = r.Header.Clone()
		case http.MethodDelete:
			if f.onDelete != nil {
				f.onDelete()
			}
			if f.failDelete {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			delete(f.objects, key)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := objstore.New(srv.URL, "media", "auto", "AK", "SK")
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}

func (f *fakeS3) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok
}

func (f *fakeS3) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}

func (f *fakeS3) set(fn func(*fakeS3)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// webpOf arma un WebP VP8L de side×side; seed cambia el contenido (y con él la huella).
func webpOf(side int, seed byte) []byte {
	payload := make([]byte, 5)
	payload[0] = 0x2f
	binary.LittleEndian.PutUint32(payload[1:], uint32(side-1)|uint32(side-1)<<14)
	payload = append(payload, bytes.Repeat([]byte{seed}, 16)...)
	chunk := append([]byte("VP8L"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(chunk[4:], uint32(len(payload)))
	chunk = append(chunk, payload...)
	out := append([]byte("RIFF"), make([]byte, 4)...)
	out = append(out, "WEBP"...)
	out = append(out, chunk...)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}

func photo(seed byte) app.ImageUpload {
	return app.ImageUpload{Large: webpOf(1200, seed), Small: webpOf(400, seed)}
}

type mediaKit struct {
	st      *store.Store
	appSt   *store.Store
	s3      *fakeS3
	svc     *app.CatalogMediaService
	ctx     context.Context
	user    int64
	product int64
	cat     int64
}

func newMediaKit(t *testing.T) *mediaKit {
	t.Helper()
	k := &mediaKit{st: newTestStore(t)}
	k.appSt = appRoleStore(t)
	var c *objstore.Client
	k.s3, c = newFakeS3(t)
	k.svc = app.NewCatalogMediaService(k.appSt, c, mediaBase)
	k.user = makeUser(t, k.st, "foto_admin", "admin")
	k.product = makeProduct(t, k.st, "Taro con perlas", pesos("65"), false)
	if err := k.st.Pool.QueryRow(context.Background(), `select category_id from products where id = $1`, k.product).Scan(&k.cat); err != nil {
		t.Fatal(err)
	}
	ctx, release, err := k.appSt.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	k.ctx = ctx
	return k
}

func (k *mediaKit) imageURL(t *testing.T, table string, id int64) string {
	t.Helper()
	var u *string
	if err := k.st.Pool.QueryRow(context.Background(), `select image_url from `+table+` where id = $1`, id).Scan(&u); err != nil {
		t.Fatal(err)
	}
	if u == nil {
		return ""
	}
	return *u
}

func keysOf(t *testing.T, url string, kind domain.MediaKind, id int64) []string {
	t.Helper()
	keys, ok := domain.MediaKeysFromURL(mediaBase, url, defaultCompanyID, kind, id)
	if !ok {
		t.Fatalf("%q no es una URL de esta API", url)
	}
	return keys
}

func (k *mediaKit) pending(t *testing.T) int {
	t.Helper()
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from media_pending_deletions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (k *mediaKit) audits(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = $1`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// PONER, REEMPLAZAR Y QUITAR LA FOTO DE UN PRODUCTO (US1).
func TestProductPhotoLifecycle(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)

	url, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user)
	if err != nil {
		t.Fatalf("poner: %v", err)
	}
	if got := k.imageURL(t, "products", k.product); got != url || !strings.HasSuffix(url, "-1200.webp") {
		t.Fatalf("image_url = %q, devolvió %q", got, url)
	}
	first := keysOf(t, url, domain.MediaProducts, k.product)
	for _, key := range first {
		if !k.s3.has(key) {
			t.Fatalf("no se subió %s", key)
		}
		h := k.s3.headers[key]
		if h.Get("Content-Type") != "image/webp" || h.Get("Cache-Control") != domain.MediaCacheControl {
			t.Errorf("%s se subió con %q / %q: se serviría con lo que diga el bucket", key, h.Get("Content-Type"), h.Get("Cache-Control"))
		}
	}
	if k.audits(t, "product.image_set") != 1 {
		t.Error("poner la foto no quedó en la bitácora")
	}

	t.Run("la misma foto otra vez no borra nada", func(t *testing.T) {
		again, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user)
		if err != nil || again != url {
			t.Fatalf("%q %v", again, err)
		}
		for _, key := range first {
			if !k.s3.has(key) {
				t.Fatalf("subir la misma foto borró %s: el producto se quedó sin foto", key)
			}
		}
	})

	t.Run("reemplazar borra la anterior", func(t *testing.T) {
		next, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(2), k.user)
		if err != nil || next == url {
			t.Fatalf("%q %v", next, err)
		}
		for _, key := range first {
			if k.s3.has(key) {
				t.Errorf("la foto anterior %s quedó en el bucket", key)
			}
		}
		if k.s3.count() != 2 {
			t.Errorf("quedaron %d archivos, quería 2 (las dos variantes de la nueva)", k.s3.count())
		}
	})

	t.Run("quitar la deja sin foto y sin archivos", func(t *testing.T) {
		if err := k.svc.RemoveImage(k.ctx, domain.MediaProducts, k.product, k.user); err != nil {
			t.Fatal(err)
		}
		if got := k.imageURL(t, "products", k.product); got != "" {
			t.Fatalf("image_url = %q", got)
		}
		if k.s3.count() != 0 {
			t.Errorf("quedaron %d archivos", k.s3.count())
		}
		if k.audits(t, "product.image_removed") != 1 {
			t.Error("quitar no quedó en la bitácora")
		}
		// Quitar otra vez no falla: no hay nada que quitar.
		if err := k.svc.RemoveImage(k.ctx, domain.MediaProducts, k.product, k.user); err != nil {
			t.Fatalf("quitar dos veces: %v", err)
		}
	})
}

// UN BORRADO QUE FALLA NO REGRESA ERROR: queda pendiente y la siguiente escritura lo reintenta.
func TestFailedDeleteIsRetriedLater(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	url, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user)
	if err != nil {
		t.Fatal(err)
	}
	old := keysOf(t, url, domain.MediaProducts, k.product)

	k.s3.set(func(f *fakeS3) { f.failDelete = true })
	if _, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(2), k.user); err != nil {
		t.Fatalf("un borrado fallido devolvió error a quien subió (la foto nueva sí quedó): %v", err)
	}
	if n := k.pending(t); n != 2 {
		t.Fatalf("pendientes = %d, quería las 2 variantes viejas", n)
	}

	k.s3.set(func(f *fakeS3) { f.failDelete = false })
	if _, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(3), k.user); err != nil {
		t.Fatal(err)
	}
	if n := k.pending(t); n != 0 {
		t.Fatalf("quedan %d pendientes después de una escritura con el bucket sano", n)
	}
	for _, key := range old {
		if k.s3.has(key) {
			t.Errorf("%s sigue en el bucket", key)
		}
	}
}

// UNA SUBIDA QUE NO SE PUEDE GUARDAR NO DEJA ARCHIVOS SUELTOS NI CAMBIA EL PRODUCTO.
func TestFailedSaveLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)

	t.Run("el almacenamiento no responde", func(t *testing.T) {
		k.s3.set(func(f *fakeS3) { f.failPut = true })
		defer k.s3.set(func(f *fakeS3) { f.failPut = false })
		_, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user)
		if !errors.Is(err, domain.ErrMediaUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if k.imageURL(t, "products", k.product) != "" {
			t.Fatal("el producto quedó con una foto que no existe")
		}
	})

	t.Run("la chica entra y la grande falla", func(t *testing.T) {
		k.s3.set(func(f *fakeS3) { f.failLarge = true })
		defer k.s3.set(func(f *fakeS3) { f.failLarge = false })
		_, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(3), k.user)
		if !errors.Is(err, domain.ErrMediaUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if k.s3.count() != 0 {
			t.Fatalf("quedó la variante de 400 huérfana (%d archivos)", k.s3.count())
		}
	})

	t.Run("la base rechaza el guardado", func(t *testing.T) {
		// Una base de medios que no pasa el check de image_url: la transacción falla después de subir.
		s3, c := newFakeS3(t)
		bad := app.NewCatalogMediaService(k.appSt, c, "ftp://medios")
		if _, err := bad.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user); err == nil {
			t.Fatal("se guardó una URL que la base rechaza")
		}
		if s3.count() != 0 {
			t.Fatalf("quedaron %d archivos subidos sin producto que los use", s3.count())
		}
	})
}

// LA GUARDA QUE IMPIDE BORRAR LO QUE LA API NO ARMÓ.
func TestForeignImageURLIsNeverDeleted(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	// Una image_url escrita a mano que apunta a un archivo de OTRO producto del mismo bucket.
	other := makeProduct(t, k.st, "Otro producto", pesos("10"), false)
	otherURL, err := k.svc.SetImage(k.ctx, domain.MediaProducts, other, photo(7), k.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `update products set image_url = $1 where id = $2`, otherURL, k.product); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.RemoveImage(k.ctx, domain.MediaProducts, k.product, k.user); err != nil {
		t.Fatal(err)
	}
	for _, key := range keysOf(t, otherURL, domain.MediaProducts, other) {
		if !k.s3.has(key) {
			t.Fatalf("quitar la foto de un producto borró %s, que es de otro", key)
		}
	}
	if k.pending(t) != 0 {
		t.Error("una URL ajena terminó en pendientes de borrar")
	}
}

// DUPLICAR UN PRODUCTO NO COPIA SU FOTO: quitar la del duplicado borraría la del original.
func TestDuplicateDoesNotCopyThePhoto(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	url, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user)
	if err != nil {
		t.Fatal(err)
	}
	dup, err := app.NewAdminService(k.appSt).DuplicateProduct(k.ctx, k.product, "Taro con perlas (copia)", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.imageURL(t, "products", dup); got != "" {
		t.Fatalf("el duplicado nació con la foto del original: %q", got)
	}
	if err := k.svc.RemoveImage(k.ctx, domain.MediaProducts, dup, k.user); err != nil {
		t.Fatal(err)
	}
	if k.imageURL(t, "products", k.product) != url || !k.s3.has(keysOf(t, url, domain.MediaProducts, k.product)[1]) {
		t.Fatal("la foto del original se perdió")
	}
}

// OTRA EMPRESA NO PONE NI QUITA FOTOS DE UN PRODUCTO AJENO, AUNQUE ADIVINE EL ID.
func TestPhotoOfAnotherCompanysProduct(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	other := makeCompany(t, k.st, "fotos-otra")
	otherUser := makeUserIn(t, k.st, other, "foto_otra", "admin")
	ctx, release, err := k.appSt.AcquireTenant(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	_, err = k.svc.SetImage(ctx, domain.MediaProducts, k.product, photo(1), otherUser)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, quería no encontrado", err)
	}
	if k.s3.count() != 0 {
		t.Fatalf("se subieron %d archivos para un producto ajeno", k.s3.count())
	}
	if err := k.svc.RemoveImage(ctx, domain.MediaCategories, k.cat, otherUser); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("quitar la foto de una categoría ajena: %v", err)
	}
	if err := k.svc.SetCategoryFamily(ctx, k.cat, "coffee", otherUser); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cambiar la familia de una categoría ajena: %v", err)
	}
}

// LO QUE EL SERVIDOR RECHAZA ANTES DE SUBIR NADA.
func TestPhotoValidationAndDisabled(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	bad := []struct {
		name string
		up   app.ImageUpload
		want error
	}{
		{"variantes invertidas", app.ImageUpload{Large: webpOf(400, 1), Small: webpOf(1200, 1)}, domain.ErrImageInvalid},
		{"falta la chica", app.ImageUpload{Large: webpOf(1200, 1)}, domain.ErrImageInvalid},
		{"no es WebP", app.ImageUpload{Large: []byte("\x89PNG\r\n\x1a\nxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"), Small: webpOf(400, 1)}, domain.ErrImageInvalid},
	}
	for _, c := range bad {
		if _, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, c.up, k.user); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
	if k.s3.count() != 0 {
		t.Fatalf("se subieron %d archivos rechazados", k.s3.count())
	}

	off := app.NewCatalogMediaService(k.appSt, nil, "")
	if _, err := off.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user); !errors.Is(err, domain.ErrMediaDisabled) {
		t.Fatalf("sin almacenamiento: %v", err)
	}
}

// LA FAMILIA Y LA FOTO DE UNA CATEGORÍA (US2, US4).
func TestCategoryPhotoAndFamily(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	if err := k.svc.SetCategoryFamily(k.ctx, k.cat, "coffee", k.user); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.SetCategoryFamily(k.ctx, k.cat, "taco", k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("familia desconocida: %v", err)
	}
	var fam *string
	if err := k.st.Pool.QueryRow(context.Background(), `select food_family::text from categories where id = $1`, k.cat).Scan(&fam); err != nil || fam == nil || *fam != "coffee" {
		t.Fatalf("food_family = %v %v", fam, err)
	}
	if k.audits(t, "category.family_set") != 1 {
		t.Error("cambiar la familia no quedó en la bitácora")
	}
	if err := k.svc.SetCategoryFamily(k.ctx, k.cat, "", k.user); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(context.Background(), `select food_family::text from categories where id = $1`, k.cat).Scan(&fam); err != nil || fam != nil {
		t.Fatalf("vacía debe volver a la genérica (null): %v %v", fam, err)
	}

	url, err := k.svc.SetImage(k.ctx, domain.MediaCategories, k.cat, photo(4), k.user)
	if err != nil || !strings.Contains(url, "/categories/") {
		t.Fatalf("%q %v", url, err)
	}
	if err := k.svc.RemoveImage(k.ctx, domain.MediaCategories, k.cat, k.user); err != nil {
		t.Fatal(err)
	}
	if k.s3.count() != 0 || k.audits(t, "category.image_set") != 1 || k.audits(t, "category.image_removed") != 1 {
		t.Fatalf("archivos=%d", k.s3.count())
	}

	cats, err := app.NewAdminService(k.appSt).CategoryList(k.ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range cats {
		if c.ID == k.cat {
			found = true
			if c.ProductCount != 1 || c.FoodFamily != "" || c.ImageURL != nil {
				t.Errorf("categoría en la lista = %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("la categoría no salió en la lista")
	}
}

// LAS LECTURAS DE FOTOS Y PENDIENTES EN LOS TRES CASOS DONDE RLS YA FALLÓ.
func TestCatalogMediaIsIsolated(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	other := makeCompany(t, k.st, "fotos-aislada")
	if _, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(context.Background(),
		`insert into media_pending_deletions (company_id, object_key, last_error) values ($1, $2, 'x')`,
		defaultCompanyID, "1/products/9/0123456789abcdef0123456789abcdef-400.webp"); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		q := st.QC(ctx)
		if _, err := q.LockProductImage(ctx, k.product); err == nil {
			t.Error("se leyó la foto de un producto ajeno")
		}
		rows, err := q.ListMediaPendingDeletions(ctx, 10)
		if err == nil && len(rows) > 0 {
			t.Errorf("se vieron %d pendientes ajenos (el barrido borraría archivos de otra empresa)", len(rows))
		}
		cats, err := q.AdminCategoryList(ctx, true)
		if err == nil {
			for _, c := range cats {
				if c.ID == k.cat {
					t.Error("se vio una categoría ajena")
				}
			}
		}
	})
}

// LA LISTA DEL CATÁLOGO Y EL MENÚ TRAEN LO NECESARIO PARA QUE NINGÚN PRODUCTO SE VEA CON UN HUECO
// (FR-013, FR-017): la foto propia, la de su categoría y la familia, sin otra llamada.
func TestListsCarryEverythingTheFallbackNeeds(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	catURL, err := k.svc.SetImage(k.ctx, domain.MediaCategories, k.cat, photo(5), k.user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.SetCategoryFamily(k.ctx, k.cat, "tea", k.user); err != nil {
		t.Fatal(err)
	}
	page, err := app.NewAdminService(k.appSt).ListProducts(k.ctx, "", "Taro", 0, "", "", "", "", 25, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("%v %d", err, len(page.Items))
	}
	p := page.Items[0]
	if p.ImageURL != nil || p.CategoryImageURL == nil || *p.CategoryImageURL != catURL || p.CategoryFoodFamily != "tea" {
		t.Fatalf("renglón = foto %v, foto de categoría %v, familia %q", p.ImageURL, p.CategoryImageURL, p.CategoryFoodFamily)
	}
	doc, err := app.NewMenuService(k.appSt, nil).Build(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range doc.Categories {
		if c.ID == k.cat && (c.FoodFamily != "tea" || c.ImageURL == nil) {
			t.Fatalf("categoría del menú = %+v", c)
		}
	}
}

// VOLVER A LA FOTO DE ANTES NO LA PIERDE: si el borrado de A quedó pendiente y luego A vuelve a ser la
// foto, el barrido no puede borrar la foto viva.
func TestReturningToAPendingPhotoKeepsIt(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	a, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user)
	if err != nil {
		t.Fatal(err)
	}
	k.s3.set(func(f *fakeS3) { f.failDelete = true })
	if _, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(2), k.user); err != nil {
		t.Fatal(err)
	}
	k.s3.set(func(f *fakeS3) { f.failDelete = false })
	if again, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user); err != nil || again != a {
		t.Fatalf("%q %v", again, err)
	}
	for _, key := range keysOf(t, a, domain.MediaProducts, k.product) {
		if !k.s3.has(key) {
			t.Fatalf("el barrido borró %s, que es la foto que el producto muestra", key)
		}
	}
	if n := k.pending(t); n != 0 {
		t.Fatalf("quedaron %d pendientes", n)
	}
}

// SI LA TABLETA SE VA JUSTO DESPUÉS DE GUARDAR, EL BORRADO QUE FALLE QUEDA ANOTADO: la limpieza no
// depende de que quien subió siga conectado.
func TestCleanupSurvivesTheClientLeaving(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	if _, err := k.svc.SetImage(k.ctx, domain.MediaProducts, k.product, photo(1), k.user); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()
	k.s3.set(func(f *fakeS3) { f.failDelete = true; f.onDelete = cancel })
	if _, err := k.svc.SetImage(ctx, domain.MediaProducts, k.product, photo(2), k.user); err != nil {
		t.Fatal(err)
	}
	if n := k.pending(t); n != 2 {
		t.Fatalf("pendientes = %d: con la petición cancelada el borrado fallido no quedó anotado y el archivo se perdió de vista", n)
	}
}

// UNA SUBCATEGORÍA SIN FOTO NI FAMILIA HEREDA LAS DE SU CATEGORÍA PADRE: quien elige «Bebida fría»
// para «Bebidas frías» espera verla también en «Bebidas frías › Embotelladas».
func TestSubcategoryInheritsItsParentsPhotoAndFamily(t *testing.T) {
	t.Parallel()
	k := newMediaKit(t)
	var sub, prod int64
	if err := k.st.Pool.QueryRow(context.Background(),
		`insert into categories (name, parent_id) values ('Embotelladas', $1) returning id`, k.cat).Scan(&sub); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(context.Background(),
		`insert into products (name, category_id, price) values ('7up de prueba', $1, 20) returning id`, sub).Scan(&prod); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.SetCategoryFamily(k.ctx, k.cat, "cold_drink", k.user); err != nil {
		t.Fatal(err)
	}
	catURL, err := k.svc.SetImage(k.ctx, domain.MediaCategories, k.cat, photo(8), k.user)
	if err != nil {
		t.Fatal(err)
	}
	page, err := app.NewAdminService(k.appSt).ListProducts(k.ctx, "", "7up de prueba", 0, "", "", "", "", 25, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("%v %d", err, len(page.Items))
	}
	p := page.Items[0]
	if p.CategoryFoodFamily != "cold_drink" || p.CategoryImageURL == nil || *p.CategoryImageURL != catURL {
		t.Fatalf("el producto de la subcategoría muestra familia %q y foto %v: no heredó las de su padre", p.CategoryFoodFamily, p.CategoryImageURL)
	}
	// Lo propio de la subcategoría gana sobre lo heredado.
	if err := k.svc.SetCategoryFamily(k.ctx, sub, "soda", k.user); err != nil {
		t.Fatal(err)
	}
	page, _ = app.NewAdminService(k.appSt).ListProducts(k.ctx, "", "7up de prueba", 0, "", "", "", "", 25, 0)
	if page.Items[0].CategoryFoodFamily != "soda" {
		t.Fatalf("familia = %q, quería la propia de la subcategoría", page.Items[0].CategoryFoodFamily)
	}
}
