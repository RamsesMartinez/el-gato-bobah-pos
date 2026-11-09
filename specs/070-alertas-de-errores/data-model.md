# Data Model

Nada en Postgres. Dos estructuras en memoria y dos llaves en Redis.

## domain.ServerAlert

| Campo | Tipo | Nota |
| --- | --- | --- |
| At | time.Time | se pinta en America/Mexico_City |
| Method, Route | string | Route = patrón de chi, «(sin ruta)» si vacío |
| RequestID | string | |
| UserID, UserName | int64, string | 0 = sin sesión |
| CompanyID, CompanyName | int64, string | nombre lo llena el notifier |
| Kind | string | tipo de error (R3) |
| Message | string | ya enmascarado y recortado |
| Panic | bool | |
| Frames | []string | `paquete.Func archivo:línea`, solo del proyecto, ≤ 12 |

`AlertKey(a) = Method + " " + Route + " " + Kind` (sha1 hex en la llave de Redis para acotar largo).

## Redis

| Llave | Valor | Vence | Quién escribe |
| --- | --- | --- | --- |
| `pos:alert:win:<sha1>` | id del proceso dueño | 15 min 30 s | SET NX (ganar), SET XX (renovar), DEL condicionado |
| `pos:alert:count:<sha1>` | repeticiones sin reportar | 1 h | INCR (no dueño), GETDEL (dueño) |

## Estados de una ventana

`sin ventana` → (error) → `abierta, mía` [correo inmediato] → (15 min) → conteo > 0 ? [resumen,
renovar] : `sin ventana`. En otro proceso: `abierta, ajena` → (error) → INCR.
