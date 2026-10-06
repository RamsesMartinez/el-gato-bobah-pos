---
name: "ambiente-dev"
description: "Prende, apaga o revisa la VM de pruebas (pos-vps-dev) en Google Cloud. Úsalo para no dejarla corriendo y pagando cuando no se ocupa, y para saber en qué estado quedó."
argument-hint: "on | off | estado (sin argumento = estado)"
user-invocable: true
disable-model-invocation: false
---

Prende o apaga el ambiente de pruebas. La VM era **spot** y se pasó a **estándar** en
septiembre de 2026, cuando las pruebas se volvieron diarias: una spot se detenía sola a media
corrida. Ya no la reclama Google; apagarla sigue siendo lo que ahorra.

Lo que cuesta, para que la decisión de apagarla tenga sentido (estimado, precios de lista):

| | Prendida | Apagada |
|---|---|---|
| VM estándar e2-micro | ~6 USD/mes | 0 |
| IP pública (efímera desde 2026-10-01) | ~3.6 USD/mes | 0 |
| Disco de 20 GB | ~1 USD/mes | ~1 USD/mes |

Ya **no tiene IP fija**: una IP reservada sin usar se cobra al doble (~0.01 USD/h), y esta VM pasa
casi todo el tiempo apagada. Cada arranque puede traer otra IP, y por eso `on` actualiza el DNS de
`api-dev` con [scripts/dns-ambiente-dev.sh](../../../scripts/dns-ambiente-dev.sh).

## Datos fijos

| | |
|---|---|
| Instancia | `pos-vps-dev`, zona `us-central1-a`, proyecto `el-gato-bobah-pos` |
| IP | efímera; la que tenga se ve con `describe` y la pone el script en `api-dev` |
| API de pruebas | `api-dev.elgatobobah.com` (la sirve esta VM) |
| Front de pruebas | `app-dev.elgatobobah.com` (proyecto de Pages `el-gato-bobah-pos-dev`) |
| Cuenta de servicio | `pos-api-dev`: solo cifrar/descifrar con la llave KMS `pos-dev/credenciales`, scope `cloud-platform` |
| Producción (NO tocar) | `pos-vps`, IP `34.68.178.107` |

> El ambiente de pruebas tiene su **propio proyecto de Pages** (`el-gato-bobah-pos-dev`), no una
> preview del de producción: en Pages un dominio propio siempre sirve el deploy de *producción* del
> proyecto, así que una preview nunca podría responder en `app-dev`. La rama de producción de ese
> proyecto se llama `staging` y es lo que CI publica en cada push a `develop`.
>
> Tampoco se sirve desde el Caddy de esta VM, que sería lo fácil: producción entrega el front desde
> Pages con el CSP de `web/public/_headers`, y con otro stack el ambiente de pruebas dejaría de
> probar lo que producción hace.

## Qué hacer según el argumento

### `on` — prenderla

```bash
gcloud compute instances start pos-vps-dev --zone us-central1-a
```

Después apunta el DNS a la IP de este arranque (ruta relativa a la raíz del repo):

```bash
scripts/dns-ambiente-dev.sh
```

Imprime `OK … ya apunta a` o `OK …: vieja -> nueva`. Si dice `FALLO`, repórtalo tal cual: lo más
probable es que el token de Cloudflare (`CLOUDFLARE_API_TOKEN` en `~/.claude/settings.json`,
permiso Zone·DNS·Edit) haya vencido o falte. El registro tiene TTL de 60 s.

Luego espera a que responda y repórtalo. El arranque completo tarda ~40s; los contenedores
suben solos por `restart: unless-stopped`:

```bash
gcloud compute instances describe pos-vps-dev --zone us-central1-a --format="value(status)"
curl -s -o /dev/null -w '%{http_code}\n' https://api-dev.elgatobobah.com/readyz
```

Si `/readyz` no contesta 200 al minuto, entra y mira los contenedores antes de reportar que está
lista.

### `off` — apagarla

```bash
gcloud compute instances stop pos-vps-dev --zone us-central1-a
```

Confirma que quedó en `TERMINATED`. Los datos del disco se conservan: la base de pruebas sigue ahí
al volver a prender.

### `estado` (o sin argumento)

```bash
gcloud compute instances describe pos-vps-dev --zone us-central1-a \
  --format="value(status,scheduling.provisioningModel,lastStartTimestamp)"
```

Si está `RUNNING`, agrega el estado de los contenedores y de la API:

```bash
gcloud compute ssh pos-vps-dev --zone us-central1-a --quiet --command="sudo docker ps --format '{{.Names}}|{{.Status}}'"
curl -s -o /dev/null -w '%{http_code}\n' https://api-dev.elgatobobah.com/readyz
```

## Reglas

- **Nunca toques `pos-vps`.** Es el que corre el negocio. Si un comando lleva el nombre de la
  instancia, verifica que diga `pos-vps-dev` antes de correrlo.
- **Reporta el estado real, no el comando que corriste.** "Prendida y `/readyz` en 200" o
  "prendida pero la API no responde todavía"; nunca "ya la prendí" a secas.
- Si `gcloud` no está autenticado (`gcloud auth list` sin cuentas), dilo y para: el login abre un
  navegador y lo tiene que hacer una persona.
- Si el estado es `TERMINATED` y nadie corrió `off`, ya **no** es Google reclamándola (la VM dejó
  de ser spot): alguien la apagó. Préndela, pero dilo en el reporte.
