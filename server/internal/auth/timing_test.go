package auth

import (
	"sort"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Oráculo de temporización (OWASP A07): si la rama "usuario no encontrado" del login
// no corre bcrypt, responde en ~microsegundos mientras que "password incorrecto" tarda
// decenas de ms, y un atacante enumera usuarios válidos midiendo la latencia.
// CheckDummySecret corre un bcrypt de descarte para cerrar esa brecha.

func TestCheckDummySecret_AlwaysFalse(t *testing.T) {
	// Nunca autentica: solo existe para quemar el tiempo de bcrypt.
	if CheckDummySecret("cualquier-cosa") {
		t.Fatal("CheckDummySecret nunca debe devolver true")
	}
	if CheckDummySecret("") {
		t.Fatal("CheckDummySecret nunca debe devolver true (input vacío)")
	}
}

// Garantía estructural (no flaky): el costo bcrypt del hash dummy iguala al de HashSecret.
// El tiempo de bcrypt lo domina el costo, así que igual costo ⇒ igual latencia entre ramas.
func TestDummyHash_SameCostAsReal(t *testing.T) {
	real, err := HashSecret("password-real")
	if err != nil {
		t.Fatal(err)
	}
	realCost, err := bcrypt.Cost([]byte(real))
	if err != nil {
		t.Fatal(err)
	}
	dummyCost, err := bcrypt.Cost(dummyHash)
	if err != nil {
		t.Fatalf("dummyHash no es un hash bcrypt válido: %v", err)
	}
	if dummyCost != realCost {
		t.Fatalf("costo dummy=%d ≠ real=%d: la latencia entre ramas no queda igualada", dummyCost, realCost)
	}
}

// Medición adversarial de ramas equivalentes: la rama "no encontrado" (dummy) debe tardar
// aproximadamente lo mismo que la rama "password incorrecto" (real). Antes del fix la rama
// dummy era un no-op (~µs); un no-op rompería esta cota por >100×.
func TestLoginBranches_ComparableTiming(t *testing.T) {
	real, err := HashSecret("password-real")
	if err != nil {
		t.Fatal(err)
	}
	median := func(f func()) time.Duration {
		const n = 5
		ds := make([]time.Duration, n)
		for i := range ds {
			start := time.Now()
			f()
			ds[i] = time.Since(start)
		}
		sort.Slice(ds, func(a, b int) bool { return ds[a] < ds[b] })
		return ds[n/2]
	}
	notFound := median(func() { CheckDummySecret("intento") })
	wrongPw := median(func() { CheckSecret(real, "intento") })
	// Cota deliberadamente holgada (mismo orden de magnitud) para no ser flaky bajo carga
	// de CI; basta para detectar la ausencia del bcrypt de descarte.
	if notFound < wrongPw/2 || notFound > wrongPw*2 {
		t.Fatalf("ramas desiguales (posible oráculo): no-encontrado=%v vs password-incorrecto=%v", notFound, wrongPw)
	}
}
