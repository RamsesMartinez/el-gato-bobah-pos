package secrets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Direcciones de Google. El metadata server va por IP y no por `metadata.google.internal`: dentro de
// la red de compose el nombre lo resuelve el DNS de Docker, y la IP no depende de eso. Verificado el
// 2026-09-27 desde un contenedor en las dos VMs.
const (
	defaultMetadataURL = "http://169.254.169.254"
	defaultKMSURL      = "https://cloudkms.googleapis.com"
)

// tokenRenewalMargin: cuánto antes de vencer se pide otro token al metadata server. El token dura
// una hora; renovarlo después del 401 haría fallar la operación que lo descubrió.
const tokenRenewalMargin = 5 * time.Minute

// maxResponseBytes acota lo que se lee de Google. Un secreto de plataforma mide cientos de bytes.
const maxResponseBytes = 1 << 20

// KMS cifra con una llave de Cloud KMS por REST, sin el SDK de Google: el SDK arrastra gRPC y
// protobuf para dos llamadas, y el principio VI pide stdlib para lo que la stdlib resuelve.
//
// Las credenciales salen del metadata server de la VM, es decir, de la cuenta de servicio adjunta.
// No hay archivo de llave de Google en ningún lado.
type KMS struct {
	keyName     string // projects/…/cryptoKeys/…
	metadataURL string
	apiURL      string
	http        *http.Client
	now         func() time.Time

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewKMS arma el cliente para una llave. `keyName` ya viene validada por config.
func NewKMS(keyName string) *KMS {
	return &KMS{
		keyName: keyName, metadataURL: defaultMetadataURL, apiURL: defaultKMSURL,
		http: &http.Client{Timeout: 10 * time.Second},
		now:  time.Now,
	}
}

func (k *KMS) Encrypt(ctx context.Context, plaintext []byte, aad string) ([]byte, error) {
	var out struct {
		Ciphertext string `json:"ciphertext"`
	}
	status, err := k.call(ctx, "encrypt", map[string]string{
		"plaintext":                   base64.StdEncoding.EncodeToString(plaintext),
		"additionalAuthenticatedData": base64.StdEncoding.EncodeToString([]byte(aad)),
	}, &out)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: encrypt returned HTTP %d", ErrUnavailable, status)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(out.Ciphertext)
	if err != nil || len(ciphertext) == 0 {
		return nil, fmt.Errorf("%w: the encrypt response carries no ciphertext", ErrUnavailable)
	}
	return append([]byte{formatKMS}, ciphertext...), nil
}

func (k *KMS) Decrypt(ctx context.Context, ciphertext []byte, aad string) ([]byte, error) {
	if len(ciphertext) < 2 || ciphertext[0] != formatKMS {
		return nil, ErrUnreadable
	}
	var out struct {
		Plaintext string `json:"plaintext"`
	}
	status, err := k.call(ctx, "decrypt", map[string]string{
		"ciphertext":                  base64.StdEncoding.EncodeToString(ciphertext[1:]),
		"additionalAuthenticatedData": base64.StdEncoding.EncodeToString([]byte(aad)),
	}, &out)
	if err != nil {
		return nil, err
	}
	// 400 es la respuesta de KMS ante un cifrado ajeno o una AAD distinta. Cualquier otro código
	// —403 por permisos, 5xx, 429— es de disponibilidad, y no dice nada del valor.
	if status == http.StatusBadRequest {
		return nil, ErrUnreadable
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: decrypt returned HTTP %d", ErrUnavailable, status)
	}
	plaintext, err := base64.StdEncoding.DecodeString(out.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("%w: the decrypt response is not base64", ErrUnavailable)
	}
	return plaintext, nil
}

// call hace el POST a `:encrypt` o `:decrypt`. Devuelve el estado HTTP para que cada operación
// decida qué significa; los errores de red ya salen como ErrUnavailable.
func (k *KMS) call(ctx context.Context, verb string, body map[string]string, out any) (int, error) {
	token, err := k.currentToken(ctx)
	if err != nil {
		return 0, err
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.apiURL+"/v1/"+k.keyName+":"+verb, bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := k.http.Do(req)
	if err != nil {
		// Sin envolver el error de red: no aporta y puede traer la URL completa.
		return 0, fmt.Errorf("%w: could not reach KMS", ErrUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return 0, fmt.Errorf("%w: unreadable KMS response", ErrUnavailable)
	}
	return resp.StatusCode, nil
}

// currentToken pide al metadata server el token de la cuenta de servicio, y lo reusa hasta poco
// antes de que venza.
func (k *KMS) currentToken(ctx context.Context) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.token != "" && k.now().Add(tokenRenewalMargin).Before(k.expires) {
		return k.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		k.metadataURL+"/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := k.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: the metadata server did not respond", ErrUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: the metadata server returned HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&tr); err != nil || tr.AccessToken == "" {
		return "", fmt.Errorf("%w: the metadata server returned no token", ErrUnavailable)
	}
	k.token = tr.AccessToken
	k.expires = k.now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return k.token, nil
}
