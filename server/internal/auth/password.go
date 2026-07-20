package auth

import "golang.org/x/crypto/bcrypt"

// IsWeakPin rejects PINs that are trivially guessable: too short, all one digit,
// or a straight ascending/descending run (1234, 4321, 000000, 6789…). A 4-digit
// PIN has only 10k combinations, so keeping the obvious ones out matters even
// with the login-path rate limiter in place.
func IsWeakPin(pin string) bool {
	if len(pin) < 4 {
		return true
	}
	allSame, asc, desc := true, true, true
	for i := 1; i < len(pin); i++ {
		if pin[i] != pin[0] {
			allSame = false
		}
		if pin[i] != pin[i-1]+1 {
			asc = false
		}
		if pin[i] != pin[i-1]-1 {
			desc = false
		}
	}
	return allSame || asc || desc
}

// HashSecret hashes a password or PIN with bcrypt. PINs get the same treatment;
// the small keyspace is defended by the account-lockout rate limiter on the login
// and pin-switch handlers (see httpapi.Handlers.authFails).
func HashSecret(secret string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckSecret is constant-time via bcrypt; returns true on match.
func CheckSecret(hash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}

// dummyHash es un hash bcrypt válido, precomputado al costo de HashSecret, contra el que
// se compara en las ramas de fallo del login que no tienen un hash real (usuario
// inexistente o sin password). Se genera en init para que el costo siga a bcrypt.DefaultCost
// automáticamente: si difiriera, la latencia dejaría de igualar a la rama real.
var dummyHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("timing-equalizer"), bcrypt.DefaultCost)
	if err != nil {
		panic(err) // input fijo y válido; un error aquí sería un bug de la librería
	}
	return h
}()

// CheckDummySecret corre un bcrypt de descarte para igualar la latencia de las ramas de
// login sin hash real (evita el oráculo de temporización de enumeración de usuarios, A07).
// Siempre devuelve false: el resultado no se usa, solo el tiempo que tarda.
func CheckDummySecret(secret string) bool {
	return bcrypt.CompareHashAndPassword(dummyHash, []byte(secret)) == nil
}
