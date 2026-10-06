package secrets

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

// Cache recuerda lo que ya se descifró, para que cada aviso de la plataforma no sea una llamada a
// Google y para que Google caído no tumbe lo que ya funcionaba.
//
// Dos reglas que sostienen la seguridad, y que tienen su test:
//   - La copia es del PAR (cifrado, AAD). El mismo cifrado pedido con la AAD de otra empresa va al
//     cifrador y falla; si saliera de la caché, la caché sería la puerta que la AAD cierra.
//   - Con Google caído se sirve la copia VENCIDA, pero nunca se inventa una: lo que no se descifró
//     nunca sigue fallando. Y lo que el cifrador declara ilegible se olvida.
//   - La copia vencida tiene caducidad (maxStale): pasado ese tiempo desde el último descifrado
//     bueno, falla cerrado aunque Google siga caído.
//
// ponytail: la caché no se poda. Crece con cada valor distinto que se descifra —dos o tres por
// empresa y plataforma, más uno por cada recaptura—, o sea cientos de bytes por cambio. El día que
// haya miles de empresas, se poda lo que no se usó en un día.
type Cache struct {
	base     Cipher
	lifetime time.Duration
	// maxStale acota cuánto se sirve la copia VENCIDA con Google caído, contado desde el último
	// descifrado bueno. Sin tope, revocar el permiso de la cuenta de servicio no cortaba nada hasta
	// reiniciar el proceso.
	maxStale time.Duration
	now      func() time.Time

	mu      sync.Mutex
	entries map[[sha256.Size]byte]entry
}

type entry struct {
	plaintext []byte
	since     time.Time
}

// WithCache envuelve un cifrador. `lifetime` es cuánto se confía en una copia sin volver a
// preguntar; `maxStale`, cuánto se sirve vencida mientras el servicio de llaves no responde.
func WithCache(base Cipher, lifetime, maxStale time.Duration) *Cache {
	return &Cache{base: base, lifetime: lifetime, maxStale: maxStale, now: time.Now, entries: map[[sha256.Size]byte]entry{}}
}

func cacheKey(ciphertext []byte, aad string) [sha256.Size]byte {
	h := sha256.New()
	h.Write(ciphertext)
	h.Write([]byte{0})
	h.Write([]byte(aad))
	var k [sha256.Size]byte
	copy(k[:], h.Sum(nil))
	return k
}

// Encrypt deja recordado el claro: lo que se acaba de guardar se usa enseguida (la prueba contra la
// plataforma, el primer aviso), y descifrarlo de vuelta sería un viaje a Google para nada.
func (c *Cache) Encrypt(ctx context.Context, plaintext []byte, aad string) ([]byte, error) {
	ct, err := c.base.Encrypt(ctx, plaintext, aad)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries[cacheKey(ct, aad)] = entry{plaintext: append([]byte(nil), plaintext...), since: c.now()}
	c.mu.Unlock()
	return ct, nil
}

func (c *Cache) Decrypt(ctx context.Context, ciphertext []byte, aad string) ([]byte, error) {
	k := cacheKey(ciphertext, aad)
	c.mu.Lock()
	e, found := c.entries[k]
	c.mu.Unlock()
	if found && c.now().Sub(e.since) < c.lifetime {
		return append([]byte(nil), e.plaintext...), nil
	}

	plaintext, err := c.base.Decrypt(ctx, ciphertext, aad)
	switch {
	case err == nil:
		c.mu.Lock()
		c.entries[k] = entry{plaintext: append([]byte(nil), plaintext...), since: c.now()}
		c.mu.Unlock()
		return plaintext, nil
	case errors.Is(err, ErrUnavailable) && found && c.now().Sub(e.since) < c.maxStale:
		return append([]byte(nil), e.plaintext...), nil
	default:
		if errors.Is(err, ErrUnreadable) {
			c.mu.Lock()
			delete(c.entries, k)
			c.mu.Unlock()
		}
		return nil, err
	}
}
