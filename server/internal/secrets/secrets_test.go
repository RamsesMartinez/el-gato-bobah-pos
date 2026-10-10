package secrets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func testLocal(t *testing.T, b byte) *Local {
	t.Helper()
	l, err := NewLocal(testKey(b))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// UN SECRETO COPIADO A LA FILA DE OTRA EMPRESA NO SE DESCIFRA. Es la segunda barrera además de
// RLS: si alguien mueve bytes entre filas (un UPDATE a mano, un restore mezclado), el secreto de la
// empresa A no le sirve a la B.
func TestLocalRejectsAnotherCompanysAAD(t *testing.T) {
	l := testLocal(t, 1)
	ctx := context.Background()
	c, err := l.Encrypt(ctx, []byte("secreto-de-la-2"), "credencial|2|6|client_secret")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := l.Decrypt(ctx, c, "credencial|2|6|client_secret"); err != nil || string(got) != "secreto-de-la-2" {
		t.Fatalf("con su propia AAD debía descifrar: %q, %v", got, err)
	}
	if _, err := l.Decrypt(ctx, c, "credencial|3|6|client_secret"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("con la AAD de otra empresa debía ser ErrUnreadable, fue %v", err)
	}
}

// UN RESPALDO RESTAURADO EN OTRO AMBIENTE NO DESCIFRA, Y ESO ES ErrUnreadable, NO UN 500. Es la
// protección funcionando: la pantalla tiene que decir «vuelve a capturar», no «error del server».
func TestAnotherEnvironmentsValueIsUnreadableNotAnError(t *testing.T) {
	ctx := context.Background()
	fromAnotherMachine, _ := testLocal(t, 1).Encrypt(ctx, []byte("x-secreto-largo"), "a")
	if _, err := testLocal(t, 2).Decrypt(ctx, fromAnotherMachine, "a"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("otra llave local: esperaba ErrUnreadable, fue %v", err)
	}
	// Lo que cifró KMS (producción) restaurado donde se descifra con la llave local (desarrollo).
	fromKMS := append([]byte{formatKMS}, []byte("bytes-que-solo-kms-entiende")...)
	if _, err := testLocal(t, 1).Decrypt(ctx, fromKMS, "a"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("un ciphertext de KMS ante el respaldo local: esperaba ErrUnreadable, fue %v", err)
	}
	for _, garbage := range [][]byte{nil, {}, {formatLocal}, {formatLocal, 1, 2, 3}} {
		if _, err := testLocal(t, 1).Decrypt(ctx, garbage, "a"); !errors.Is(err, ErrUnreadable) {
			t.Fatalf("%v: esperaba ErrUnreadable, fue %v", garbage, err)
		}
	}
}

// Dos cifrados del mismo valor salen distintos (nonce aleatorio). Por eso la migración ya no puede
// comparar dos llaves en un `check`, y el servicio compara en plaintext: este test es el porqué.
func TestTwoEncryptionsOfTheSameValueDiffer(t *testing.T) {
	l := testLocal(t, 1)
	a, _ := l.Encrypt(context.Background(), []byte("igual-igual-igual"), "x")
	b, _ := l.Encrypt(context.Background(), []byte("igual-igual-igual"), "x")
	if bytes.Equal(a, b) {
		t.Fatal("dos cifrados iguales: el nonce no es aleatorio y el ciphertext filtra qué valores se repiten")
	}
}

func TestLocalKeyMustBe32Bytes(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33} {
		if _, err := NewLocal(make([]byte, n)); err == nil {
			t.Fatalf("una llave de %d bytes debía rechazarse", n)
		}
	}
}

// --- KMS contra un server falso ---

// fakeKMS imita el metadata server y la API de KMS. Cifra "de juguete" (prefijo + AAD) para poder
// comprobar que la AAD viaja en las dos direcciones.
type fakeKMS struct {
	tokensIssued  atomic.Int32
	tokenLifetime int
	encryptStatus int
	decryptStatus int
	lastPath      string
	authorization string
}

func (f *fakeKMS) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/service-accounts/default/token"):
			if r.Header.Get("Metadata-Flavor") != "Google" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			n := f.tokensIssued.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "tok-" + string(rune('0'+n)), "expires_in": f.tokenLifetime, "token_type": "Bearer",
			})
		case strings.HasSuffix(r.URL.Path, ":encrypt"):
			f.lastPath, f.authorization = r.URL.Path, r.Header.Get("Authorization")
			if f.encryptStatus != 0 {
				w.WriteHeader(f.encryptStatus)
				return
			}
			var in struct{ Plaintext, AdditionalAuthenticatedData string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			ct := base64.StdEncoding.EncodeToString([]byte(in.AdditionalAuthenticatedData + "|" + in.Plaintext))
			_ = json.NewEncoder(w).Encode(map[string]string{"ciphertext": ct})
		case strings.HasSuffix(r.URL.Path, ":decrypt"):
			f.lastPath, f.authorization = r.URL.Path, r.Header.Get("Authorization")
			if f.decryptStatus != 0 {
				w.WriteHeader(f.decryptStatus)
				return
			}
			var in struct{ Ciphertext, AdditionalAuthenticatedData string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			raw, _ := base64.StdEncoding.DecodeString(in.Ciphertext)
			aad, pt, ok := strings.Cut(string(raw), "|")
			if !ok || aad != in.AdditionalAuthenticatedData {
				// Lo que responde KMS de verdad ante AAD distinta o ciphertext ajeno.
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"plaintext": pt})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

const testKMSKey = "projects/p/locations/us-central1/keyRings/pos-dev/cryptoKeys/credenciales"

func kmsAgainst(srv *httptest.Server, now func() time.Time) *KMS {
	k := NewKMS(testKMSKey)
	k.metadataURL, k.apiURL, k.now = srv.URL, srv.URL, now
	return k
}

func TestKMSEncryptsAndDecryptsBindingTheAAD(t *testing.T) {
	f := &fakeKMS{tokenLifetime: 3600}
	srv := f.server(t)
	defer srv.Close()
	k := kmsAgainst(srv, time.Now)
	ctx := context.Background()

	c, err := k.Encrypt(ctx, []byte("el-secreto"), "credencial|2|6|client_secret")
	if err != nil {
		t.Fatal(err)
	}
	if c[0] != formatKMS {
		t.Fatalf("el ciphertext debe marcar que lo hizo KMS (primer byte %d)", c[0])
	}
	if f.lastPath != "/v1/"+testKMSKey+":encrypt" || f.authorization != "Bearer tok-1" {
		t.Fatalf("petición a %q con %q", f.lastPath, f.authorization)
	}
	if got, err := k.Decrypt(ctx, c, "credencial|2|6|client_secret"); err != nil || string(got) != "el-secreto" {
		t.Fatalf("descifrar: %q, %v", got, err)
	}
	if _, err := k.Decrypt(ctx, c, "credencial|3|6|client_secret"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("otra empresa en la AAD: esperaba ErrUnreadable, fue %v", err)
	}
	if n := f.tokensIssued.Load(); n != 1 {
		t.Fatalf("pidió %d tokens para tres operaciones: el token del metadata server se reusa", n)
	}
}

// El token del metadata server dura una hora. Un proceso que vive semanas no puede quedarse con el
// primero: se renueva ANTES de vencer, no después del 401.
func TestKMSRenewsTheTokenBeforeItExpires(t *testing.T) {
	f := &fakeKMS{tokenLifetime: 3600}
	srv := f.server(t)
	defer srv.Close()
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	k := kmsAgainst(srv, func() time.Time { return now })

	_, _ = k.Encrypt(context.Background(), []byte("a"), "x")
	now = now.Add(50 * time.Minute)
	_, _ = k.Encrypt(context.Background(), []byte("a"), "x")
	if n := f.tokensIssued.Load(); n != 1 {
		t.Fatalf("a los 50 min todavía sirve el primero; pidió %d", n)
	}
	now = now.Add(6 * time.Minute) // 56 min: dentro del margen de renovación
	_, _ = k.Encrypt(context.Background(), []byte("a"), "x")
	if n := f.tokensIssued.Load(); n != 2 {
		t.Fatalf("a los 56 min debía renovarlo; lleva %d tokens", n)
	}
}

// KMS caído o sin permisos NO es «ilegible»: eso mandaría a recapturar una credencial que está bien.
func TestKMSDownIsNotMistakenForUnreadable(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusForbidden, http.StatusTooManyRequests} {
		f := &fakeKMS{tokenLifetime: 3600, decryptStatus: status}
		srv := f.server(t)
		k := kmsAgainst(srv, time.Now)
		c := append([]byte{formatKMS}, []byte("x|y")...)
		_, err := k.Decrypt(context.Background(), c, "x")
		srv.Close()
		if !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrUnreadable) {
			t.Fatalf("HTTP %d: esperaba ErrUnavailable, fue %v", status, err)
		}
	}
	// Y lo que cifró el respaldo local (desarrollo) llegando a KMS sí es ilegible, sin viaje de red.
	f := &fakeKMS{tokenLifetime: 3600}
	srv := f.server(t)
	defer srv.Close()
	local, _ := testLocal(t, 1).Encrypt(context.Background(), []byte("zz"), "x")
	if _, err := kmsAgainst(srv, time.Now).Decrypt(context.Background(), local, "x"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("un ciphertext local ante KMS: esperaba ErrUnreadable, fue %v", err)
	}
	if f.tokensIssued.Load() != 0 {
		t.Fatal("no hacía falta hablar con Google para saber que ese formato no es suyo")
	}
}

// --- Memoria ---

type countingCipher struct {
	base  *Local
	calls int
	fail  error
}

func (c *countingCipher) Encrypt(ctx context.Context, plaintext []byte, aad string) ([]byte, error) {
	return c.base.Encrypt(ctx, plaintext, aad)
}
func (c *countingCipher) Decrypt(ctx context.Context, ciphertext []byte, aad string) ([]byte, error) {
	c.calls++
	if c.fail != nil {
		return nil, c.fail
	}
	return c.base.Decrypt(ctx, ciphertext, aad)
}

// CADA AVISO DE UBER NO PUEDE SER UNA LLAMADA A GOOGLE: la memoria descifra una vez por vida.
func TestCacheDecryptsOncePerLifetime(t *testing.T) {
	base := &countingCipher{base: testLocal(t, 1)}
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	m := WithCache(base, 15*time.Minute, 24*time.Hour)
	m.now = func() time.Time { return now }
	c, _ := m.Encrypt(context.Background(), []byte("s"), "a")

	for i := 0; i < 5; i++ {
		if got, err := m.Decrypt(context.Background(), c, "a"); err != nil || string(got) != "s" {
			t.Fatal(got, err)
		}
	}
	if base.calls != 0 {
		t.Fatalf("recién ciphertext ya se conoce el plaintext; descifró %d veces", base.calls)
	}
	now = now.Add(16 * time.Minute)
	_, _ = m.Decrypt(context.Background(), c, "a")
	if base.calls != 1 {
		t.Fatalf("vencida la vida, debía descifrar otra vez; calls=%d", base.calls)
	}
}

// GOOGLE CAÍDO NO TUMBA LOS PEDIDOS QUE YA SE PODÍAN VERIFICAR: se sirve la copia vieja. Pero una
// credencial que nunca se descifró no se inventa, y un valor ilegible no se sirve de memoria.
func TestWithGoogleDownOnlyTheStaleCopyIsServed(t *testing.T) {
	base := &countingCipher{base: testLocal(t, 1)}
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	m := WithCache(base, 15*time.Minute, 24*time.Hour)
	m.now = func() time.Time { return now }
	c, _ := base.base.Encrypt(context.Background(), []byte("s"), "a")
	other, _ := base.base.Encrypt(context.Background(), []byte("t"), "a")

	if _, err := m.Decrypt(context.Background(), c, "a"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	base.fail = ErrUnavailable
	if got, err := m.Decrypt(context.Background(), c, "a"); err != nil || string(got) != "s" {
		t.Fatalf("con Google caído debía servir la copia vieja: %q, %v", got, err)
	}
	if _, err := m.Decrypt(context.Background(), other, "a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin copia no hay nada que servir: esperaba ErrUnavailable, fue %v", err)
	}
	base.fail = ErrUnreadable
	if _, err := m.Decrypt(context.Background(), c, "a"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("un valor que ya no descifra no se sirve de memoria: fue %v", err)
	}
}

// La copia de memoria es del par (ciphertext, AAD): el mismo ciphertext pedido con la AAD de otra
// empresa no sale de memoria — si saliera, la memoria sería la puerta que la AAD cierra.
func TestCacheDoesNotBypassTheAAD(t *testing.T) {
	m := WithCache(testLocal(t, 1), 15*time.Minute, 24*time.Hour)
	c, _ := m.Encrypt(context.Background(), []byte("de-la-2"), "credencial|2|6|client_secret")
	if _, err := m.Decrypt(context.Background(), c, "credencial|3|6|client_secret"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("esperaba ErrUnreadable, fue %v", err)
	}
}

// LA COPIA VIEJA TIENE CADUCIDAD. Sin tope, quitarle a la cuenta de servicio el permiso sobre la
// llave —lo primero que se hace ante un incidente— no cortaba nada: el proceso vivo seguía usando
// el secreto en memoria hasta reiniciarse. Con tope, a más tardar a las 24 h del último descifrado
// bueno deja de servirse y falla cerrado.
func TestTheStaleCopyExpiresAfterTheMaxAge(t *testing.T) {
	base := &countingCipher{base: testLocal(t, 1)}
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	c := WithCache(base, 15*time.Minute, 24*time.Hour)
	c.now = func() time.Time { return now }
	ct, _ := base.base.Encrypt(context.Background(), []byte("s"), "a")
	if _, err := c.Decrypt(context.Background(), ct, "a"); err != nil {
		t.Fatal(err)
	}

	base.fail = ErrUnavailable
	now = now.Add(23 * time.Hour)
	if got, err := c.Decrypt(context.Background(), ct, "a"); err != nil || string(got) != "s" {
		t.Fatalf("a las 23 h con Google caído todavía se sirve la copia: %q, %v", got, err)
	}
	now = now.Add(2 * time.Hour) // 25 h desde el último descifrado bueno
	if _, err := c.Decrypt(context.Background(), ct, "a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a las 25 h la copia vieja se siguió sirviendo: %v", err)
	}
}
