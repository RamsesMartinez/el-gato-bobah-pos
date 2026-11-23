package objstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// El ejemplo «GET Object» de la documentación de AWS para SigV4 con cabecera (Signature
// Calculations for the Authorization Header: Transferring Payload in a Single Chunk). Si la firma
// propia difiere en un solo byte de la de AWS, R2 responde 403 a todo y nada en local lo diría.
func TestSignMatchesTheAWSPublishedExample(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	emptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	at := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	sign(req, emptyHash, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", at)

	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request," +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date," +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("la firma no es la del ejemplo de AWS:\n got  %s\n want %s", got, want)
	}
}

func TestPutSendsWhatR2Needs(t *testing.T) {
	body := []byte("RIFF....WEBP")
	var seen *http.Request
	var seenBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		seenBody, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	c, err := New(srv.URL, "media", "auto", "AK", "SECRET-NO-DEBE-SALIR")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put(context.Background(), "2/products/15/abc-1200.webp", body, "image/webp", "public, max-age=31536000, immutable"); err != nil {
		t.Fatal(err)
	}
	if seen.Method != http.MethodPut || seen.URL.Path != "/media/2/products/15/abc-1200.webp" {
		t.Fatalf("%s %s: el objeto debe ir en el bucket por ruta", seen.Method, seen.URL.Path)
	}
	sum := sha256.Sum256(body)
	checks := map[string]string{
		"Content-Type":         "image/webp",
		"Cache-Control":        "public, max-age=31536000, immutable",
		"X-Amz-Content-Sha256": hex.EncodeToString(sum[:]),
	}
	for h, want := range checks {
		if got := seen.Header.Get(h); got != want {
			t.Errorf("%s = %q, quería %q", h, got, want)
		}
	}
	auth := seen.Header.Get("Authorization")
	if !strings.Contains(auth, "/auto/s3/aws4_request,") ||
		!strings.Contains(auth, "SignedHeaders=cache-control;content-type;host;x-amz-content-sha256;x-amz-date,") {
		t.Errorf("Authorization = %q: Content-Type y Cache-Control deben ir firmados, o un intermediario los cambia", auth)
	}
	if string(seenBody) != string(body) {
		t.Errorf("cuerpo = %q", seenBody)
	}
}

func TestErrorsDoNotLeakTheSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<Error><Code>SignatureDoesNotMatch</Code><Message>x</Message></Error>`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "media", "auto", "AK", "SECRET-NO-DEBE-SALIR")
	err := c.Put(context.Background(), "k", []byte("x"), "image/webp", "")
	if err == nil {
		t.Fatal("un 403 debe ser error")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("el error lleva el secreto: %v", err)
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
		t.Errorf("el error %q no dice el estado ni el código: no se puede diagnosticar", err)
	}
}

func TestDeleteTreatsMissingAsDone(t *testing.T) {
	status := http.StatusNotFound
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/media/k" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "media", "auto", "AK", "S")
	if err := c.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("borrar lo que ya no existe debe contar como hecho: %v", err)
	}
	status = http.StatusNoContent
	if err := c.Delete(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
	status = http.StatusInternalServerError
	if err := c.Delete(context.Background(), "k"); err == nil {
		t.Fatal("un 500 debe ser error: el borrado queda pendiente")
	}
}

func TestNewRejectsABadEndpoint(t *testing.T) {
	for _, e := range []string{"", "nada", "ftp://x", "https://"} {
		if _, err := New(e, "b", "auto", "a", "s"); err == nil {
			t.Errorf("endpoint %q aceptado", e)
		}
	}
}

// TestAgainstARealS3 corre solo con OBJSTORE_TEST_ENDPOINT (el versitygw de `make start`): es el
// único que verifica la firma de verdad. Sube, lee de forma anónima y borra.
func TestAgainstARealS3(t *testing.T) {
	ep := os.Getenv("OBJSTORE_TEST_ENDPOINT")
	if ep == "" {
		t.Skip("sin OBJSTORE_TEST_ENDPOINT")
	}
	c, err := New(ep, "media", "us-east-1", "gatobobah-dev", "gatobobah-dev-secret")
	if err != nil {
		t.Fatal(err)
	}
	key := "0/products/0/prueba-objstore.webp"
	if err := c.Put(context.Background(), key, []byte("hola"), "image/webp", "public, max-age=31536000, immutable"); err != nil {
		t.Fatalf("PUT firmado rechazado por un S3 real: %v", err)
	}
	resp, err := http.Get(strings.TrimRight(ep, "/") + "/media/" + key)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("GET anónimo: %d, Cache-Control %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	if err := c.Delete(context.Background(), key); err != nil {
		t.Fatalf("DELETE firmado: %v", err)
	}
}
