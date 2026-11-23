// Package objstore habla con un almacenamiento compatible con S3 (Cloudflare R2 en producción,
// versitygw en local) para las fotos del catálogo (spec 091).
//
// Solo hace PUT y DELETE de un objeto, con la firma AWS SigV4 escrita sobre la stdlib: el SDK de
// AWS son ~15 módulos para dos llamadas (constitución VI). La firma se prueba contra el ejemplo
// publicado por AWS y, en local, contra un servidor que la verifica de verdad.
package objstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// Client sube y borra objetos de un bucket.
type Client struct {
	endpoint  *url.URL
	bucket    string
	region    string
	accessKey string
	secretKey string
	http      *http.Client
}

// New arma un cliente. El endpoint es la raíz del servicio (https://<cuenta>.r2.cloudflarestorage.com);
// el bucket va en la ruta, que R2 y versitygw aceptan sin DNS por bucket.
func New(endpoint, bucket, region, accessKey, secretKey string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(endpoint, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("objstore: endpoint inválido; debe ser http(s)://host")
	}
	// Un tope de tiempo propio: sin él, un almacenamiento colgado retiene el request (y su
	// conexión a la base) hasta que el cliente se rinda.
	return &Client{endpoint: u, bucket: bucket, region: region, accessKey: accessKey, secretKey: secretKey,
		http: &http.Client{Timeout: 20 * time.Second}}, nil
}

// Put escribe un objeto completo.
func (c *Client) Put(ctx context.Context, key string, body []byte, contentType, cacheControl string) error {
	req, err := c.request(ctx, http.MethodPut, key, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	if cacheControl != "" {
		req.Header.Set("Cache-Control", cacheControl)
	}
	return c.do(req, body, http.StatusOK)
}

// Delete borra un objeto. Uno que ya no existe cuenta como borrado: es el estado que se buscaba.
func (c *Client) Delete(ctx context.Context, key string) error {
	req, err := c.request(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil, http.StatusNoContent, http.StatusOK, http.StatusNotFound)
}

func (c *Client) request(ctx context.Context, method, key string, body []byte) (*http.Request, error) {
	u := *c.endpoint
	u.Path = "/" + c.bucket + "/" + strings.TrimLeft(key, "/")
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	//nolint:gosec // G704: el host sale de MEDIA_S3_ENDPOINT (config) y la llave la arma el servidor; nada del cliente.
	return http.NewRequestWithContext(ctx, method, u.String(), rd)
}

var s3Code = regexp.MustCompile(`<Code>([A-Za-z]{1,64})</Code>`)

func (c *Client) do(req *http.Request, body []byte, ok ...int) error {
	sum := sha256.Sum256(body)
	sign(req, hex.EncodeToString(sum[:]), c.accessKey, c.secretKey, c.region, time.Now().UTC())
	//nolint:gosec // G704: la misma petición de request(), contra el endpoint de la configuración.
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("objstore: %s: %w", req.Method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if slices.Contains(ok, resp.StatusCode) {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil
	}
	// Solo el estado y el código de S3: el cuerpo puede traer la ruta firmada, y un log no necesita más.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	code := ""
	if m := s3Code.FindSubmatch(raw); m != nil {
		code = " " + string(m[1])
	}
	return fmt.Errorf("objstore: %s respondió %d%s", req.Method, resp.StatusCode, code)
}

// sign agrega x-amz-date, x-amz-content-sha256 y Authorization (SigV4, servicio s3). Firma el host
// y toda cabecera que ya traiga la petición: Content-Type y Cache-Control van firmados, así nadie en
// medio cambia con qué se sirve la foto.
func sign(req *http.Request, payloadHash, accessKey, secretKey, region string, at time.Time) {
	amzDate := at.Format("20060102T150405Z")
	day := at.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "user-agent" {
			continue
		}
		headers[lk] = strings.TrimSpace(strings.Join(v, ","))
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(headers[k])
		canonHeaders.WriteByte('\n')
	}
	signed := strings.Join(names, ";")

	canonical := strings.Join([]string{
		req.Method, uriEncodePath(req.URL.Path), canonicalQuery(req.URL.Query()),
		canonHeaders.String(), signed, payloadHash,
	}, "\n")
	scope := day + "/" + region + "/s3/aws4_request"
	h := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(h[:])

	key := hmacSHA256([]byte("AWS4"+secretKey), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, toSign))

	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
		",SignedHeaders="+signed+",Signature="+sig)
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// uriEncodePath codifica cada byte fuera de los no reservados de RFC 3986, salvo la diagonal: la
// regla de S3, que no es la de url.PathEscape.
func uriEncodePath(p string) string {
	if p == "" {
		return "/"
	}
	return uriEncode(p, true)
}

func uriEncode(p string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		ch := p[i]
		if (ch == '/' && keepSlash) || ch == '-' || ch == '_' || ch == '.' || ch == '~' ||
			(ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteByte(ch)
		} else {
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

func canonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, uriEncode(k, false)+"="+uriEncode(v, false))
		}
	}
	return strings.Join(parts, "&")
}
