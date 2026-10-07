# Respaldos y horario de producción

Runbook de `pos-vps` (zona `us-central1-a`, proyecto `el-gato-bobah-pos`). Configurado el
2026-09-29. **Hasta ese día producción no tenía ningún respaldo automático**: ni cron, ni snapshot
programado, ni bucket. Los únicos respaldos eran los que se tomaban a mano antes de una migración.

## 1. La noche, en orden (hora del centro de México, UTC−6 todo el año)

**La VM ya no se apaga** (decidido el 2026-10-05): ni producción ni pruebas tienen horario adjunto.
La política `pos-vps-horario` sigue existiendo pero no está asignada a ninguna VM. Lo que corre cada
noche:

| Hora | Qué | Dónde se configura |
|---|---|---|
| 00:45 | `pg_dump` de la base, verificado y subido fuera de la VM | cron de la VM, `/etc/cron.d/respaldo-gatobobah` (06:45 UTC: la VM corre en UTC) |
| 02:00 | Snapshot del disco completo, con la VM prendida | política `pos-vps-diario` (snapshot schedule, 08:00 UTC) sobre el disco `pos-vps` |

Con la VM prendida, el snapshot es del disco en uso: sirve para reconstruir la máquina, pero la base
se recupera del dump, que es consistente por construcción.

## 2. Dos copias, y por qué dos

- **El dump lógico** ([scripts/respaldo-produccion.sh](../scripts/respaldo-produccion.sh), instalado
  en `/usr/local/bin/respaldo-gatobobah.sh`). Formato custom, comprobado con `pg_restore -l` antes
  de guardarlo. Queda en `/var/backups/gatobobah` (las últimas 14) **y** en
  `gs://el-gato-bobah-pos-respaldos` (90 días por ciclo de vida, más 7 de *soft delete*). Es lo que
  se restaura para recuperar datos o para traerlos a local.
- **El snapshot del disco** (14 días). Es lo que se usa si la VM entera se pierde: trae el `.env`,
  la configuración de Caddy y los volúmenes de Docker, no solo la base.

**La VM no puede borrar sus respaldos.** Su cuenta de servicio (`pos-api-prod`) solo tiene
`storage.objectCreator` sobre el bucket: crea, no lee, no borra, no sobrescribe. Quien tomara el
servidor no alcanza las copias. Por eso el script sube por la API JSON con `ifGenerationMatch=0`
y no con `gcloud storage cp`, que antes de subir pide leer el objeto destino y falla con 403.

## 3. Cómo saber si corrió

```bash
# en la VM: la última línea es OK o FALLO, con tamaño y sha256
sudo journalctl -t respaldo-gatobobah -n 5
# desde fuera
gcloud storage ls -l gs://el-gato-bobah-pos-respaldos/ | tail -3
gcloud compute snapshots list --filter="sourceDisk~pos-vps$" --sort-by=~creationTimestamp --limit 3
```

**Nada avisa hoy si falla**: el script termina en error y lo deja en el journal, pero nadie lo lee.
Pendiente: una alerta (Cloud Monitoring sobre la antigüedad del último objeto del bucket, o un
correo).

## 4. Restaurar

- **En local**: bajar el `.dump` a `backups/prod/` y `make db-restaurar dump=backups/prod/<archivo>`.
  Restaura con dueños y GRANT (ver `AGENTS.md` §2); nunca con `--no-owner` ni `--no-privileges`.
- **Producción, datos**: `pg_restore` del dump sobre una base nueva, con la API detenida. Tómese
  antes un dump del estado actual, aunque esté dañado.
- **Producción, VM perdida**: crear un disco desde el último snapshot y una VM nueva con él, con la
  cuenta de servicio `pos-api-prod` (sin ella no descifra las credenciales de plataforma ni sube
  respaldos) y la IP fija.

Verificado el 2026-09-29: el primer dump del bucket se bajó, coincidió su sha256 y restauró sin
errores en una base temporal.

## 5. Si se vuelve a apagar de noche

Se apagó de 01:00 a 08:00 entre el 2026-09-29 y el 2026-10-05 para ahorrar mientras el sistema sirve
a un solo negocio; se quitó porque encontrar la VM apagada a media sesión costaba más que el ahorro.
Si se vuelve a poner: asignar `pos-vps-horario` a la VM y mover el cron del respaldo para que corra
15 minutos antes del apagado. Mientras esté apagada no hay API, un deploy de CI falla y los pedidos
de Uber por webhook no pueden llegar.
