# docs/ — índice

Qué hay aquí y **cuál sigue vigente**. Las reglas de ingeniería no viven en `docs/`: los
principios están en [`.specify/memory/constitution.md`](../.specify/memory/constitution.md) y la
mecánica del repo en [`AGENTS.md`](../AGENTS.md).

## Referencia viva — consúltala antes de tocar el área

| Documento | Para qué |
|---|---|
| [security-owasp.md](security-owasp.md) | Auditoría OWASP + ronda adversarial (2026-07-19) y los controles que dejó. Es el respaldo del **principio V** de la constitución y la vara del subagente `security-auditor`. Léelo antes de tocar auth, config, middleware o logging. |
| [email-zoho.md](email-zoho.md) | Runbook de SMTP: Mailpit en local, Zoho en producción, y qué pasa cuando `SMTP_HOST` está vacío (hoy en prod lo está: la recuperación por correo está apagada). |
| [impresion-tickets.md](impresion-tickets.md) | Runbook de impresión: ajustes del driver térmico, navegador en modo impresión directa y el catálogo de síntomas cuando el papel sale mal (en blanco, tenido, hoja larguísima). El contenido del ticket se configura dentro del sistema, en **Impresión**. |
| [auditoria/](auditoria/) | Los hallazgos de la auditoría adversarial de los precios por plataforma y los diagramas de por qué 47 bytes de JSON quemaban 25 segundos de CPU. Léelo antes de tocar `domain.Round2`/`ValidMoney` o de agregar una llave foránea a una tabla per-tenant: explica por qué la guarda mira el exponente y no el valor, y por qué los chequeos de llave foránea saltan RLS. |
| [corte-produccion/](corte-produccion/) | Cómo quedaron partidas las dos empresas al pasar el POS a principal (`gatobobah` = producción, `bobah-pruebas` = histórico de pruebas), qué se copió y qué no, y los scripts con su rollback. Léelo antes de tocar datos de un tenant o de abrir otra sucursal: documenta los cuatro fallos que rompen un copiado de catálogo. |

## Histórico — se conserva, **no** se sigue

| Documento | Qué fue |
|---|---|
| [design/](design/) | Los 4 planes de la construcción inicial (plan maestro con fases F0–Fn, modelo de dominio, arquitectura del backend, UX del POS). Se ejecutaron; el sistema está en producción desde 2026-07-22. Sirven para entender **por qué** el diseño es como es — no para saber cómo está hoy. Detalles como "React 18 + Chakra v2" o "tablets 8–10\"" ya no aplican: la verdad actual es el código, `AGENTS.md` y la constitución. |
| [reorg/](reorg/) | Reorganización del menú migrado de FUDO: SQL numerado `NN_*.sql` con su `NN_rollback.sql` y los CSV de análisis. Ya aplicados en producción. El patrón (migración numerada + rollback gemelo) es el que se sigue usando para cambios de datos del menú. Incluye [`16_NOTAS_porciones_recetas.md`](reorg/16_NOTAS_porciones_recetas.md), una decisión **aplazada** sobre porciones por variante que sigue pendiente. |

## Fixtures, no documentación

`tickets/` son documentos de compra reales (PDF) que usan como casos de prueba
[`purchasedoc_test.go`](../server/internal/domain/purchasedoc_test.go),
[`expenseDraft.test.ts`](../web/src/features/backoffice/expenseDraft.test.ts) y la herramienta
[`parse-doc`](../server/cmd/parse-doc/main.go) (`make parse-doc f=docs/tickets/ticket.pdf`). Los
tres tienen estructuras distintas a propósito. **No los borres ni los renombres**: hay tests que
dependen de esos nombres.

## Dónde va un documento nuevo

- **Spec de una feature nueva** → `specs/NNN-slug/`, generado por spec-kit (`/speckit-specify`). No a mano, y no aquí.
- **Un principio de ingeniería** → la constitución, subiendo su versión.
- **Un comando, puerto o quirk de herramienta** → `AGENTS.md`.
- **Un runbook operativo** (cómo se configura X en producción) → aquí, y agrégalo a la tabla de arriba.
