---
name: "ambiente-dev"
description: "Prende, apaga o revisa la VM de pruebas (pos-vps-dev) en Google Cloud. Úsalo para no dejarla corriendo y pagando cuando no se ocupa, y para saber en qué estado quedó."
argument-hint: "on | off | estado (sin argumento = estado)"
user-invocable: true
disable-model-invocation: false
---

Prende o apaga el ambiente de pruebas. La VM es **spot**, así que cuesta ~4 veces menos que una
normal a cambio de que Google pueda reclamarla con 30 segundos de aviso; cuando eso pasa se
**detiene** (no se destruye) y basta volver a prenderla.

Lo que cuesta, para que la decisión de apagarla tenga sentido (estimado):

| | Prendida | Apagada |
|---|---|---|
| VM spot e2-micro | ~3 USD/mes | 0 |
| IP fija reservada | ~3 USD/mes | ~3 USD/mes (se cobra igual) |
| Disco de 20 GB | ~1 USD/mes | ~1 USD/mes |

O sea: apagarla ahorra la VM, no la IP ni el disco. La IP se conserva a propósito — sin ella el
registro DNS de `api-dev` se rompería en cada arranque.

## Datos fijos

| | |
|---|---|
| Instancia | `pos-vps-dev`, zona `us-central1-a`, proyecto `el-gato-bobah-pos` |
| IP fija | `34.61.175.194` |
| Subdominios | `api-dev.elgatobobah.com` · `app-dev.elgatobobah.com` |
| Producción (NO tocar) | `pos-vps`, IP `34.68.178.107` |

## Qué hacer según el argumento

### `on` — prenderla

```bash
gcloud compute instances start pos-vps-dev --zone us-central1-a
```

Después espera a que responda y repórtalo. El arranque completo tarda ~40s; los contenedores
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
- Que la VM sea spot significa que **puede aparecer apagada sin que nadie la haya apagado**. Si el
  estado es `TERMINATED` y nadie corrió `off`, fue Google reclamándola: préndela y sigue.
