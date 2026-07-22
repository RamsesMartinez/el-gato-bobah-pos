# Email: Mailpit (local) y Zoho Mail (producción)

El sistema envía correo **solo** para la recuperación de contraseña (flujo "olvidé mi
contraseña"). El transporte es SMTP estándar (`net/smtp`, sin dependencias), así que el mismo
código habla con Mailpit en local y con Zoho en producción — solo cambian las variables de
entorno.

Si `SMTP_HOST` está vacío, el email queda **deshabilitado**: la recuperación por correo no se
ofrece y el admin restablece contraseñas a mano desde el panel de Empleados. Todo lo demás
funciona igual.

## Variables de entorno (deploy/.env)

| Variable | Local (Mailpit) | Producción (Zoho) |
|---|---|---|
| `SMTP_HOST` | `localhost` (lo pone `dev-api.sh`) | `smtp.zoho.com` |
| `SMTP_PORT` | `1025` | `587` (STARTTLS) o `465` (SSL) |
| `SMTP_USER` | *(vacío)* | tu buzón Zoho, ej. `no-reply@tudominio.com` |
| `SMTP_PASS` | *(vacío)* | **App Password** de Zoho (no la contraseña normal) |
| `MAIL_FROM` | `no-reply@gatobobah.local` | igual que `SMTP_USER` |
| `APP_BASE_URL` | `http://localhost:3000` | `https://app.tudominio.com` (para el link del correo) |

> Zoho por región: si tu cuenta es de la UE usa `smtp.zoho.eu`; India `smtp.zoho.in`;
> Australia `smtp.zoho.com.au`. El puerto y el resto no cambian.

## Local (ya configurado)

`make start` levanta **Mailpit** en Docker (`deploy/docker-compose.dev.yml`). Abre la bandeja
de correos capturados en **http://localhost:8025** — ahí verás los emails de recuperación con su
enlace, sin enviar nada a internet.

## Producción — pasos para obtener las credenciales de Zoho

Zoho Mail **no** deja usar tu contraseña normal por SMTP si tienes 2FA (y deberías tenerlo). Se
usa una **App Password** (contraseña de aplicación) dedicada. Pasos:

1. **Ten un buzón dedicado al envío.** En el Admin Console de Zoho Mail
   (https://mailadmin.zoho.com) crea un usuario/buzón tipo `no-reply@tudominio.com`
   (o un alias con buzón). Ese será `SMTP_USER` y `MAIL_FROM`.
   - Requiere que tu dominio esté verificado en Zoho y con los registros **SPF** y **DKIM**
     publicados (Zoho te da los valores en *Domains → tu dominio → Email Configuration*). Sin
     SPF/DKIM, los correos caerán en spam o serán rechazados.

2. **Activa la verificación en dos pasos** en esa cuenta (https://accounts.zoho.com →
   *Security → Two-factor Authentication*). Es requisito para poder generar App Passwords.

3. **Genera la App Password:**
   - Entra a **https://accounts.zoho.com** con la cuenta `no-reply@…`.
   - *Security → App Passwords* (a veces "Application-Specific Passwords").
   - Clic en **Generate New Password**, ponle un nombre como `elgatobobah-pos-smtp`.
   - Copia la contraseña generada (16 caracteres). **Es `SMTP_PASS`.** No se vuelve a mostrar;
     si la pierdes, genera otra y revoca la anterior.

4. **Habilita el acceso SMTP** para la cuenta si tu plan lo pide:
   *Zoho Mail → Settings → Mail Accounts → IMAP/SMTP Access* → activar SMTP. (En planes de
   organización puede estar en el Admin Console.)

5. **Rellena `deploy/.env`** en el servidor:
   ```
   SMTP_HOST=smtp.zoho.com
   SMTP_PORT=587
   SMTP_USER=no-reply@tudominio.com
   SMTP_PASS=<la App Password de 16 caracteres>
   MAIL_FROM=no-reply@tudominio.com
   APP_BASE_URL=https://app.tudominio.com
   ```

6. **Prueba el envío.** Desde la app: *¿Olvidaste tu contraseña?* con un usuario que tenga
   `recovery_email` registrado. Debe llegar el correo con el enlace. Si no llega:
   - Revisa spam y que SPF/DKIM estén verdes en Zoho.
   - `530 Authentication required` / `535` → `SMTP_USER`/`SMTP_PASS` mal (usa la **App
     Password**, no la del login).
   - Timeout → el firewall del servidor bloquea el puerto 587/465 de salida; ábrelo.

## Nota: Zoho ZeptoMail (alternativa, NO usada)

Zoho tiene un producto aparte, **ZeptoMail**, específico para correo transaccional (mejor
entregabilidad/reputación) que se usa por API o SMTP con un token. Aquí **no** lo usamos:
elegimos Zoho **Mail** por SMTP porque ya tienes esas cuentas y es el mismo camino que Mailpit.
Si algún día el volumen o la entregabilidad lo justifican, ZeptoMail se conecta cambiando
`SMTP_HOST=smtp.zeptomail.com`, `SMTP_USER=emailapikey` y `SMTP_PASS=<token de envío>` — sin
tocar código.

## Límites de envío de Zoho Mail

Zoho Mail (buzón normal) tiene límites diarios de envío según el plan (típicamente cientos/día).
Para recuperación de contraseña de un POS de un local es de sobra. Si escalas a muchas empresas
con alto volumen, evalúa ZeptoMail.
