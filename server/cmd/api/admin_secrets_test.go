package main

import (
	"errors"
	"strings"
	"testing"
)

// fakePrompt contesta en orden y falla si se le pide más de lo previsto: un paso de más es una
// pregunta que el script de quien lo use no sabe contestar.
func fakePrompt(t *testing.T, answers ...string) func(string) (string, error) {
	t.Helper()
	i := 0
	return func(string) (string, error) {
		if i >= len(answers) {
			return "", errors.New("se pidió un valor de más")
		}
		i++
		return answers[i-1], nil
	}
}

// LA CONTRASEÑA Y EL PIN DEL ADMIN YA NO VIVEN EN deploy/.env.
//
// Quedaban en el .env de las dos VMs y de cada laptop, legibles para siempre, aunque el arranque
// solo los usara con la base vacía. Ahora se piden al momento, y cada borde de la captura es un
// admin que se queda sin poder entrar sin que nada lo diga.
func TestAdminSecretsAreAskedNotRead(t *testing.T) {
	cases := []struct {
		name    string
		answers []string
		wantPIN string
		reject  string // fragmento del error; vacío = se acepta
	}{
		{"contraseña y PIN confirmados", []string{"Contrasena-Larga-1!", "Contrasena-Larga-1!", "482715", "482715"}, "482715", ""},
		{"PIN vacío es entrar solo con contraseña", []string{"Contrasena-Larga-1!", "Contrasena-Larga-1!", ""}, "", ""},
		{"la confirmación de la contraseña no coincide", []string{"Contrasena-Larga-1!", "Contrasena-Larga-2!"}, "", "no coinciden"},
		{"la confirmación del PIN no coincide", []string{"Contrasena-Larga-1!", "Contrasena-Larga-1!", "482715", "482716"}, "", "no coinciden"},
		{"contraseña vacía", []string{"", ""}, "", "vacía"},
		{"el valor de ejemplo del repo público", []string{"cambia-esto", "cambia-esto"}, "", "ejemplo"},
		{"PIN de secuencia", []string{"Contrasena-Larga-1!", "Contrasena-Larga-1!", "1234", "1234"}, "", "débil"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pw, pin, err := askAdminSecrets(fakePrompt(t, c.answers...))
			if c.reject != "" {
				if err == nil || !strings.Contains(err.Error(), c.reject) {
					t.Fatalf("se esperaba un rechazo con %q y salió err=%v", c.reject, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("se rechazó una captura válida: %v", err)
			}
			if pw != c.answers[0] || pin != c.wantPIN {
				t.Fatalf("se guardaría otra cosa de lo que se tecleó: contraseña %q PIN %q", pw, pin)
			}
		})
	}
}

// CUANDO NO HAY TERMINAL, CADA RESPUESTA ES UN RENGLÓN. Es lo que deja a un script crear el admin
// de un worktree nuevo sin escribir su contraseña en ningún archivo. El `\r` importa: un renglón
// que llega de Windows lo trae, y sin quitarlo la contraseña guardada no es la que se teclea.
func TestLinePromptReadsOneAnswerPerLine(t *testing.T) {
	ask := linePrompt(strings.NewReader("Contrasena-Larga-1!\r\nsegunda\n"))
	for _, want := range []string{"Contrasena-Larga-1!", "segunda"} {
		got, err := ask("x: ")
		if err != nil || got != want {
			t.Fatalf("leyó %q (err=%v), quería %q", got, err, want)
		}
	}
	if _, err := ask("x: "); err == nil {
		t.Fatal("sin más renglones tiene que fallar; devolver vacío dejaría al admin sin PIN en silencio")
	}
}

// EL ADMIN QUE CREA -reset-admin NACÍA SIN LA HUELLA DE SU PIN.
//
// `-reset-admin` armaba sus propios datos al crear y se le olvidó `PinLookup`: el PIN quedaba
// guardado pero el modo de solo-PIN —que busca por la huella— no lo encontraba, y el admin no podía
// desbloquear sin que nada lo dijera. Era un camino raro mientras el arranque creaba al primer
// admin; desde que las credenciales dejaron el .env, es EL camino. Por eso los dos que crean un
// admin pasan por la misma función.
func TestANewAdminKeepsItsPinLookup(t *testing.T) {
	pinHash, pinLookup, err := pinDeAdmin("482715", "pepper-de-prueba-suficientemente-largo")
	if err != nil {
		t.Fatal(err)
	}
	p := newAdminParams("admin", "hash-de-contrasena", pinHash, pinLookup)
	if p.PinLookup == nil || *p.PinLookup != *pinLookup {
		t.Fatal("el admin nuevo se guarda sin la huella del PIN: con solo-PIN no puede desbloquear")
	}
	if p.PinHash == nil || p.PasswordHash == nil || p.Username == nil || *p.Username != "admin" {
		t.Fatalf("faltan datos del admin: %+v", p)
	}
}
