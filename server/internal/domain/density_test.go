package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestValidDensity(t *testing.T) {
	tests := []struct {
		in string
		ok bool
	}{
		{"0.2999", false}, {"0.3", true}, {"1.1", true}, {"3", true}, {"3.0001", false},
		{"0", false}, {"-1", false},
		// Cinco decimales no caben en la columna: se rechazan en vez de guardarse distintos.
		{"1.10001", false},
		// Ceros de más no cambian el valor: «1.10000» es 1.1.
		{"1.10000", true},
		// Exponentes absurdos de la frontera (revisión de seguridad): 400, nunca 500.
		{"1e21", false}, {"1e-21", false}, {"1e10000", false},
	}
	for _, tt := range tests {
		err := ValidDensity(dec(tt.in))
		if (err == nil) != tt.ok {
			t.Errorf("ValidDensity(%s) = %v; quería ok=%v", tt.in, err, tt.ok)
		}
	}
}

func TestDensityFromWeighing(t *testing.T) {
	tests := []struct {
		name            string
		full, empty, ml string
		want, errHas    string
	}{
		{name: "la salsa del spec", full: "1250", empty: "150", ml: "1000", want: "1.1"},
		{name: "redondea a 4 decimales", full: "1000", empty: "0", ml: "900", want: "1.1111"},
		{name: "vacío igual a lleno", full: "150", empty: "150", ml: "1000", errHas: "vacío"},
		{name: "vacío mayor que lleno (los invirtió)", full: "150", empty: "1250", ml: "1000", errHas: "vacío"},
		{name: "contenido cero", full: "1250", empty: "150", ml: "0", errHas: "contiene"},
		{name: "vacío negativo", full: "1250", empty: "-1", ml: "1000", errHas: "vacío"},
		// Pesar en kg y escribirlo como g da una densidad imposible: el mensaje dice cuánto dio.
		{name: "fuera de lo posible dice el valor", full: "1.25", empty: "0.15", ml: "1000", errHas: "0.0011"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DensityFromWeighing(dec(tt.full), dec(tt.empty), dec(tt.ml))
			if tt.errHas != "" {
				if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), tt.errHas) {
					t.Fatalf("err = %v; quería ErrValidation que diga %q", err, tt.errHas)
				}
				return
			}
			if err != nil || !got.Equal(dec(tt.want)) {
				t.Fatalf("= %s, %v; quería %s", got, err, tt.want)
			}
		})
	}
}

func TestDensityFromLabel(t *testing.T) {
	got, err := DensityFromLabel(dec("4300"), dec("3785"))
	if err != nil || !got.Equal(dec("1.1361")) {
		t.Fatalf("= %s, %v; quería 1.1361", got, err)
	}
	if _, err := DensityFromLabel(dec("0"), dec("3785")); !errors.Is(err, ErrValidation) {
		t.Fatalf("peso neto cero: %v", err)
	}
	if _, err := DensityFromLabel(dec("4300"), dec("0")); !errors.Is(err, ErrValidation) {
		t.Fatalf("contenido cero: %v", err)
	}
}

func TestDensityFactor(t *testing.T) {
	tests := []struct {
		from, to, d, want string
		err               bool
	}{
		{from: "masa", to: "volumen", d: "1.3", want: "0.769231"},
		{from: "volumen", to: "masa", d: "0.92", want: "0.92"},
		{from: "masa", to: "pieza", d: "1.3", err: true},
		{from: "longitud", to: "masa", d: "1.3", err: true},
		{from: "masa", to: "masa", d: "1.3", err: true},
	}
	for _, tt := range tests {
		got, err := DensityFactor(tt.from, tt.to, dec(tt.d))
		if tt.err {
			if err == nil {
				t.Errorf("%s→%s: quería error", tt.from, tt.to)
			}
			continue
		}
		if err != nil || !got.Equal(dec(tt.want)) {
			t.Errorf("%s→%s = %s, %v; quería %s", tt.from, tt.to, got, err, tt.want)
		}
	}
}

func TestNetFromGross(t *testing.T) {
	tests := []struct {
		name string
		in   GrossInput
		want string
		err  error
	}{
		{name: "en gramos resta la tara", in: GrossInput{GrossG: dec("1000"), TareG: dec("50"), Containers: 3, BaseKind: "masa"}, want: "850"},
		{name: "en ml resta y convierte con la densidad", in: GrossInput{GrossG: dec("850"), TareG: dec("120"), Containers: 2, BaseKind: "volumen", Density: decPtr("1.1")}, want: "554.5455"},
		{name: "empaques vacíos son un no hay válido", in: GrossInput{GrossG: dec("240"), TareG: dec("120"), Containers: 2, BaseKind: "volumen", Density: decPtr("1.1")}, want: "0"},
		{name: "pesa menos que los empaques vacíos", in: GrossInput{GrossG: dec("200"), TareG: dec("120"), Containers: 2, BaseKind: "masa"}, err: ErrGrossBelowTare},
		{name: "en ml sin densidad no se adivina", in: GrossInput{GrossG: dec("850"), TareG: dec("120"), Containers: 2, BaseKind: "volumen"}, err: ErrUnitKindMismatch},
		{name: "en piezas no se pesa con empaque", in: GrossInput{GrossG: dec("850"), TareG: dec("120"), Containers: 2, BaseKind: "pieza"}, err: ErrValidation},
		{name: "cero empaques", in: GrossInput{GrossG: dec("850"), TareG: dec("120"), Containers: 0, BaseKind: "masa"}, err: ErrValidation},
		{name: "mil empaques", in: GrossInput{GrossG: dec("850"), TareG: dec("120"), Containers: 1000, BaseKind: "masa"}, err: ErrValidation},
		{name: "tara de más del tope", in: GrossInput{GrossG: dec("850"), TareG: dec("100001"), Containers: 1, BaseKind: "masa"}, err: ErrValidation},
		{name: "bruto negativo", in: GrossInput{GrossG: dec("-1"), TareG: dec("10"), Containers: 1, BaseKind: "masa"}, err: ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NetFromGross(tt.in)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("err = %v; quería %v", err, tt.err)
				}
				return
			}
			if err != nil || !got.Equal(dec(tt.want)) {
				t.Fatalf("= %s, %v; quería %s", got, err, tt.want)
			}
		})
	}
}

// Un valor con un exponente absurdo no se repite en el mensaje: con 1e10000 la respuesta pesaba 10 KB.
func TestValidDensityDoesNotEchoAnAbsurdValue(t *testing.T) {
	err := ValidDensity(dec("1e10000"))
	if err == nil || len(err.Error()) > 200 {
		t.Fatalf("mensaje de %d letras: %v", len(err.Error()), err)
	}
}

func TestValidTare(t *testing.T) {
	for in, ok := range map[string]bool{"0": false, "-1": false, "0.01": true, "100000": true, "100000.01": false, "1.001": false,
		"120.00": true, "1e21": false, "1e-21": false, "1e10000": false} {
		if err := ValidTare(dec(in)); (err == nil) != ok {
			t.Errorf("ValidTare(%s) = %v; quería ok=%v", in, err, ok)
		}
	}
}

// Spec 058, US3: entre peso y líquido la equivalencia la pone la densidad, nunca quien captura.
func TestUnitChangeFactor(t *testing.T) {
	tests := []struct {
		name, from, to, factor string
		density                *string
		want                   string
		err                    error
	}{
		{name: "g a ml sin equivalencia la deriva", from: "masa", to: "volumen", factor: "0", density: sptr("1.3"), want: "0.769231"},
		{name: "ml a g con la equivalencia que mostró la vista previa", from: "volumen", to: "masa", factor: "0.92", density: sptr("0.92"), want: "0.92"},
		{name: "una equivalencia que ya no es la de la densidad", from: "masa", to: "volumen", factor: "0.75", density: sptr("1.3"), err: ErrDensityChanged},
		{name: "sin densidad no hay cambio entre peso y líquido", from: "masa", to: "volumen", factor: "1", err: ErrValidation},
		{name: "a piezas sigue con la equivalencia capturada", from: "masa", to: "pieza", factor: "0.004", density: sptr("1.3"), want: "0.004"},
		{name: "a piezas sin equivalencia se rechaza", from: "masa", to: "pieza", factor: "0", err: ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d *decimal.Decimal
			if tt.density != nil {
				d = decPtr(*tt.density)
			}
			got, err := UnitChangeFactor(tt.from, tt.to, dec(tt.factor), d)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("err = %v; quería %v", err, tt.err)
				}
				return
			}
			if err != nil || !got.Equal(dec(tt.want)) {
				t.Fatalf("= %s, %v; quería %s", got, err, tt.want)
			}
		})
	}
}

func sptr(s string) *string { return &s }

// #204: al volver a abrir lo contado pesando, lo cerrado sale de lo contado menos lo pesado con los
// números que se usaron al capturar (no los de hoy). Si no cuadra, no se inventa un desglose.
func TestClosedFromWeighed(t *testing.T) {
	tests := []struct {
		name    string
		counted string
		in      GrossInput
		want    string
		ok      bool
	}{
		{name: "3 cerradas más lo pesado en ml", counted: "3554.5455", in: GrossInput{GrossG: dec("850"), TareG: dec("120"), Containers: 2, BaseKind: "volumen", Density: decPtr("1.1")}, want: "3000", ok: true},
		{name: "solo lo pesado no deja cerrados", counted: "850", in: GrossInput{GrossG: dec("1000"), TareG: dec("50"), Containers: 3, BaseKind: "masa"}, want: "0", ok: true},
		{name: "lo contado es menos que lo pesado: no cuadra", counted: "100", in: GrossInput{GrossG: dec("1000"), TareG: dec("50"), Containers: 3, BaseKind: "masa"}},
		{name: "en ml sin la densidad de la captura no se adivina", counted: "3554.5455", in: GrossInput{GrossG: dec("850"), TareG: dec("120"), Containers: 2, BaseKind: "volumen"}},
		{name: "en g con decimales", counted: "1850.25", in: GrossInput{GrossG: dec("1000.25"), TareG: dec("50"), Containers: 3, BaseKind: "masa"}, want: "1000", ok: true},
		{name: "una pesada que ya no da cuenta", counted: "850", in: GrossInput{GrossG: dec("100"), TareG: dec("50"), Containers: 3, BaseKind: "masa"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ClosedFromWeighed(dec(tt.counted), tt.in)
			if ok != tt.ok || (ok && !got.Equal(dec(tt.want))) {
				t.Fatalf("= %s, %v; quería %s, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
