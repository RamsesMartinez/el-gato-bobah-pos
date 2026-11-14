package app

import (
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Un insumo y un producto con el mismo nombre se pisaban en el correo: el renglón del producto salía
// con el proveedor y la compra sugerida del insumo, porque la lista de compra se buscaba por nombre.
func TestTheEmailFindsThePurchaseByItemNotByName(t *testing.T) {
	buy := buyIndex([]ShoppingLine{
		{ItemType: "ingrediente", ID: 7, Name: "Arizona", Supplier: "Bodega del insumo"},
		{ItemType: "producto", ID: 7, Name: "Arizona", Supplier: "Distribuidora del producto"},
	})
	m := &NotificationMailer{}
	got := m.item(mailItem{kind: string(domain.KindStockOut), name: "Arizona", key: -7}, buy)
	if got.Supplier != "Distribuidora del producto" {
		t.Fatalf("el producto salió con el proveedor %q", got.Supplier)
	}
	got = m.item(mailItem{kind: string(domain.KindStockOut), name: "Arizona", key: 7}, buy)
	if got.Supplier != "Bodega del insumo" {
		t.Fatalf("el insumo salió con el proveedor %q", got.Supplier)
	}
}

// Con el correo apagado, «Enviarme por correo» respondía 500: el error no era el que la API sabe
// decir («el correo no está configurado»).
func TestMailOffIsTheErrorTheAPIKnows(t *testing.T) {
	sent := 0
	send := MailSender(func() bool { return false }, func(string, string, string) error { sent++; return nil })
	if err := send([]string{"a@ejemplo.mx"}, "s", "b"); !errors.Is(err, ErrMailDisabled) || sent != 0 {
		t.Fatalf("correo apagado: %v (enviados %d)", err, sent)
	}
	on := MailSender(func() bool { return true }, func(string, string, string) error { sent++; return nil })
	if err := on([]string{"a@ejemplo.mx", "b@ejemplo.mx"}, "s", "b"); err != nil || sent != 2 {
		t.Fatalf("correo encendido: %v (enviados %d)", err, sent)
	}
}
