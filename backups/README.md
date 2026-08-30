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

## Cómo restaurar en local para ensayar

Nunca se ensaya contra producción. Se restaura en una base de trabajo aparte:

```bash
MSYS_NO_PATHCONV=1 docker cp backups/prod/pre-$S.dump deploy-postgres-1:/tmp/prod.dump
docker exec deploy-postgres-1 psql -U gatobobah -d postgres -c "create database gatobobah_ensayo"
docker exec deploy-postgres-1 pg_restore -U gatobobah -d gatobobah_ensayo --no-owner --no-privileges /tmp/prod.dump
```

Ojo con lo que **no** se ve en un ensayo local: la base local tiene una sola empresa y la API sirve
como owner (sin RLS ni grants). Todo lo que dependa de un segundo tenant o del rol `gatobobah_app`
hay que probarlo con un test de integración, no a ojo.
