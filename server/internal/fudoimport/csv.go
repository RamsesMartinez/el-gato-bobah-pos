package fudoimport

import (
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadSources lee los CSV de FUDO de una carpeta. Un archivo que falta es un error: cargar sin las
// recetas, por ejemplo, terminaría «bien» sin haber hecho nada.
func ReadSources(dir string) (Sources, error) {
	var s Sources
	var err error
	if s.Recipes, err = readCSV(filepath.Join(dir, "recetas.csv")); err != nil {
		return s, err
	}
	if s.SubIngredients, err = readCSV(filepath.Join(dir, "subingredientes.csv")); err != nil {
		return s, err
	}
	if s.SubProducts, err = readCSV(filepath.Join(dir, "subproductos.csv")); err != nil {
		return s, err
	}
	ings, err := readCSV(filepath.Join(dir, "ingredientes.csv"))
	if err != nil {
		return s, err
	}
	s.IngredientUnits = map[string]string{}
	for _, r := range ings {
		s.IngredientUnits[strings.TrimSpace(r["Nombre"])] = r["Unidad"]
	}
	return s, nil
}

func readCSV(path string) ([]Row, error) {
	f, err := os.Open(path) //nolint:gosec // G304: la carpeta la da quien corre la carga, a propósito
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	rd := csv.NewReader(f)
	rd.FieldsPerRecord = -1
	header, err := rd.Read()
	if err != nil {
		return nil, err
	}
	for i := range header {
		header[i] = strings.TrimSpace(strings.TrimPrefix(header[i], "\uFEFF"))
	}
	var out []Row
	for {
		rec, err := rd.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		m := make(Row, len(header))
		for i, h := range header {
			if i < len(rec) {
				m[h] = rec[i]
			}
		}
		out = append(out, m)
	}
}
