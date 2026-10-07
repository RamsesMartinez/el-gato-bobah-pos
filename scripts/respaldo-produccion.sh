#!/usr/bin/env bash
# Respaldo diario de la base de producción. Corre EN LA VM (pos-vps), por cron, a las 00:45 hora del
# centro de México, cuando el local ya cerró. (Se eligió esa hora cuando la VM se apagaba a la 01:00;
# desde el 2026-10-05 ya no se apaga, y la hora se quedó porque sigue siendo la de menos uso.)
#
#   1. pg_dump en formato custom dentro del contenedor de Postgres.
#   2. Comprueba que el archivo se puede leer (pg_restore -l). Un respaldo que no se verificó es un
#      respaldo que no se sabe si sirve.
#   3. Guarda una copia en la VM (/var/backups/gatobobah, las últimas 14).
#   4. Sube otra al bucket, FUERA de la VM: una copia en el mismo disco no sobrevive a que el disco
#      falle. La cuenta de servicio de la VM solo puede CREAR objetos ahí (objectCreator): no puede
#      leerlos ni borrarlos, así que quien tomara el servidor no puede destruir los respaldos.
#
# Falla ruidoso: cualquier paso que falle termina con código distinto de cero y queda en el journal
# (journalctl -t respaldo-gatobobah). Instalación y restauración: docs/respaldos-produccion.md.
set -euo pipefail

CONTENEDOR="${PG_CONTAINER:-deploy-postgres-1}"
BASE="${POSTGRES_DB:-gatobobah}"
DUENO="${POSTGRES_USER:-gatobobah}"
DIR="${RESPALDO_DIR:-/var/backups/gatobobah}"
BUCKET="${RESPALDO_BUCKET:-gs://el-gato-bobah-pos-respaldos}"
CONSERVAR="${RESPALDO_CONSERVAR:-14}"

log() { logger -t respaldo-gatobobah "$*"; echo "$*"; }

# La hora del nombre es la del negocio, no la de la VM (que corre en UTC).
SELLO="$(TZ=America/Mexico_City date +%Y%m%d-%H%M)"
NOMBRE="gatobobah-${SELLO}.dump"
mkdir -p "$DIR"
chmod 700 "$DIR"

docker exec "$CONTENEDOR" pg_dump -U "$DUENO" -d "$BASE" --format=custom --compress=6 -f "/tmp/$NOMBRE"
docker exec "$CONTENEDOR" pg_restore -l "/tmp/$NOMBRE" > /dev/null
docker cp "$CONTENEDOR:/tmp/$NOMBRE" "$DIR/$NOMBRE"
docker exec "$CONTENEDOR" rm -f "/tmp/$NOMBRE"
chmod 600 "$DIR/$NOMBRE"

SUMA="$(sha256sum "$DIR/$NOMBRE" | cut -d' ' -f1)"
TAMANO="$(stat -c %s "$DIR/$NOMBRE")"
if [ "$TAMANO" -lt 10000 ]; then
  log "FALLO: $NOMBRE mide $TAMANO bytes; un respaldo de la base real no puede ser tan chico"
  exit 1
fi

# Subida por la API JSON y no con `gcloud storage cp`: gcloud consulta antes el objeto destino
# (storage.objects.get), y esta cuenta a propósito no puede leer. ifGenerationMatch=0 solo crea:
# si el nombre ya existe, Storage responde 412 y no se pisa un respaldo anterior.
TOKEN="$(curl -sf -H 'Metadata-Flavor: Google' \
  'http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token' |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')"
CODIGO="$(curl -s -o /dev/null -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/octet-stream' \
  --data-binary @"$DIR/$NOMBRE" \
  "https://storage.googleapis.com/upload/storage/v1/b/${BUCKET#gs://}/o?uploadType=media&ifGenerationMatch=0&name=$NOMBRE")"
if [ "$CODIGO" != "200" ]; then
  log "FALLO: el bucket respondió $CODIGO al subir $NOMBRE; la copia local sí quedó en $DIR"
  exit 1
fi
log "OK $NOMBRE $TAMANO bytes sha256=$SUMA subido a $BUCKET"

# Poda local: solo las últimas $CONSERVAR. En el bucket la poda la hace su ciclo de vida.
ls -1t "$DIR"/gatobobah-*.dump | tail -n +"$((CONSERVAR + 1))" | xargs -r rm -f
