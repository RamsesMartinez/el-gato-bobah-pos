# Servidor de RustDesk

Runbook del servidor propio de RustDesk (acceso remoto a los equipos del local). Movido de la VM de
pruebas a producción el 2026-10-02.

## Dónde vive

| | |
|---|---|
| VM | `pos-vps` (producción), fuera de Docker: paquetes `rustdesk-server-hbbs` y `rustdesk-server-hbbr` 1.1.16, servicios systemd del mismo nombre |
| Nombre | `rustdesk.elgatobobah.com` → IP fija de producción, registro DNS-only (sin proxy de Cloudflare: RustDesk no es HTTP) |
| Puertos | 21115-21119 TCP y 21116 UDP; los abre la regla de firewall `allow-rustdesk` sobre la etiqueta `rustdesk-server` |
| Llave y datos | `/var/lib/rustdesk-server/` (`id_ed25519`, `db_v2.sqlite3`); logs en `/var/log/rustdesk-server/` |

**Por qué en producción y no en pruebas:** la VM de pruebas pasa casi todo el tiempo apagada y desde
2026-10-01 tiene IP efímera, así que el servidor se caía con ella y los equipos apuntaban a una IP
que ya no existe. Producción está prendida y con IP fija.

**Por qué fuera del `docker-compose` del POS:** el deploy de CI hace `compose up` sobre ese
archivo; RustDesk no tiene nada que ver con el POS y no debe reiniciarse ni romperse con un deploy
de la API.

**Se conservó la llave del servidor anterior**, a propósito: los equipos ya capturados solo cambian
la dirección del servidor, no la llave. Una llave nueva obligaría a recapturarla en cada equipo.

## Lo que cuesta

- Producción ya no se apaga de noche (horario quitado el 2026-10-05), así que responde 24 h.
- Memoria: hbbs + hbbr usan unos MB en una VM de 1 GB.

## Configurar un equipo

En RustDesk → Ajustes → Red → Servidor ID/Relay:

1. **Servidor ID**: `rustdesk.elgatobobah.com`
2. **Servidor relay**: `rustdesk.elgatobobah.com`
3. **Key**: la pública del servidor, que se lee con
   `sudo cat /var/lib/rustdesk-server/id_ed25519.pub` en `pos-vps`. No se copia aquí: el repo es
   público.

## Revisar que corre

```bash
# en pos-vps
systemctl is-active rustdesk-hbbs rustdesk-hbbr
sudo tail -20 /var/log/rustdesk-server/hbbs.log
# desde fuera
timeout 5 bash -c '</dev/tcp/rustdesk.elgatobobah.com/21116' && echo abierto
```

## Respaldo

La llave entra en el snapshot diario del disco de producción; no en el dump de la base. Si la VM se
pierde y se reconstruye sin snapshot, se genera una llave nueva y hay que recapturarla en cada
equipo.

En la VM de pruebas quedaron los paquetes instalados pero **deshabilitados** (`systemctl disable`),
con la misma llave. No se vuelven a prender: dos servidores con la misma llave reparten los equipos
entre los dos.
