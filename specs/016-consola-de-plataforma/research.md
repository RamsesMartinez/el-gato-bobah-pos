# Research: la consola de plataforma

Lo que se decidió, contra qué se comparó y qué se descartó.

## 1. Dónde vive el operador de plataforma

**Decisión**: tabla propia `platform_operators`, sin `company_id`.

**Rationale**: el login del negocio consulta `users`. Si el operador no está ahí, FR-001 se cumple
**por construcción**, no por un `where` que alguien pueda olvidar al escribir la siguiente consulta.

**Alternativas consideradas**:

| Alternativa | Por qué no |
|---|---|
| Rol nuevo en `users` con empresa «plataforma» | Queda al lado de los usuarios de cada cliente. La separación pasa a depender de que ninguna de las decenas de consultas sobre `users` se olvide de filtrar |
| `users` con `company_id` nulo | Rompe el supuesto de que todo usuario tiene empresa, que hoy sostiene RLS y el middleware de tenant. Un nulo ahí se propaga a lugares que nadie va a revisar |

## 2. Cómo se separan las sesiones

**Decisión**: **dos secretos de firma**. La consola firma y valida con `PLATFORM_JWT_SECRET`; el
negocio sigue con `JWT_SECRET`. `config.Validate` rechaza al arranque que falte, que sea débil o
**que sea igual** al del negocio.

**Rationale**: con un solo secreto y un claim que distinga, el ataque «usar el token de allá acá» se
**rechaza con un `if`**; con dos secretos **no se puede construir**. El spec nació justamente de no
querer que la separación fuera una bandera.

Que sean distintos hay que comprobarlo al arranque y no confiar en la configuración: dos secretos
iguales son un secreto, y el error es silencioso.

**Alternativas consideradas**:

| Alternativa | Por qué no |
|---|---|
| Un claim `kind: "platform"` con el mismo secreto | Un `if` más, exactamente lo que el spec evita. Y un defecto al leerlo abre las dos superficies a la vez |
| Cookies de dominio distinto | El acceso ya vive en memoria a propósito (para que un XSS no lo lea); cambiar eso es un rediseño del login, no una separación |

## 3. Cómo se impone en la base (FR-006)

**Decisión**: rol `gatobobah_platform` con `select` **solo** sobre `companies` y las tablas de
plataforma. Ningún permiso sobre las tablas de operación. Un tercer pool en el binario.

**Rationale**: es el patrón que el repo **ya usa** — el binario abre hoy dos pools, el del owner y
el de `gatobobah_app`, y RLS es la barrera del segundo. Aquí la barrera es el grant: si un handler
se equivoca, Postgres responde `42501` y no hay fuga.

La constitución ya nombra por qué esto no se prueba en local: *«la API de desarrollo se conecta como
owner y sin `APP_DATABASE_URL`: ahí RLS y los grants sencillamente no aplican»*. Por eso **estas
pruebas van a integración**, bajo el rol de verdad. Un unitario las vería pasar siempre.

**Alternativas consideradas**:

| Alternativa | Por qué no |
|---|---|
| Solo comprobar en el servicio | Es la clase de chequeo que el camino nuevo se salta. El repo ya lo aprendió con el PIN débil, que se validaba en `SetPIN` y `Create` lo aceptaba |
| Vistas que expongan solo lo permitido | Más superficie, mismo efecto. Los grants ya dicen exactamente lo mismo con menos piezas |
| Base de datos aparte | Habría que replicar `companies` y mantener dos verdades. Más caro y peor |

## 4. Una app aparte, sin microfrontends

**Decisión**: segunda configuración de Vite con entrada y salida propias, dentro de `web/`. Un solo
`node_modules`, dos paquetes, dos proyectos de Pages.

**Rationale**: lo que el dueño pidió es que **la tableta no descargue la consola**, y eso lo da una
entrada distinta. Lo demás es costo sin beneficio.

**Alternativas consideradas**:

| Alternativa | Por qué no |
|---|---|
| **Microfrontends** (Module Federation) | Resuelven que varios equipos desplieguen sin coordinarse; aquí hay una persona. Negocian dependencias en tiempo de ejecución y **duplican React y Chakra** salvo configuración cuidadosa: pesan más, no menos. Y traen dependencias nuevas a un repo donde un CVE bloquea el merge |
| Proyecto de npm aparte | Segundo `node_modules`, segundo lockfile, segundo trabajo de CI. No compra nada que la segunda entrada no dé |
| Misma aplicación con rutas ocultas | El código viaja a la tableta aunque no se pueda navegar. Incumple FR-011 y deja la consola a la vista de cualquiera que abra el paquete |

**El riesgo que esta decisión sí tiene**: un `import` descuidado de un árbol al otro mete la consola
en el paquete del POS sin que nadie lo note. Por eso FR-011 no se da por bueno razonando — se
**revisa el paquete**, y así está puesto en el quickstart.

## 5. Cómo nace el primer operador

**Decisión**: una bandera del binario, como el `-reset-admin` que ya existe: lee variables de
entorno, no expone pantalla y no deja credenciales en el repositorio.

**Rationale**: el patrón ya está y ya se usó hoy para restaurar el admin de dev. Reusarlo es cero
superficie nueva.

**Lo que hay que evitar, y que el código actual ya tropezó**: `-reset-admin` filtra por `username`
sin acotar empresa, y su propio comentario lo advierte para el caso multi-empresa. La bandera nueva
no debe copiar esa forma — busca en `platform_operators`, donde no hay empresa.

## 6. El nombre del subdominio

**Decisión**: `staff-dev.elgatobobah.com` y `staff.elgatobobah.com`.

**Rationale**: «admin» ya es un **rol de negocio** en este producto. Llamar `admin.` a la consola
haría que la misma palabra signifique dos cosas, que es la clase de ambigüedad que en el cierre de
caja ya costó: por eso «Va al cajón» no reusa «Automático».

**Alternativa considerada**: `admin.` — descartada por lo anterior.

## Lo que NO se resolvió aquí, y toca al implementar

- **El tiempo de vida del acceso en la consola.** FR-013 exige que desactivar corte el acceso sin
  esperar a que caduque la sesión; hay que elegir entre un acceso corto o una comprobación por
  petición, y medir qué cuesta cada una.
- **Qué significa exactamente «esquema al día»** en la salud de una empresa. Hoy la versión de
  migración es global a la base, no por empresa: el dato existe, pero hay que decidir qué se muestra
  para que no diga una cosa por otra.
