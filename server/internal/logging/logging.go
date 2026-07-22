// Package logging configura slog escribiendo a stdout y a un archivo rotado en logs/.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Setup crea el logger JSON que escribe a stdout + logs/app.log con rotación por
// tamaño (archivos chicos, fáciles de revisar). Devuelve también el *lumberjack
// por si se quiere cerrar.
func Setup(dir, level string) (*slog.Logger, error) {
	if dir == "" {
		dir = "logs"
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	rotator := &lumberjack.Logger{
		Filename: filepath.Join(dir, "app.log"),
		// ponytail: 1MB es el mínimo real de lumberjack (MaxSize son megabytes enteros, sin
		// fracciones) — la etapa de desarrollo pidió 64KB, no alcanzable con esta librería tal
		// cual; 1MB + 10 backups sigue siendo chico y deja ver la rotación sin generar GBs de prueba.
		MaxSize:    1,
		MaxBackups: 10, // archivos históricos
		MaxAge:     14, // días
		Compress:   true,
	}
	w := io.MultiWriter(os.Stdout, rotator)
	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)}))
	return logger, nil
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
