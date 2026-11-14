-- Medios de pago (lookup)

-- name: ListPaymentMethods :many
-- delivery_platform_id: a qué plataforma pertenece el método, o NULL si no es de plataforma. Es
-- lo que deja al POS ofrecer solo los dos de la plataforma activa sin comparar nombres.
select id, name, kind, affects_cash_drawer, auto_declare, delivery_platform_id from payment_methods where is_active order by sort_key, name;

-- name: ListAllPaymentMethods :many
-- Todos, incluidos los apagados. Es la lista de la pantalla de AJUSTES, y por eso no puede ser la
-- misma que ofrece el POS para cobrar.
--
-- Existe porque apagar un método era una puerta de un solo sentido: la tabla de interruptores se
-- pintaba con la lista filtrada, así que el renglón desaparecía junto con su propio interruptor y
-- no quedaba camino en la aplicación para volver a encenderlo. Un dedo que falla por milímetros
-- sobre «Efectivo» dejaba al mostrador sin cobrar en efectivo hasta que alguien entrara a la base.
select id, name, kind, is_cash, affects_cash_drawer, auto_declare, delivery_platform_id, is_active
from payment_methods order by sort_key, name;

-- name: GetPaymentMethod :one
-- Trae `is_active` para que quien MUEVE DINERO con este método pueda rechazarlo si el negocio lo
-- apagó. No se filtra en el where: configurar un método desactivado —cambiarle el auto-declare, por
-- ejemplo— tiene que seguir siendo posible, y ahí el estado no estorba.
select id, name, kind, is_cash, affects_cash_drawer, auto_declare, delivery_platform_id, is_active
from payment_methods where id = $1;

-- name: LockPaymentMethod :one
-- El mismo renglón que GetPaymentMethod, tomado para actualizar.
--
-- Existe porque configurar un método es leer-validar-escribir, y sin lock dos PATCH simultáneos
-- —uno que enciende «automático» y otro que enciende «va al cajón»— pasan cada uno su validación
-- contra el estado viejo y dejan escrita la combinación que el código considera imposible. Que
-- hoy no se pierda dinero por eso es una coincidencia del orden en que `CloseSession` resuelve los
-- métodos del cajón, no una garantía.
select id, name, kind, is_cash, affects_cash_drawer, auto_declare, delivery_platform_id, is_active
from payment_methods where id = $1 for update;

-- name: UpdatePaymentMethodFlags :one
-- Los tres interruptores del método, en una actualización PARCIAL: lo que no viene, no cambia.
--
-- `sqlc.narg` y no tres parámetros obligatorios, porque un `bool` sin puntero no distingue "ausente"
-- de "falso": un PATCH que solo quería desactivar el método le habría apagado de paso el interruptor
-- del cajón, sacando su dinero del arqueo. Plata movida por el tipo de dato y no por el negocio.
update payment_methods set
  auto_declare        = coalesce(sqlc.narg('auto_declare'), auto_declare),
  is_active           = coalesce(sqlc.narg('is_active'), is_active),
  affects_cash_drawer = coalesce(sqlc.narg('affects_cash_drawer'), affects_cash_drawer)
where id = $1
returning id, name, kind, affects_cash_drawer, auto_declare, is_active;

-- Cajas (registros físicos con nombre; la primaria recibe las ventas del POS)

-- name: ListCashRegisters :many
-- Cajas activas + el id de su sesión abierta (null si está cerrada). Para pickers y la vista de caja.
-- LEFT JOIN (no subquery escalar) para que sqlc infiera open_session_id como nullable; el índice
-- único one_open_session_per_register garantiza ≤1 sesión abierta por caja (sin duplicar filas).
select r.id, r.name, r.is_primary, r.is_active, r.counts_cash, s.id as open_session_id
from cash_registers r
left join register_sessions s on s.register_id = r.id and s.status = 'abierta'
where r.is_active
order by r.is_primary desc, r.name;

-- name: ListAllCashRegisters :many
-- fixed_float va aquí (configuración) y NO en ListCashRegisters, la que lee el cajero para abrir: el
-- fijo es casi siempre el esperado de la apertura y la apertura se cuenta a ciegas (spec 035).
select id, name, is_primary, is_active, fixed_float, counts_cash from cash_registers order by is_primary desc, name;

-- name: GetCashRegister :one
select id, name, is_primary, is_active, fixed_float, counts_cash from cash_registers where id = $1;

-- name: GetCashRegisterForShare :one
-- La caja de un pago de gasto, con candado compartido (spec 048, R4): una apertura, que la toma FOR
-- UPDATE, termina antes —y el pago la ve abierta— o espera a que el pago confirme y lo barre.
select id, counts_cash from cash_registers where id = $1 for share;

-- name: GetCashRegisterForUpdate :one
-- Bajo candado: quitar el arqueo y abrir un turno a la vez dejarían un turno abierto en una caja
-- que no se cuenta (spec 047). Las dos operaciones leen la caja con este FOR UPDATE.
select id, is_primary, counts_cash from cash_registers where id = $1 for update;

-- name: SetRegisterFixedFloat :one
update cash_registers set fixed_float = sqlc.narg(fixed_float) where id = sqlc.arg(id)
returning id, name, is_primary, is_active, fixed_float, counts_cash;

-- name: CreateCashRegister :one
-- is_primary/is_active toman su default (secundaria, activa): la primaria la fija la migración.
insert into cash_registers (name) values ($1)
returning id, name, is_primary, is_active;

-- name: UpdateCashRegister :one
-- counts_cash con coalesce: renombrar una caja desde una pantalla que no conoce el ajuste no se lo
-- quita (el mismo defecto que ya costó affects_cash_drawer).
update cash_registers set name = sqlc.arg(name), is_active = sqlc.arg(is_active),
       counts_cash = coalesce(sqlc.narg(counts_cash), counts_cash)
where id = sqlc.arg(id)
returning id, name, is_primary, is_active, counts_cash;

-- name: CurrentBranch :one
-- La sucursal de la sesión (0076). Error EGB01 si la empresa tiene cero o más de una activa.
select current_branch_id()::bigint;

-- name: SeedBasePaymentMethods :exec
-- Métodos de pago base para una empresa recién creada. Desde 0037 la tabla es per-tenant, así que
-- una empresa nueva nace SIN NINGUNO y no podría cobrar: /payment-methods devolvería vacío y el
-- checkout se quedaría sin botones. Antes los heredaba por ser una tabla global.
--
-- Los de PLATAFORMA quedan fuera a propósito: vender por Uber/DiDi/Rappi exige que ese negocio haya
-- hecho su propia vinculación con la plataforma, y darle tres formas de cobro que no tiene
-- contratadas es peor que no darle ninguna.
-- `is_cash` se escribe explícito: la columna nace en `false` y la 0067 le puso un `check` que
-- exige que solo lo que se cobra en billetes entre al cajón. Sin esto, sembrar «Efectivo» con
-- `affects_cash_drawer` viola la restricción y **una empresa nueva no se puede crear** — lo
-- encontró el propio `check`, en el sembrado de la segunda empresa de los tests de aislamiento.
insert into payment_methods (company_id, name, kind, is_cash, affects_cash_drawer, is_active, sort_key, auto_declare)
values
  ($1, 'Efectivo',           'efectivo',      true,  true,  true, 100, false),
  ($1, 'Tarjeta débito',     'tarjeta',       false, false, true, 200, true),
  ($1, 'Tarjeta crédito',    'tarjeta',       false, false, true, 250, true),
  ($1, 'Transferencia SPEI', 'transferencia', false, false, true, 300, true)
on conflict (company_id, name) do nothing;

-- name: GetBusinessTimezone :one
-- Zona horaria del local, para calcular la FECHA de negocio. La base guarda instantes en UTC; la
-- fecha es una decisión de calendario y depende de dónde está el negocio.
select timezone from business_settings limit 1;

-- name: SeedDeliveryPlatforms :exec
-- Plataformas de reparto de una empresa nueva. Son etiquetas: sin ellas la venta no puede registrar
-- por dónde entró, y ni siquiera existe "Propio" para el reparto del propio negocio.
--
-- El margen nace en 0, NO en 35%. El margen es la decisión de negocio que viene con la vinculación:
-- ese local todavía no tiene contrato con Uber, y estrenarlo cobrando 35% más sería una sorpresa
-- cara. El dueño del sistema lo configura cuando ese negocio lo pide, junto con sus métodos de pago
-- de plataforma — que por la misma razón tampoco se siembran.
--
-- company_id NO se lista: lo pone el DEFAULT desde el GUC del tenant. Tampoco se podría — 0023
-- agregó esa columna con SQL dinámico y para sqlc no existe. Corre dentro de WithTenant.
insert into delivery_platforms (name, price_markup_pct)
values ('Didi', 0), ('Uber Eats', 0), ('Rappi', 0), ('Propio', 0)
on conflict do nothing;

-- name: LockRegistersForSalesMove :many
-- Pasar la venta (spec 049): la caja que va a vender y la que vende hoy en SU sucursal, bloqueadas
-- en orden de id para que dos cambios a la vez no se esperen en ciclo. La sucursal sale de la fila
-- de la caja destino y no de la sesión: con dos sucursales y sin selector, la sesión no la sabe.
-- Abrir turno toma la caja con GetCashRegisterForUpdate, así que no se cuela entre esto y el cambio.
select r.id, r.branch_id, r.is_active, r.is_primary
  from cash_registers r
 where r.id = sqlc.arg(target_id)
    or (r.is_primary and r.branch_id = (select t.branch_id from cash_registers t where t.id = sqlc.arg(target_id)))
 order by r.id
   for update of r;

-- name: RegistersWithOpenShift :many
-- Qué cajas tienen turno abierto. Va en su propio statement DESPUÉS del candado: dentro del mismo
-- SELECT ... FOR UPDATE se leería con la foto de antes de esperar, y no vería el turno que la
-- apertura confirmó mientras este cambio esperaba.
select register_id from register_sessions
 where register_id = any(sqlc.arg(ids)::bigint[]) and status = 'abierta';

-- name: SetRegisterSells :exec
-- La que deja de vender va primero: el índice de una principal por sucursal no admite dos.
update cash_registers set is_primary = sqlc.arg(sells), counts_cash = counts_cash or sqlc.arg(sells)
 where id = sqlc.arg(id);
