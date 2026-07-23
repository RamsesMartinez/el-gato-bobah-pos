// Command migrate aplica todas las migraciones goose embebidas a DATABASE_URL y sale. Standalone
// a propósito (sin config.Validate ni JWT_SECRET): lo usan CI (cargar el esquema para `sqlc vet`)
// y operaciones puntuales, sin arrastrar la validación de secretos del arranque del API.
package main

import (
	"context"
	"log"
	"os"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("migrate: define DATABASE_URL")
	}
	ctx := context.Background()
	st, err := store.New(ctx, url)
	if err != nil {
		log.Fatalf("migrate: conexión: %v", err)
	}
	defer st.Close()
	if err := store.Migrate(ctx, st.Pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrate: migraciones aplicadas")
}
