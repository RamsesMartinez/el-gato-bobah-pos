package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxWarehouseName es el largo del nombre de un almacén, en letras.
const MaxWarehouseName = 40

var (
	// ErrWarehouseNotFound: el almacén no existe, está archivado donde no se admite, o es de otra
	// empresa o de otra sucursal. Las tres se ven igual a propósito: no dice qué ids existen.
	ErrWarehouseNotFound = fmt.Errorf("%w: ese almacén no existe", ErrNotFound)
	// ErrWarehouseIsSales: el almacén de venta no se archiva; de él salen las ventas.
	ErrWarehouseIsSales = fmt.Errorf("%w: el almacén de venta no se archiva", ErrConflict)
	// ErrWarehouseHasStock: con existencias distintas de cero archivarlo las escondería.
	ErrWarehouseHasStock = fmt.Errorf("%w: el almacén tiene existencias", ErrConflict)
	// ErrWarehouseCountOpen: archivarlo dejaría un conteo abierto sin almacén donde cerrarse.
	ErrWarehouseCountOpen = fmt.Errorf("%w: el almacén tiene un conteo abierto", ErrConflict)
	// ErrWarehousePendingPurchases: tiene renglones de compra por recibir; lo que llegara después
	// entraría a un almacén que ya no sale en ninguna lista.
	ErrWarehousePendingPurchases = fmt.Errorf("%w: el almacén tiene compras por recibir", ErrConflict)
	// ErrWarehouseNameTaken: ya hay un almacén activo con ese nombre en la sucursal.
	ErrWarehouseNameTaken = fmt.Errorf("%w: ya hay un almacén con ese nombre", ErrConflict)
	// ErrWarehouseMissing: la sucursal no tiene almacén de venta (EGB03 de la base). No debería
	// pasar: lo crea CreateCompany con la matriz. Es la base negándose a adivinar.
	ErrWarehouseMissing = errors.New("la sucursal no tiene almacén de venta")
	// ErrWarehouseNotWritableYet: mientras la base guarde las existencias con llave por sucursal
	// (primera subida de la spec 094), solo el almacén de venta recibe movimientos y no se puede
	// cambiar cuál es (EGB04).
	ErrWarehouseNotWritableYet = fmt.Errorf("%w: ese almacén todavía no recibe mercancía", ErrConflict)
)

// WarehouseHasStock lleva cuántos artículos tienen existencia, para que la pantalla lo diga.
type WarehouseHasStock struct{ Items int64 }

func (e WarehouseHasStock) Error() string {
	return fmt.Sprintf("el almacén tiene %d artículos con existencia: traspásalos o cuéntalos en cero antes de archivarlo", e.Items)
}
func (e WarehouseHasStock) Unwrap() error { return ErrWarehouseHasStock }

// CleanWarehouseName es el nombre como se guarda: sin espacios en las orillas ni repetidos adentro.
func CleanWarehouseName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// ValidWarehouseName pide de 1 a 40 letras, contadas después de limpiar los espacios.
func ValidWarehouseName(name string) error {
	n := utf8.RuneCountInString(CleanWarehouseName(name))
	if n == 0 || n > MaxWarehouseName {
		return fmt.Errorf("%w: el almacén lleva de 1 a %d letras", ErrValidation, MaxWarehouseName)
	}
	return nil
}

// CanArchiveWarehouse decide si un almacén se puede archivar. El de venta se revisa primero: aunque
// esté vacío, archivarlo dejaría a la sucursal sin de dónde vender.
func CanArchiveWarehouse(isSales bool, itemsWithStock int64, countOpen, pendingPurchases bool) error {
	switch {
	case isSales:
		return ErrWarehouseIsSales
	case itemsWithStock != 0:
		return WarehouseHasStock{Items: itemsWithStock}
	case countOpen:
		return ErrWarehouseCountOpen
	case pendingPurchases:
		return ErrWarehousePendingPurchases
	}
	return nil
}

// WarehouseSel es qué almacén pidió una lectura: ninguno (el de venta), uno, o todos sumados.
type WarehouseSel struct {
	ID  int64
	All bool
}

// Sales dice que no se pidió almacén: se lee el de venta.
func (s WarehouseSel) Sales() bool { return s.ID == 0 && !s.All }

// ParseWarehouseParam lee el almacén de una petición. Ausente es el de venta; presente y malformado
// se rechaza, nunca cae al de venta en silencio (constitución V): una pantalla que pidió «Bodega»
// y recibe «Local» enseña cifras que nadie pidió.
func ParseWarehouseParam(raw string, allowAll bool) (WarehouseSel, error) {
	switch {
	case raw == "":
		return WarehouseSel{}, nil
	case raw == "all" && allowAll:
		return WarehouseSel{All: true}, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return WarehouseSel{}, fmt.Errorf("%w: almacén inválido", ErrValidation)
	}
	return WarehouseSel{ID: id}, nil
}

// TransferCounterpart es el otro almacén de una pata de traspaso: si la pata está en el origen, el
// destino; si no, el origen. Una pata no dice hacia dónde fue y sin esto la lista de movimientos
// mostraría «traspaso −6» sin decir a dónde.
func TransferCounterpart(legWarehouse, fromWarehouse int64, fromName, toName string) string {
	if legWarehouse == fromWarehouse {
		return toName
	}
	return fromName
}

// WarehouseFromBody lee el almacén del cuerpo de una escritura: ausente es el de venta; presente y
// no positivo se rechaza, igual que en la URL.
func WarehouseFromBody(id *int64) (WarehouseSel, error) {
	if id == nil {
		return WarehouseSel{}, nil
	}
	if *id <= 0 {
		return WarehouseSel{}, fmt.Errorf("%w: almacén inválido", ErrValidation)
	}
	return WarehouseSel{ID: *id}, nil
}
