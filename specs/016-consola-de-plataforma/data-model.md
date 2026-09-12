# Data model: la consola de plataforma

## `platform_operators` (nueva)

Quien vende y mantiene el producto. **Deliberadamente separada de `users`**: el login del negocio
consulta `users`, así que un operador no puede aparecer ahí ni por error.

| Columna | Tipo | Por qué |
|---|---|---|
| `id` | `bigserial` primary key | |
| `username` | `citext not null unique` | Único **global**, no por empresa: aquí no hay empresas |
| `name` | `text not null` | Para la bitácora, no para mostrar a nadie más |
| `password_hash` | `text not null` | `bcrypt`, con el mismo `auth.HashSecret` del resto |
| `is_active` | `boolean not null default true` | FR-013: retirar el acceso sin borrar el rastro |
| `created_at` | `timestamptz not null default now()` | |

**Sin `company_id`, y eso es el punto** (FR-002). No lleva `pin_hash`: el PIN existe para relevos en
el mostrador y aquí no hay mostrador.

**Sin RLS.** No es un descuido: RLS aísla *por empresa* y esta tabla no pertenece a ninguna. Lo que
la protege es que **el rol de la app no tiene ningún permiso sobre ella** — ver los grants.

## Rol de base `gatobobah_platform` (nuevo)

Lo que de verdad impone FR-005 y FR-006.

| Permiso | Sobre qué |
|---|---|
| `select` | `companies`, `platform_operators`, y las tablas de plataforma que lleguen después |
| **nada** | `orders`, `order_lines`, `order_payments`, `order_refunds`, `register_sessions`, `register_session_totals`, `session_cash_counts`, `expenses`, `products`, `users`… |

Y el que se olvida y no se nota: **`grant usage on schema public`**. Sin él, los `select`
explícitos no sirven de nada — es el mismo olvido que la 0024 documenta.

No se escribe como una lista de revokes sino como **grants explícitos sobre lo permitido**: una lista
de prohibiciones se queda corta el día que nace una tabla nueva; una de permisos, no. Es la
diferencia entre olvidarse de prohibir —que falla abierto— y olvidarse de permitir, que falla
cerrado y se nota enseguida.

**Nunca `grant ... on all tables` para este rol**, ni por atajo al agregar las tablas de la spec
017. Escribirlo una sola vez expondría `orders`, `users` y `order_payments` de golpe. Va como
comentario en la propia migración, porque es el atajo que alguien con prisa va a considerar.

Simétricamente, `gatobobah_app` **no recibe ningún permiso** sobre `platform_operators`, y esto no
hay que escribirlo: su grant fue puntual y ocurrió antes de que la tabla existiera. Verificado
contra Postgres — no hay default privileges en el clúster, así que una tabla nueva **no** queda
accesible por un `grant on all tables` anterior.

## El `Down` de esta migración no borra el rol

En Postgres los roles son del **servidor**, no de cada base: un rol con permisos en otra base del
mismo Postgres hace fallar `drop role`, y `drop owned by` no alcanza esos grants.

**Dónde falla y dónde no** (verificado el 2026-09-11, contando bases en cada Postgres):

| Dónde | Bases en ese Postgres | ¿Falla el `Down`? |
|---|---|---|
| CI | 1 (`gatobobah_test`) | No |
| Producción | 1 | **No** |
| VM de pruebas | 2 | Sí |
| Máquina de desarrollo | 5 | Sí |

O sea: **un `Down` que pasa en CI y funcionaría en producción, pero truena en la máquina de quien
programa**. Esa forma es peor de lo que parece: quien lo sufre concluye que su entorno está roto, no
que la migración lo está.

La 0024 ya arrastra ese defecto; esta migración no lo repite. El arreglo sirve en los cuatro
ambientes por igual, que es la razón de fondo: un `Down` correcto no depende de cuántas bases
tenga el servidor donde se ejecute.

El `Down` hace `alter role gatobobah_platform with nologin` y revoca lo que el `Up` otorgó. Eso
corta el acceso, que es lo único que importa al revertir. Borrar el rol del clúster es un paso
manual, cuando se confirme que ninguna otra base lo usa.

## Salud de una empresa (vista de lectura, sin tabla nueva)

Lo que la consola muestra por empresa sale de datos que ya existen:

| Dato | De dónde | Cuidado |
|---|---|---|
| Nombre y slug | `companies` | |
| Desde cuándo | `companies.created_at` | |
| Última actividad | El instante más reciente de una tabla de operación | **Requiere permiso de lectura sobre algo que los grants prohíben.** Ver abajo |
| Esquema al día | `goose_db_version` | **Es global a la base, no por empresa.** Ver abajo |

### Dos problemas que este modelo destapa, y que hay que resolver al implementar

**1. «Última actividad» choca con los grants.** Saber cuándo alguien usó el sistema exige mirar una
tabla de operación, y el rol de plataforma **no puede**. Las salidas, en orden de preferencia:

- Un **contador por empresa** que se actualiza al usar el sistema (una tabla de plataforma, que el
  rol sí puede leer). Es además exactamente lo que la spec 017 va a escribir, así que puede nacer
  ahí y no duplicarse.
- Conceder `select` sobre una vista agregada. Más superficie, y hay que probar que la vista no deja
  ver filas.

**Recomendación**: que «última actividad» **espere a la 017** y no se invente aquí un camino que
después se tira. En la primera versión, la salud se limita a lo que se puede leer sin abrir la mano.

**2. «Esquema al día» no es por empresa.** `goose_db_version` es de la base, no del cliente: hoy
todas las empresas comparten esquema y el dato sería **idéntico en todos los renglones**. Mostrar
una columna que dice lo mismo siempre es una pantalla que aparenta informar. O se muestra **una vez,
para la instalación**, o no se muestra.

**Recomendación**: mostrarlo como dato de la instalación, arriba, y no como columna por empresa.

Las dos son correcciones al FR-007 tal como está escrito, y conviene que queden en el spec antes de
implementar.
