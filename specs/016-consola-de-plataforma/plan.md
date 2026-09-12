# Implementation Plan: La consola de plataforma, separada del negocio

**Branch**: `016-consola-de-plataforma` | **Date**: 2026-09-11 | **Spec**: [spec.md](./spec.md)

## Summary

Una superficie nueva —identidad, login, aplicación y subdominio propios— para quien **vende y
mantiene** el producto, sin ningún punto de contacto con la del negocio. Primera versión de **solo
lectura**: una pantalla con las empresas y su salud.

La decisión que ordena todo el diseño: **la separación no se impone con un `if`, se impone con tres
barreras independientes** —firma distinta, rol de base distinto y grants distintos—, cada una capaz
de detener por sí sola lo que las otras dejen pasar. Es lo que vuelve aceptable un acceso que cruza
empresas.

## Technical Context

**Lenguaje/versión**: Go 1.27 (backend), React 19 + Vite (front), ambos ya en el repo.
**Almacenamiento**: PostgreSQL 16, el mismo. Sin base nueva.
**Dependencias nuevas**: **ninguna**, ni en Go ni en el front. Principio VI y decisión del dueño.
**Pruebas**: `go test` (dominio), integración con `-tags=integration` contra Postgres real,
vitest y Playwright.
**Plataforma**: la consola se usa desde una **computadora**, no desde tableta. El presupuesto de
1024×600 no aplica aquí y por eso no se diseña contra él.
**Escala**: hoy 2 empresas. VM e2-micro, 1 GB, medida el 2026-09-11 con 602 MB de pico y 367 MB
libres.

## Constitution Check

| Principio | Cómo lo cumple este plan |
|---|---|
| **I. Layering** | `httpapi` (grupo `/platform`) → `app.PlatformService` → `store` con su propio pool. La regla de qué puede leer vive en `domain`, no en el handler |
| **II. Errores** | Sentinels nuevos en `domain`, envueltos con `%w`; el mapeo a HTTP sigue viviendo solo en `httpapi.Error` |
| **III. Dinero** | **No aplica y esa es la gracia**: la consola no toca dinero. FR-008 lo vuelve requisito |
| **IV. Test-first** | Cada barrera nace con su test, y los de aislamiento van a integración porque RLS y los grants **no existen para el owner**: un unitario los vería pasar siempre |
| **V. Seguridad** | Es el centro de la feature. Ver *Las tres barreras* |
| **VI. YAGNI** | Una pantalla, solo lectura, cero dependencias. Sin microfrontends: se evaluaron y se descartaron |
| **VII. Comentarios** | El porqué de cada barrera va en el código, no en un documento aparte |
| **VIII. Puertas** | Ver *Qué puerta abre y cuál no cierra* |

### Las tres barreras

Lo que hace que esto sea defendible y no un `if`:

| # | Barrera | Qué detiene sola |
|---|---|---|
| 1 | **Firma distinta del token.** La consola firma con su propio secreto | Un token de negocio **no valida** contra la API de la consola, ni al revés. No es una comparación de campos: es criptografía. Cierra FR-003 y FR-004 aunque el código de roles esté mal |
| 2 | **Rol de base propio** (`gatobobah_platform`), con su propio pool | Aunque un handler leyera mal, la conexión que usa **no tiene permiso** sobre las tablas de operación |
| 3 | **Grants explícitos, y nada más** | `select` sobre `companies` y sobre las tablas de plataforma. **Cero** sobre `orders`, `order_payments`, `register_sessions`, `expenses` y las demás. Postgres responde `42501` |

**Y una política de RLS acotada, que el plan original no tenía.** `/speckit-analyze` encontró que el
grant **no basta**: `companies` lleva la política `company_self` y RLS también le aplica al rol de
plataforma, así que con `select` a secas la consola vería **una empresa de dos** — FR-007 imposible.
Se abre con una política de una línea, para **una tabla, un comando y un rol**, verificada contra
datos reales: el rol pasa a ver 2 de 2 y el de la app sigue viendo solo la suya.

Lo que **no** se hace es darle `BYPASSRLS`: resolvería esto y abriría todo lo demás el día que
alguien agregue un grant por comodidad. Además el arranque lo rechaza a propósito.

La barrera 2 y la 3 son la respuesta a FR-006. No son teoría: el repo ya vive esta forma —el binario
abre **dos** pools hoy, el del owner y el de `gatobobah_app`— y esta feature agrega el tercero.

**Por qué la barrera 1 y no solo comparar un campo del token.** Un claim `kind: "platform"` que el
middleware compara es un `if` más, y el propio spec nació de no querer depender de un `if`. Con dos
secretos, el ataque «usar el token de allá acá» no se rechaza: **no se puede construir**.

### Qué puerta abre y cuál no cierra

| Puerta | Estado después de este plan |
|---|---|
| **Varios operadores de plataforma** | Abierta: la tabla es de operadores, no una fila especial |
| **Suscripciones y vigencias** | Abierta: cuelgan de `companies`, que la consola ya lee |
| **Acciones de soporte (spec 018)** | Abierta y **cerrada con llave a propósito**: los grants son de solo lectura, así que construirla exige un cambio deliberado de permisos. Es la puerta que uno *quiere* que cueste abrir |
| **Mapa de calor (spec 017)** | Abierta: sus tablas nacerán en el mismo grupo de grants |
| **Que un operador vea datos de operación** | **Cerrada a propósito.** Si algún día hiciera falta, es una decisión nueva con su propia revisión, no un olvido |

Lo que este plan **no** decide y deja abierto: si más adelante la consola necesita escribir, el
camino es un rol de base distinto para esa operación — no ampliar los grants de éste.

## Project Structure

### Documentation (this feature)

```
specs/016-consola-de-plataforma/
├── spec.md
├── plan.md            # este archivo
├── research.md        # las decisiones y lo que se descartó
├── data-model.md      # operadores de plataforma y salud de empresa
├── contracts/api.md   # el grupo /platform
└── quickstart.md      # cómo comprobar que las tres barreras están puestas
```

### Source Code

```
server/
├── migrations/0068_consola_de_plataforma.sql   # tabla, rol de base y grants
├── queries/platform.sql                        # solo lectura
├── internal/
│   ├── domain/plataforma.go                    # qué es un operador y qué puede ver
│   ├── auth/                                   # segundo manager de JWT (otro secreto)
│   ├── app/plataforma.go                       # PlatformService
│   ├── httpapi/handlers_plataforma.go
│   └── integration/                            # las tres barreras, contra Postgres real
└── cmd/api/main.go                             # tercer pool + flag para crear al primer operador

web/
├── vite.config.ts                              # el build del POS, sin tocar
├── vite.consola.config.ts                      # SEGUNDO build: outDir 'dist-consola', publicDir propio
├── tsconfig.json                               # el del POS, EXCLUYE src/consola
├── tsconfig.consola.json                       # el de la consola
├── eslint.config.js                            # regla nueva: nadie importa consola/ desde fuera
└── src/
    ├── (el POS, sin tocar)
    └── consola/                                # la consola: su propio árbol y su propia entrada
```

**Una sola instalación de dependencias, dos builds.** No se crea un proyecto de npm aparte: sería
un segundo `node_modules`, un segundo lockfile y un segundo trabajo de CI para no ganar nada. Dos
configuraciones de Vite con **entradas distintas** producen **paquetes distintos**, que es lo que
FR-011 pide. Y como eso depende de que nadie importe de un árbol al otro por descuido, el propio
paquete se revisa: ver *quickstart*.

## Lo que la revisión de arquitectura cambió

Seis hallazgos, todos con su escenario. Estos son los que movieron el diseño:

### 1. El `Down` del rol de base NO es reversible aquí (verificado contra Postgres real)

Iba a copiar el `drop owned by` + `drop role` de la 0024. **En Postgres los roles son objetos del
servidor, no de cada base**: si el rol tiene permisos en **otra** base del mismo Postgres, el
`drop role` falla, y `drop owned by` no alcanza esos grants —están fuera de su alcance por diseño—.

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

**La 0024 ya arrastra este defecto hoy.** No se toca —está aplicada—, pero la 0068 no lo repite.

**Decisión**: el `Down` **no borra el rol**. Hace `alter role gatobobah_platform with nologin` y
revoca explícitamente lo que esta migración otorgó. Eso corta el acceso de inmediato, que es lo
único que importa en un rollback de emergencia; borrar el rol del clúster queda como paso manual
cuando se confirme que ninguna otra base lo usa, y va dicho en el comentario del `Down`.

**Y algo que conviene saber del rollback**: `drop role` tiene éxito aunque el rol tenga una sesión
abierta. Revertir **no corta conexiones vivas**, solo impide nuevas.

### 2. Falta el chequeo de arranque, y el repo ya tiene el patrón

`main.go` ya trae `assertRLSEnforced`: aborta el arranque si el rol de servicio resulta ser
superusuario o tiene `bypassrls`, y comprueba **funcionalmente** que una consulta sin tenant
devuelva cero filas. Existe porque una variable mal puesta en producción no debe descubrirse por
accidente.

Yo solo había puesto la verificación en el quickstart, o sea a mano. **Escenario concreto**: si
`PLATFORM_DATABASE_URL` quedara apuntando al owner por un copy-paste de `DATABASE_URL`, el tercer
pool serviría con bypass total y nadie lo notaría.

**Decisión**: `assertPlatformGrantsEnforced` en el mismo lugar y con la misma forma — comprobar
`not (rolsuper or rolbypassrls)` y que un `select` canario sobre `orders` falle con `42501`, antes
de servir tráfico.

### 3. El chequeo anti-contaminación del front era malo

Había propuesto un `grep` sobre el paquete construido. Dos fallas: **no corre solo** —nadie lo
ejecuta en cada cambio— y busca texto literal en un paquete minificado, así que puede decir
«limpio» con la consola dentro.

**El repo ya tiene el mecanismo correcto y automático**: `eslint.config.js` trae un bloque
`FRONTERAS` con `no-restricted-imports` que ya bloquea imports cruzados entre carpetas, y corre en
cada commit y en CI.

**Decisión**: una regla ahí que prohíba importar `**/consola/**` desde fuera de `src/consola/`. El
`grep` del quickstart se queda como verificación de lo que de verdad se sirve, no como única
barrera.

### 4. Un error de TypeScript en la consola bloquearía el deploy del POS

`tsconfig.json` incluye todo `src/`, y `bun run build` —que es `tsc --noEmit && vite build`— es
justo el script del que dependen los despliegues del punto de venta.

**Escenario concreto**: un typo en una pantalla que ni siquiera corre en tableta deja **varado un
arreglo urgente de cobro**.

**Decisión**: `tsconfig` separados. El del POS excluye `src/consola`; la consola tiene el suyo.

### 5. El segundo build podía borrar el paquete del POS

No había fijado `outDir`. El default de Vite es `dist/` con `emptyOutDir`, o sea **el mismo
directorio** que CI acaba de llenar para el POS y está por subir a Pages.

**Escenario concreto**: según el orden de los pasos, se despliega el paquete equivocado a uno de los
dos proyectos, y ningún test lo nota — los e2e corren contra lo ya desplegado.

**Decisión**: `outDir: 'dist-consola'`, explícito en el plan y no como detalle de implementación.

### 6. La consola heredaría la identidad del POS

`publicDir` compartido copia `manifest.json` y `_headers` a cualquier build. La consola quedaría
instalable como PWA **con el nombre y el logo del restaurante**, y con reglas de caché pensadas para
el service worker del POS.

**Decisión**: `publicDir` propio en la configuración de la consola.

### Lo que la revisión confirmó, sin objeción

- **Grants explícitos y no `on all tables`**: correcto, verificado contra Postgres — no hay default
  privileges en el clúster y una tabla creada después de un `grant on all tables` **no** queda
  accesible. El patrón de grants puntuales que el repo usa desde la 0025 es el correcto.
- **La simetría sale gratis**: `gatobobah_app` no obtiene acceso a `platform_operators` porque su
  grant fue puntual y ocurrió antes de que la tabla existiera. No hace falta revocar nada.
- **Sin RLS en `platform_operators`**: correcto. RLS particiona por empresa y esa tabla no tiene ese
  eje; la barrera real es el grant.
- **`staff.` en vez de `admin.`**: confirmado con evidencia — `roles.ts` define `admin` como rol de
  negocio real. La misma palabra para dos cosas es la ambigüedad que ya costó en el cierre de caja.
- **Ninguna puerta del principio VIII se cierra.**

### La medición «antes» del paquete del POS

Sin ella no hay forma de saber después si la consola se coló. Medido el 2026-09-11:

| | |
|---|---|
| Chunk principal | **1,133.50 kB** (322.90 gzip) |
| `dist/` completo | **1.5 MB** |
| Precache de la PWA | 16 entradas, **1,430.58 KiB** |
| CSS | 2.47 kB |

Si el chunk principal crece al aterrizar esta feature, es contaminación — y esa señal es más clara
que cualquier `grep`.

## Decisiones que este plan toma

### 1. El operador vive en su propia tabla, sin empresa

`platform_operators`, separada de `users`. No es una fila con bandera: el login del negocio consulta
`users` y **ahí no está**, así que FR-001 se cumple por construcción y no por un `where`.

### 2. Dos secretos, dos managers de JWT

El secreto de la consola es una variable de entorno nueva, validada al arranque con la misma vara
que `JWT_SECRET` —`config.Validate` ya rechaza secretos débiles y placeholders, y la API **no
arranca** si no pasan—. Si falta o es igual al del negocio, no arranca: dos secretos iguales son un
secreto.

### 3. Tercer pool, tercer rol de base

`PLATFORM_DATABASE_URL` conecta como `gatobobah_platform`. En desarrollo puede caer al owner igual
que hoy hace el rol de app —y por eso **el aislamiento se prueba en integración**, donde sí hay
roles de verdad; en local, conectados como owner, estos tests pasarían siempre—.

### 4. Rutas bajo `/api/v1/platform`, con su propio middleware

Ni una ruta nueva dentro del grupo del negocio. El middleware de la consola valida con el manager de
la consola y no conoce empresas.

### 5. El primer operador se crea con una bandera del binario

Como ya se hace con `-reset-admin`: lee variables de entorno, no expone pantalla y no lleva
credenciales en el repositorio. Resuelve FR-014 con el patrón que el repo ya tiene.

### 6. Subdominios

`staff-dev.elgatobobah.com` y `staff.elgatobobah.com`, cada uno con su proyecto de Pages, igual que
hoy `app-dev` es un proyecto distinto de `app`. Se propone **`staff`** y no `admin` a propósito:
«admin» ya es un **rol de negocio** en este producto, y llamar igual a dos cosas distintas es la
clase de ambigüedad que ya costó caro en el cierre de caja.

## Lo que se midió antes de escribir esto

| Qué | Medido |
|---|---|
| Conexiones con rol distinto que ya abre el binario | **2** (owner y `gatobobah_app`) — esta feature agrega la tercera, no inventa el patrón |
| Empresas en producción | **2** |
| Memoria libre bajo carga en la VM | **367 MB** de 969, sin swap |
| Proyectos de Pages que ya existen | **2**, uno por ambiente |

## Complexity Tracking

| Qué agrega complejidad | Por qué se acepta |
|---|---|
| Un tercer pool y un tercer rol de base | Es la barrera que vuelve aceptable cruzar empresas. Sin ella, la consola es la llave maestra de todos los negocios |
| Un segundo secreto de JWT | Convierte «usar el token de allá acá» de *rechazado* en *imposible de construir*. Una variable de entorno más |
| Un segundo build del front | Es lo que garantiza que la tableta no descargue la consola, que el dueño pidió explícitamente |

Nada de esto es especulativo: los tres salen de un requisito escrito.
