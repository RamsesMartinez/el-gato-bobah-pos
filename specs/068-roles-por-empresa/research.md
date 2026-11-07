# Research — 068

## Entrega 1 (cerrada)

`orders.delivered_view`; devolver por `payments.void`; sin migración. Ver historial del archivo.

## Entregas 2 y 3

### Dónde se decide el permiso de una petición
- **Decision**: en cada petición, contra la base, con una consulta memorizada en el contexto.
- **Rationale**: FR-016 pide que quitar un permiso aplique a la siguiente petición. El JWT dura
  15 min; con los permisos dentro del token, un cajero al que se le quitó «devolver» podría seguir
  15 min. La consulta va por llaves primarias.
- **Alternatives**: permisos en el JWT (ventana de 15 min); caché en Redis con versión por empresa
  (más piezas; YAGNI hasta medir).

### Catálogo en la base y constantes en `domain`
- **Decision**: las dos, con un test que exige que coincidan (código, área y siembra).
- **Rationale**: el dueño pidió ids universales en la base; la herencia en SQL necesita
  `system_role_permissions`. Las constantes evitan escribir el código del permiso como texto suelto en
  el router.
- **Alternatives**: solo código (sin FK para los ajustes); solo base (el router compara strings).

### Nombre de los sugeridos
- **Decision**: lo pone el producto (Admin, Gerente, Cajero, Mesero); la empresa no lo renombra.
- **Rationale**: el sugerido es el mismo en todas las empresas; renombrarlo confunde soporte y medición.
  Quien quiera otro nombre crea uno propio.

### Un rol propio puede tener «quitar»
- **Decision**: no. En un propio solo se guarda lo agregado.
- **Rationale**: no hay base que quitar; un «quitar» no tendría efecto y confundiría la pantalla.

### La medición de uso con roles propios
- **Decision**: agrupa por los cuatro sugeridos; lo de un propio se guarda sin rol.
- **Rationale**: la medición ya tiene el balde «sin rol» para no identificar a una persona; un rol
  propio puede ser de una sola persona.

### Último administrador de roles
- **Decision**: la app rechaza cambiar de rol o dar de baja al último usuario activo cuyo rol tenga
  `roles.manage`, bajo el candado 28068 de la empresa.
- **Rationale**: con el admin siempre activo y con `roles.manage`, basta con que exista una persona
  activa con ese rol (u otro con el permiso).

### Convivencia de `users.role`
- **Decision**: trigger `sync_user_role` en la 0108, borrado en la 0109.
- **Alternatives**: que la API nueva escriba los dos (no cubre a la anterior escribiendo `role`).
