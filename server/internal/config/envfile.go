package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvFile carga variables desde un archivo .env con parseo 100% LITERAL: el valor
// es todo lo que sigue al '=' hasta el fin de línea, SIN expansión de $ ni comentarios
// inline (soporta cualquier contraseña: #, $, !, espacios, etc., como python-decouple).
// Solo se quitan comillas envolventes opcionales. No sobrescribe variables ya definidas
// (dev inyecta la conexión; prod usa compose).
func LoadEnvFile() {
	var path string
	for _, f := range []string{os.Getenv("ENV_FILE"), "deploy/.env", "../deploy/.env"} {
		if f == "" {
			continue
		}
		if _, err := os.Stat(f); err == nil {
			path = f
			break
		}
	}
	if path == "" {
		return
	}
	f, err := os.Open(path) //nolint:gosec // ruta de configuración conocida
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // línea vacía o comentario (solo a inicio de línea)
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// quitar comillas envolventes opcionales, sin tocar el contenido
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
