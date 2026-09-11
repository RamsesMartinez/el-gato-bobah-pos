-- +goose Up

-- UN SOLO ARQUEO DE CAJÓN (spec 015).
--
-- El cajón es un solo montón de billetes, pero el corte pedía una cifra declarada por CADA método
-- que lo toca: el efectivo del mostrador y los tres de plataforma en efectivo. Pedirle a una persona
-- que parta un montón por canal de venta es pedirle que invente un reparto, y el sistema lo guardaba
-- como un hecho. Medido en el ambiente de pruebas: un turno con «Efectivo» en diferencia 0.00 y
-- «Didi efectivo» en −64.80, sin forma de saber si eran $64.80 que no llegaron o $64.80 que sí
-- estaban en el cajón y se contaron dentro de los otros.
--
-- A partir de aquí el conteo se compara contra UNA cifra y la diferencia es una. Lo que esta
-- migración agrega son los dos HECHOS que el cierre tiene que congelar para que un arqueo firmado se
-- pueda reconstruir tal como se firmó.

-- lock_timeout: agregar una columna GENERADA `stored` reescribe la tabla bajo ACCESS EXCLUSIVE. Hoy
-- es barato porque `session_cash_counts` nació en la 0066 y todavía no tiene datos en producción,
-- pero el día que los tenga, una migración que se quede esperando un lock detiene la caja. Fallar
-- rápido es mejor que dejar el deploy colgado con la API a medio sustituir.
set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. El conteo pasa a ser el arqueo: guarda contra qué se comparó.
-- ---------------------------------------------------------------------------------------------

-- POR QUÉ SE GUARDA Y NO SE RECALCULA. El esperado sale de `order_payments`, y una venta cancelada
-- o reembolsada después del cierre lo mueve: recalcularlo mañana daría otra cifra que la que el
-- operador firmó hoy. Es el mismo snapshot que `order_lines.unit_price`, por la misma razón.
--
-- Nulo en la apertura, a propósito: al abrir no hay nada que esperar — el fondo ES lo que se contó.
alter table session_cash_counts
  add column expected numeric(10,2);

-- La resta la hace Postgres. Es la misma decisión que `register_session_totals.difference`: con dos
-- lugares donde escribir el mismo faltante, tarde o temprano difieren y nadie sabe cuál creer.
alter table session_cash_counts
  add column difference numeric(10,2) generated always as (total - expected) stored;

-- EL CHECK VA `NOT VALID`, y eso no es descuido. La 0066 ya está en el ambiente de pruebas con
-- conteos de cierre sin esperado: un check validado los rechazaría y la migración no correría.
-- `not valid` perdona lo que ya existe sin escanear la tabla y obliga a TODO insert nuevo.
--
-- Lo que atrapa: un `CloseSession` que algún día olvide setear el esperado deja un nulo que se lee
-- IGUAL que un corte anterior a esta feature. Sin el check, el bug se disfraza de historia; con él,
-- falla ruidoso con un 23514.
alter table session_cash_counts
  add constraint session_cash_counts_expected_por_momento check (
    (moment = 'apertura' and expected is null)
    or (moment = 'cierre' and expected is not null)
  ) not valid;

-- ---------------------------------------------------------------------------------------------
-- 2. El renglón del corte guarda si ese método tocaba el cajón.
-- ---------------------------------------------------------------------------------------------

-- SNAPSHOT, por la puerta que cierra si no está. Hasta hoy `affects_cash_drawer` se reconstruía en
-- vivo por join a `payment_methods`, y era inofensivo porque nadie podía cambiarlo: no había
-- endpoint que lo escribiera. La spec 015 le da uno.
--
-- El día que el dueño apague «el efectivo de Didi llega al cajón» —porque esa plataforma cambió de
-- forma de repartir—, TODO corte cerrado antes se reagruparía con el flag de hoy: sus cifras no
-- cambian, pero la forma de su reporte sí, y un arqueo que se lee distinto según cuándo lo abras no
-- se puede auditar. Es el mismo defecto que la constitución ya nombró para la categoría de un
-- producto: recategorizar reescribe el pasado.
--
-- `default false` solo para poder poblar las filas que ya existen sin adivinar; el insert del cierre
-- lo manda siempre explícito. Se deja el default puesto porque quitarlo obligaría a otra reescritura
-- de tabla y no compra nada.
alter table register_session_totals
  add column affects_cash_drawer boolean not null default false;

-- Las filas que ya existen se pueblan desde el catálogo: es el único momento en que el flag de hoy
-- es la mejor respuesta disponible para el pasado. De aquí en adelante lo escribe el cierre.
update register_session_totals t
   set affects_cash_drawer = pm.affects_cash_drawer
  from payment_methods pm
 where pm.id = t.payment_method_id;

-- UN MÉTODO DE CAJÓN NO DECLARA UNA CIFRA PROPIA, y esto es la mitad de la regla que ningún código
-- puede garantizar solo. Si un camino futuro escribe un declarado distinto del esperado para un
-- método cuyo dinero está en el cajón, vuelve el faltante repartido que cancela sobrantes con
-- faltantes — el defecto que esta feature entera viene a cerrar.
--
-- `not valid` por lo mismo que el otro: los cortes ya cerrados tienen declarados tecleados por
-- método y no se van a reescribir.
alter table register_session_totals
  add constraint register_session_totals_cajon_no_se_declara check (
    not affects_cash_drawer or declared = expected
  ) not valid;

-- LA OTRA MITAD DE LA MISMA REGLA, EN EL CATÁLOGO: un método cuyo dinero se cuenta en el cajón no
-- se puede auto-declarar.
--
-- La regla ya vivía en Go, pero configurar un método es leer-validar-escribir y hasta esta feature
-- nadie podía hacerlo desde la aplicación. Ahora sí, y dos PATCH simultáneos —uno que enciende
-- «automático», otro «va al cajón»— validan cada uno contra el estado viejo y dejan escrita la
-- combinación imposible. El servicio ya toma el renglón con `for update`; esto es el respaldo que
-- no depende de que el siguiente camino se acuerde.
--
-- Auto-declarar significa "el servidor declara lo que él mismo espera": sobre el dinero del cajón
-- eso hace que la diferencia sea cero SIEMPRE y que un faltante real no se pueda detectar.
--
-- Validado y no `not valid`: se verificó que ningún método lo viola hoy, ni en la base de
-- desarrollo ni en el respaldo restaurado de producción.
alter table payment_methods
  add constraint payment_methods_cajon_no_se_autodeclara check (
    not (auto_declare and affects_cash_drawer)
  );

-- UN SOLO DUEÑO DEL FONDO POR EMPRESA.
--
-- El fondo de apertura y los movimientos de caja se le suman al renglón del método de tipo
-- `efectivo`, y el código asume que hay exactamente uno: "el ÚNICO al que pertenecen el fondo de
-- apertura y los movimientos". Con dos, el fondo se sumaría a los dos renglones — que es
-- literalmente el defecto que le inventó $4,500 de faltante a un turno, sumando el fondo una vez
-- por método que tocaba el cajón.
--
-- No había endpoint que creara métodos cuando eso se escribió, así que la suposición se sostenía
-- sola. La 015 abre la configuración de métodos desde la aplicación y es el momento de anclarla
-- donde no dependa de que nadie inserte una fila. Verificado: hoy hay exactamente uno por empresa,
-- en desarrollo y en el respaldo restaurado.
create unique index payment_methods_un_efectivo_por_empresa
  on payment_methods (company_id) where kind = 'efectivo';

-- ---------------------------------------------------------------------------------------------
-- 3. El arqueo ciego, como ajuste del negocio.
-- ---------------------------------------------------------------------------------------------

-- Quien cuenta el cajón no ve lo que el sistema espera, y la diferencia aparece después de
-- confirmar. Protege al negocio de que alguien acomode lo que declara para que cuadre.
--
-- NACE APAGADO. El comportamiento de hoy —la diferencia se ve antes de confirmar— es un requisito de
-- la spec 003 que ya está implementado y probado; una migración que lo invierta de golpe le mueve la
-- pantalla a quien opera sin que nadie lo haya pedido. Y encenderlo enmienda ese requisito, que es
-- una decisión del dueño y no un efecto de un deploy.
--
-- Un `boolean not null default false` con default constante no reescribe la tabla desde PG11: es
-- metadata. Va aquí y no en `cash_registers` ni en `users` porque es política del negocio: por caja
-- no significaría nada, y por usuario invitaría a exentarse a sí mismo.
alter table business_settings
  add column blind_cash_count boolean not null default false;

-- +goose Down

-- Revertir esto NO pierde dinero: los totales y los conteos siguen donde estaban. Lo que se pierde
-- es contra qué se comparó cada conteo y si ese método tocaba el cajón — o sea, la capacidad de
-- reconstruir un arqueo viejo tal como se firmó. Eso no se recupera volviendo a aplicar la
-- migración: el esperado de un turno cerrado ya no se puede recalcular sin mentir.
alter table business_settings drop column if exists blind_cash_count;
alter table payment_methods drop constraint if exists payment_methods_cajon_no_se_autodeclara;
drop index if exists payment_methods_un_efectivo_por_empresa;
alter table register_session_totals drop constraint if exists register_session_totals_cajon_no_se_declara;
alter table register_session_totals drop column if exists affects_cash_drawer;
alter table session_cash_counts drop constraint if exists session_cash_counts_expected_por_momento;
alter table session_cash_counts drop column if exists difference;
alter table session_cash_counts drop column if exists expected;
