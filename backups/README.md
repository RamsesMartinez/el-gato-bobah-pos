# backups/

Respaldos de la base de **producción**, tomados antes de cada cambio riesgoso de datos o esquema.

`backups/prod/` está en `.gitignore`: los dumps traen datos reales del negocio (ventas, usuarios,
hashes de contraseña) y **no se versionan**. Solo se versiona el `.gitkeep` para que la carpeta
exista al clonar.

## Cómo tomar uno

```bash
S=$(date +%Y%m%d-%H%M)
ssh -i ~/.ssh/google_compute_engine ramys@34.68.178.107 \
  "sudo docker exec deploy-postgres-1 pg_dump -U gatobobah -d gatobobah --format=custom --compress=6 \
     -f /tmp/pre-$S.dump && sudo docker cp deploy-postgres-1:/tmp/pre-$S.dump /tmp/ && sudo chown ramys /tmp/pre-$S.dump"
scp -i ~/.ssh/google_compute_engine ramys@34.68.178.107:/tmp/pre-$S.dump backups/prod/
```

**Verifica el checksum en los dos lados antes de tocar nada.** Un respaldo que no se comparó es un
respaldo que no sabes si sirve:

```bash
sha256sum backups/prod/pre-$S.dump
ssh -i ~/.ssh/google_compute_engine ramys@34.68.178.107 "sha256sum /tmp/pre-$S.dump"
```

## Cómo restaurar en local

```bash
make db-restaurar                                   # el .dump más reciente de backups/prod/
make db-restaurar dump=backups/prod/pre-XXXX.dump   # uno en particular
```

Reemplaza la base de desarrollo entera (detén la API antes) y restaura **con los dueños y los
GRANT de producción**. Es a propósito: la API local sirve como `gatobobah_app`, sujeta a RLS igual
que allá, y un restore sin permisos fabrica un ambiente que no se parece al real.

**No uses `--no-owner` ni `--no-privileges`**, aunque "hagan que el restore no se queje":

- `--no-privileges` deja a `gatobobah_app` sin un solo GRANT y la API no arranca.
- `--no-owner` le quita la vista `candidatas_del_aviso` a `gatobobah_webhook` y el webhook resuelve
  con un bypass que en producción no existe.

Una sesión de `psql -U gatobobah` es owner y ve todas las empresas. Para ver lo que ve la API:
`set role gatobobah_app; set app.company_id = '2';` antes de la consulta.

El porqué completo está en la constitución (IV, *La base local niega lo mismo que producción*) y la
mecánica en `AGENTS.md` §2.
