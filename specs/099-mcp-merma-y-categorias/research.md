# Research — 099 merma esperada y categorías desde el MCP

## R1. Herramienta propia para la merma, no un campo de `ingredient_update`

- **Decisión**: `ingredient_expected_waste_set`, herramienta aparte con `reason` obligatorio.
- **Por qué**: la pantalla protege la merma del artículo con `menu.configure`
  (`PUT /stock/articles/{id}/expected-waste`), y `ingredient_update` cuelga de
  `inventory.configure`. Cada herramienta del catálogo declara **un** permiso y
  `TestEveryToolMatchesItsRouteGate` exige que sea el de su ruta: meterla en `ingredient_update`
  obligaría a una herramienta con dos permisos, o a que alguien con `inventory.configure` cambiara el
  costeo sin el permiso de la pantalla. Además el motivo y la vista previa de costos son propios de
  este cambio (como `ingredient_cost_set`).
- **Alcance del token**: `ScopeWriteCatalog`, el de todas las escrituras con `menu.configure`.
- **Alternativas**: campo en `ingredient_update` (rechazada por el permiso); herramienta genérica
  para merma de insumo, producto y general (rechazada: YAGNI, solo se pidió la del insumo).

## R2. Nulo contra cero en la entrada

- **Decisión**: dos campos excluyentes: `pct` (texto decimal `^[0-9]{1,2}(\.[0-9]{1,2})?$`) o
  `clear: true`. Exactamente uno; ninguno o los dos → error de argumentos antes de despachar.
- **Por qué**: un `pct` ausente u omitido por el agente no puede significar «quitar la captura»
  (constitución V: el default es para el ausente, y aquí el ausente es un error). `"0"` es 0 %. Es el
  mismo contrato que la ruta (`{"pct": null}` explícito).
- **Rechazada**: `pct: null` en JSON — muchos clientes MCP no distinguen null de ausente.

## R3. Vista previa con costo antes y después sin escribir

- **Decisión**: ruta nueva de solo lectura `GET /stock/articles/{id}/expected-waste/preview?pct=8`
  (o `?pct=none`), `menu.configure`. El servicio carga el grafo de costeo (`loadGraph`, el mismo del
  recálculo), lo copia con la merma del insumo cambiada (`CostGraph.WithIngredientWaste`) y una
  función pura de dominio (`ExpectedWasteImpact`) devuelve, para cada producto, extra y preparado
  que usa el insumo (directo o a través de preparados), el costo antes y después.
- **Por qué**: correr el cambio dentro de una transacción y revertirla tomaría el candado de
  composición (`LockCompositionEdits`) y escribiría costos solo para deshacerlos; el grafo en memoria
  da el mismo número sin tocar la base, y es lógica pura, probable en `domain`.
- **Costo efectivo del insumo**: si queda capturado, costo × (1 + %/100). Si queda «sin captura», no
  hay un número único: depende del producto (su merma o la general). La vista previa dice
  `effective_cost: null` con la fuente (`product_or_general`) y los costos van por producto.
- **Usos**: «usa el insumo» es transitivo por preparados; un producto con costo manual lo usa pero su
  costo no cambia (la vista previa lo marca `cost_source: manual`).

## R4. El motivo de la merma en la bitácora

- **Decisión**: la ruta `PUT /stock/articles/{id}/expected-waste` acepta un `reason` opcional (1 a
  200 letras tras quitar espacios); si llega, va en el `after` del evento
  `ingredient.expected_waste`. La herramienta MCP lo exige; la pantalla no lo manda y sigue igual.
- **Origen**: la bitácora ya guarda dispositivo/origen del MCP (`OriginFrom` de la sesión MCP) y el
  usuario dueño del token; no hace falta nada nuevo.
- **Mismo valor**: `samePct` ya evita el renglón cuando no cambia; la vista previa lo avisa.

## R5. Reglas del árbol de categorías

- **Hecho medido**: la pantalla de venta (`CategoryRail.tsx`) pinta raíces y las subcategorías de
  la raíz elegida: **dos niveles**. Un tercer nivel desaparecería de la venta sin error.
- **Decisión** (función pura `domain.CheckCategoryChange`):
  - nombre: se quitan espacios; 1 a 60 caracteres;
  - padre: debe existir en la empresa, ser raíz (sin padre) y no ser la propia categoría;
  - una categoría con subcategorías (activas o no) no puede recibir padre;
  - una categoría activa no puede quedar bajo un padre inactivo (crear, mover o reactivar);
  - desactivar se niega con productos activos (cuenta) o subcategorías activas (nombres).
  - Con «el padre es raíz» y «quien tiene hijos no recibe padre» no existen ciclos.
- **Predicado de «producto activo»**: `is_active and system_kind is null`, el mismo que cuenta la
  lista de categorías (`AdminCategoryList.product_count`): lista y regla del mismo predicado
  (constitución III, corolario).
- **Alternativas**: desactivar en cascada subcategorías o productos (rechazada: un toque del agente
  apagaría media carta sin que la persona lo vea); permitir N niveles (rechazada: la venta no los
  pinta).

## R6. Padre de otra empresa

- **Hecho**: `categories.parent_id references categories(id)` es de una columna; una FK salta RLS,
  así que la base aceptaría un padre de otra empresa (es la familia del #344, que decide el dueño).
- **Decisión**: dos barreras sin migración: (1) el chequeo de refs del MCP (#335) rechaza el id antes
  de la vista previa; (2) el servicio lee el padre `for update` como `gatobobah_app` (RLS) y, si no
  aparece, responde «no existe». La FK compuesta queda para el #344.

## R7. Concurrencia

- **Un solo candado, en orden de id**: el servicio lee la categoría y su padre (actual o nuevo) en
  una sola consulta `where id = any(@ids) order by id for update`. Mover A bajo B y B bajo A a la vez
  con candados en orden distinto daba deadlock (40P01, revisión de arquitectura); con orden fijo, la
  segunda espera y después falla limpio por la regla («quien tiene hijos no recibe padre»). Test:
  dos `UpdateCategory` cruzados en goroutines bajo `appRoleStore`.
- Desactivar: con la categoría bloqueada se cuentan hijos y productos activos; un hijo nuevo bajo
  ella también la bloquea (es su padre), así que no puede colarse.
- Un `product_create` concurrente en una categoría que se está apagando toma `FOR KEY SHARE` por la
  FK, espera el commit de quien la apaga e inserta en una categoría ya inactiva; reactivar un
  producto no pasa por la FK. Queda posible un producto activo en categoría inactiva. Se acepta (lo
  hace una persona, caso raro) y no se acopla `product_create` a un candado nuevo; se anota en
  `docs/mecanica/catalogo-y-sucursales.md`, y la lista con `status=all` lo deja ver (cuenta de
  productos activos en una inactiva).
- Las reglas viven solo en el servicio: una escritura por SQL manual o el importador de FUDO no las
  aplican. `CheckCategoryChange` corre en toda ruta de la API que escribe `categories` (alta y PATCH;
  la familia no toca árbol ni estado).

## R8. Nombre repetido

- `categories_name_scope` es único `(company_id, coalesce(parent_id,0), name)` con `name citext`:
  no distingue mayúsculas. El servicio busca primero el homónimo con
  `parent_id is not distinct from @parent` (un `=` no encuentra raíces) y `id <> @self` al editar
  (renombrar sin cambiar no choca consigo misma), para decir su id y si está inactivo.
- La carrera entre dos altas la cubre el índice: un `23505` cuyo `ConstraintName` es
  `categories_name_scope` (no cualquier 23505) se traduce a `ErrCategoryNameTaken` y se devuelve de
  inmediato: la transacción ya está abortada y escribir la bitácora daría `25P02`.

## R8b. Índices

- No hacen falta. Hijos activos: decenas de filas por empresa, entra por `categories_name_scope`
  (prefijo `company_id` de RLS). Productos activos: `products_category (category_id) where
  is_active`. `inactive_product_count`: scan de ~170 productos en una lista de admin que no corre en
  caliente. Si el catálogo llegara a miles, esa agregación se revisa primero con `explain`.

## R9. Refresco del menú

- Las tablas `categories` tienen los triggers de `catalog_versions` (0093): cualquier insert/update
  sube la versión y el POS y el menú público vuelven a leer. No hace falta invalidar a mano.

## R10. Lista con inactivas

- `GET /admin/categories?status=all` agrega las inactivas con `isActive` e `inactiveProductCount`.
  Sin parámetro, igual que hoy (solo activas; la pantalla no cambia). Cualquier otro valor → 400.
- `categories_list` del MCP gana `include_inactive` (bool).
