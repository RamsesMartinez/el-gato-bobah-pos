# Phase 1 — Cómo se valida que esto funciona

**Feature** [020](spec.md). Escenarios corribles, no código. El detalle del esquema está en
[data-model.md](data-model.md) y el de los endpoints en [contracts/api.md](contracts/api.md).

## Prerrequisitos

1. **Credenciales del ambiente de pruebas de Uber** en `deploy/.env`:

   ```
   UBER_EATS_CLIENT_ID=…
   UBER_EATS_CLIENT_SECRET=…
   UBER_EATS_ENV=sandbox
   ```

   **`UBER_EATS_STORE_ID` se quita.** La tienda es una fila de `platform_connections`, no
   configuración: una empresa va a tener varias sucursales. Hoy no lo lee nada del código
   (verificado el 2026-09-14), así que borrarlo no rompe nada.

2. **Una base con datos reales** para lo que el harness de integración no puede ver:

   ```bash
   TEST_RESTORED_DATABASE_URL="postgres://gatobobah:gatobobah@localhost:5490/gatobobah_restaurado?sslmode=disable" \
     make respaldo-anonimo
   ```

   Trae dos empresas y el catálogo real. Con una sola empresa, todo camino «por cada otra empresa»
   es un no-op y la migración pasa verde para romper en producción.

   > **⚠️ `company_id = 1` NO es El Gato Bobah.** Es **«Bobah Pruebas»**, una empresa de prueba con
   > su propio catálogo, que además se parece mucho al bueno. El negocio real es **`company_id = 2`,
   > slug `gatobobah`**. Escribir `where company_id = 1` porque «es la primera» da un catálogo
   > plausible y equivocado: 172 productos en vez de 174, cero excepciones de precio en vez de 7, y
   > un catálogo que parece llevar meses sin tocarse cuando se editó anteayer. Ya pasó una vez, y no
   > falló nada — solo salieron números que no eran. Si una consulta filtra por empresa, que sea por
   > `slug = 'gatobobah'`, no por el id.

3. Las peticiones a mano, para ver el menú crudo sin levantar nada:
   [http/uber-eats.http](../../http/uber-eats.http) con la extensión REST Client.

## Los gates, antes de dar nada por bueno

```bash
cd server && go build ./... && go test ./...          # = make api-build && make api-test
cd server && go test -tags=integration ./internal/integration/...
cd web && bun run lint && bun run vitest run && bun run build
```

---

## 1. La migración, contra datos reales

```bash
cd server && TEST_RESTORED_DATABASE_URL=… go test -tags=integration -run TestMenusDePlataforma ./internal/integration/...
```

Qué tiene que probar, y por qué un unitario no puede:

| Caso | Por qué exige Postgres |
| --- | --- |
| Los cuatro `grant` a `gatobobah_app` | El de 0024 fue puntual; una tabla nueva no hereda nada. Falta uno y sale `42501` en producción, nunca en dev (que se conecta como owner) |
| Aislamiento entre empresas | Las políticas de RLS **no existen para el owner**. Corre bajo `appRoleStore` |
| Que la conexión de una empresa no empareje con un producto de otra | Los chequeos de FK de Postgres **saltan RLS** por diseño |
| Que `status='ok'` con `item_count=0` sea rechazado | Es un `check`, no hay función que llamar (FR-005) |
| Que dos tiendas de la misma plataforma quepan | Es el caso que la llave única tiene que permitir |

## 2. Que el emparejamiento no se borre por ninguno de sus DOS lados

**Los más importantes de todos**, porque su fallo es silencioso y semanas después. El
emparejamiento es trabajo humano que no se reconstruye, y tiene dos vecinos que podrían llevárselo.

```bash
go test -tags=integration -run 'TestElEmparejamientoSobreviveALaPoda|TestBorrarUnProductoEmparejadoFalla|TestLaPodaConservaLaUltimaLectura' ./internal/integration/...
```

| Prueba | Qué defecto atrapa |
| --- | --- |
| `TestElEmparejamientoSobreviveALaPoda` | Crea parejas, poda lecturas viejas, verifica que siguen. Falla si alguien pone una FK hacia `platform_menu_items`, diciendo **cuántas parejas se perdieron** |
| `TestBorrarUnProductoEmparejadoFalla` | `delete from products` sobre uno emparejado tiene que fallar por integridad (`on delete restrict`). Si vuelve a ser `cascade`, el reorg de datos de AGENTS.md §6 borra parejas sin avisar |
| `TestLaPodaConservaLaUltimaLectura` | Una conexión sin leer en más de la retención **conserva su última lectura**. Sin esto, «se leyó hace 4 meses» se vuelve «nunca se ha leído», y FR-003 pide distinguirlos |

## 3. Que nada pueda escribir en la plataforma (US4)

```bash
go test ./internal/uber/...
```

Dos pruebas distintas y las dos hacen falta:

- **`TestElTransporteRechazaTodoVerboQueNoSeaGET`** — le pide al cliente un `POST` contra el host
  de menú y verifica que falla **antes de abrir el socket**, nombrando la plataforma y la
  operación. Es la garantía: aunque la credencial tenga permisos de escritura concedidos por Uber,
  el sistema no ejerce ninguno (US4 ac.3).
- **`TestNingunaEscrituraEnElPaquete`** — parsea el AST de `internal/uber` y falla si aparece
  `http.MethodPost/Put/Patch/Delete` fuera del cliente de token. Es la alarma temprana: falla en
  `go test`, no en producción (FR-007, US4 ac.2).

## 4. La comparación, con tablas y sin base (US1)

```bash
go test ./internal/domain/ -run TestComparar
```

Los cinco escenarios del spec como casos de tabla, más los ocho edge cases. El que no puede
faltar, porque es el que invita a la acción destructiva:

> **Lectura vacía.** No produce «sobra todo el catálogo»: el servicio la rechaza antes de llegar a
> `Comparar`, con `ErrLecturaVacia`.

Y la frontera de dinero (principio III):

```bash
go test ./internal/domain/ -run TestPesosDeCentavos
```

`13000 → 130.00`, el cero, el tope de `MaxMoney`, y un valor que desborda → `ErrValidation`, no un
número raro.

## 5. Contra la plataforma de verdad

Lo que ningún simulador atrapa. Con el ambiente de pruebas de Uber ya activo:

```bash
# 1. Dar de alta la tienda (el store_id sale del request 2 de http/uber-eats.http)
curl -X POST …/admin/platform-menus/connections -d '{"platformId":2,"externalStoreId":"…","label":"Centro"}'

# 2. Disparar la lectura: tiene que responder 202 de inmediato, no esperar al menú
time curl -X POST …/admin/platform-menus/connections/1/read

# 3. Ver que terminó
curl …/admin/platform-menus/connections/1/reads
```

Qué se está verificando, con los números medidos el 2026-09-14:

| Se espera | Medido en la tienda real |
| --- | --- |
| El `POST` responde en milisegundos | El menú pesa **211 KB** sin comprimir; leerlo dentro del request rompería SC-006 |
| La lectura guarda **222 items** | 65 platillos + 157 opciones. Si guarda 233 está contando los 11 huérfanos; si guarda 21 platillos, cayó en la trampa del `type` ausente |
| Ningún `external_data` llega con contenido | Viene **vacío en los 233**. Un lector que espere encontrar ahí nuestro id no falla: empareja mal |
| Los ids se guardan completos | 59 llegan al tope de 20 caracteres y traen acentos y emoji |

## 6. El emparejamiento y la comparación, de punta a punta (US2 + US1)

1. `GET …/pairing` → propone las coincidencias exactas **marcadas como propuestas**. Con los datos
   de hoy son **6 de 65**; si propone muchas más, está emparejando por aproximación y eso está
   prohibido (FR-011).
2. Confirmar `Chamoyada_de_Mango` → producto `Chamoyada`, y después `Chamoyada_de_Fresa` → **el
   mismo producto**. Las dos tienen que entrar: es FR-010, y es el caso real (12 chamoyadas arriba,
   una abajo).
3. Intentar emparejar `Chamoyada_de_Mango` con un segundo producto → **409** (FR-012).
4. `GET …/differences` → la diferencia de precio sale contra `base × 1.35`, no contra el precio de
   mostrador (FR-017). `Chamoyada` base $57 → catálogo $76.95 contra los $99.00 publicados.
5. Renombrar el producto en el POS y volver a pedir → **la pareja sigue** (FR-014).
6. Deshacer y volver a pedir → aparece como sin emparejar **de inmediato** (FR-013, US2 ac.5).
7. `GET …/pairing?kind=opcion` → ahora la lista son **ingredientes**, y `unlinkedProducts` trae
   opciones de modificador del POS, no productos (FR-014a).
8. Emparejar un ingrediente con `localKind: "producto"` → **se rechaza**. Y con un `localKind`
   inventado, también: nunca cae a un default (FR-014b).

## 7. La pantalla, en el tamaño que existe (1024×600)

```bash
cd web && bun run dev     # y el navegador a 1024×600
```

El presupuesto real, medido en el repo: el rail de `AppShell` son 76 px y el `p={6}` de `Page` otros
48 → **900 × 552 px útiles**. La disposición está decidida en el punto 6 del plan; esto verifica que
se respetó.

- **El emparejamiento es UNA decisión a la vez, a ancho completo.** Dos listas lado a lado no caben:
  a ~418 px por columna, «Chamoyada de Mango», «de Mora» y «de Maracuyá» se truncan las tres a
  «Chamoyada de M…». Si ves dos columnas, la pantalla está mal antes de probarla.
- **Nada de `<select>` nativo.** Elegir plataforma o producto va con
  [Picker](../../web/src/components/Picker.tsx), que además trae el buscador — sin él, encontrar un
  producto entre 174 son hasta 19 pantallas de scroll **por cada una de las 59 parejas manuales**.
- **44 px de alto mínimo** en todo lo tappable, y el nombre del platillo **a dos líneas, nunca
  truncado**.
- **Confirmar avanza sola** al siguiente pendiente. Cuenta los taps de una pareja: si son más de
  dos en el caso con propuesta, sobra uno.
- **La lista de diferencias abre con `precio` y `disponibilidad`.** Los 109 «solo en un lado» van en
  otra pestaña con el conteo en la etiqueta. Cuenta los renglones visibles: tienen que quedar ~11,
  no ~8.
- **El texto es para quien opera el negocio.** «La lectura falló por tiempo agotado», no
  `failureKind: tiempo_agotado`. «ID de tienda en Uber Eats», no `externalStoreId`. Nada de nombres
  de endpoint ni de columnas en pantalla.
- **El borrado de una conexión no está junto a «Leer ahora».** Se lleva hasta 65 parejas; vive en
  configuración y su diálogo dice cuántas se pierden.

## 8. Lo que esta feature NO tiene que hacer, y conviene verificar que no hace

- No aparece ningún botón que cambie algo **en Uber**.
- No propone borrar nada a partir de una diferencia (FR-019).
- Una lectura corriendo no hace más lenta la captura de un pedido: abre el POS en otra pestaña
  mientras corre el paso 5 (SC-006).
