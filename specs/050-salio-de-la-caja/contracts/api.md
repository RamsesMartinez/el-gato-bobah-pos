# Contratos: 050
- Pago (`POST /expenses`, `POST /expenses/{id}/pay`): `drawerExtra?: string` (dinero). 400 si supera lo excluido disponible o es inválido.
- Detalle, corte (`ListExpensesBySession`, desglose) y lista de pendientes traen `drawerExtra`; el desglose del corte trae aparte «No es del negocio».
- `PATCH /expenses/{id}/items/{itemId}` con `excluded: false`: 400 si lo excluido quedaría por debajo de lo usado como diferencia (R2).
