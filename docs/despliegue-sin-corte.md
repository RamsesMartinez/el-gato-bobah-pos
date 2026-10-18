# Despliegue sin corte (azul/verde)

Desde la spec 041 la API de producción y la del ambiente de pruebas tienen **dos lados**,
`api-blue` y `api-green`, detrás de Caddy. Solo uno recibe tráfico. Lo corre CI con
`make deploy-image`, que llama a [scripts/deploy-azul-verde.sh](../scripts/deploy-azul-verde.sh).

## Qué hace una subida

1. Arranca la versión nueva en el lado que no atiende, sin tocar el que atiende.
2. Espera a que responda `/readyz` (incluye la base), hasta 60 s.
3. Escribe el lado nuevo en `deploy/upstream/activo.caddy` y recarga Caddy. El reload no corta
   conexiones.
4. Vuelve a revisar el lado nuevo; si cayó, regresa el tráfico al viejo.
5. Apaga el viejo con 30 s para terminar lo que traía (la API se da 25).

Si la nueva no queda lista, el tráfico nunca se movió: el deploy termina en rojo con el motivo y
la versión anterior sigue atendiendo.

## Qué lado atiende

```bash
cat ~/el-gato-bobah-pos/deploy/upstream/activo.caddy      # to api-blue:8080
docker ps --filter name=api- --format '{{.Names}} {{.Status}}'
```

## Regresar a la versión anterior

Volver a desplegar el commit anterior: el job de CI con ese SHA, o en la VM
`API_IMAGE=ghcr.io/ramsesmartinez/el-gato-bobah-pos/api:sha-<anterior> make deploy-image`. Es una
subida normal, sin corte. Si la versión anterior no tolera el esquema que dejó la nueva, el
problema es de la migración (constitución IV, *Toda migración tolera a la versión anterior*).

## Un despliegue que se quedó a la mitad

No hay que arreglar nada a mano: el siguiente `make deploy-image` parte del lado que dice
`activo.caddy` y recrea el otro. El archivo solo se escribe cuando la nueva ya respondió lista.

## Cambiar el Caddyfile

`git checkout` reemplaza el archivo y el contenedor de Caddy se queda viendo el anterior: un
reload no basta. Hay que recrearlo, que corta ~1.5 s:

```bash
docker compose -f deploy/docker-compose.yml up -d --no-deps --force-recreate --timeout 1 caddy
```

La primera subida con dos lados lo hace sola, por la misma razón: el Caddy de antes no tenía
montado `deploy/upstream/`. Es el único corte del esquema, y ocurre una sola vez por máquina.

## Bitácora

```bash
sudo journalctl SYSLOG_IDENTIFIER=api --since "…" -o cat     # en hora UTC
```

## Medir el corte

Desde otra terminal, mientras CI despliega:

```bash
scripts/medir-corte.sh https://api-dev.elgatobobah.com/healthz 180
```

Reporta peticiones, fallidas y latencia máxima, y sale distinto de 0 si hubo una sola fallida.
En producción no se corre: allá solo se revisa la salud después de subir.
