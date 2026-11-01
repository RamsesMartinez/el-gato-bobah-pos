# Contratos: 050
- Pago (`POST /expenses`, `POST /expenses/{id}/pay`): `drawerExtra?: string` (dinero). 400 si supera lo excluido disponible o es inválido.
- Detalle: cada pago trae `drawerExtra`.
