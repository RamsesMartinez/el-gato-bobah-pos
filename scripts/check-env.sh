#!/usr/bin/env bash
# Valida que deploy/.env exista y tenga las variables requeridas configuradas
# (no vacías ni con el valor de ejemplo). Si falta el archivo, lo crea desde el
# ejemplo con un JWT_SECRET aleatorio y falla pidiendo completar los secretos.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/deploy/.env"
EXAMPLE="$ROOT/deploy/.env.example"
GREEN='\033[0;32m'; RED='\033[0;31m'; YEL='\033[0;33m'; NC='\033[0m'

# Variables obligatorias para producción
# PLATFORM_JWT_SECRET entra aquí porque la API NO ARRANCA sin él (consola de plataforma, spec
# 016). Atraparlo aquí evita el arranque que falla con un mensaje que nadie relaciona con el .env.
REQUIRED=(POSTGRES_PASSWORD JWT_SECRET PLATFORM_JWT_SECRET ADMIN_PASSWORD ADMIN_PIN)

# PLATFORM_DB_PASSWORD no entra en REQUIRED porque en DESARROLLO se sirve como dueño y no hace
# falta; en producción sí, y ahí lo exige config.Validate al arrancar. Fallaba feo sin ese check:
# el compose la interpola dentro de PLATFORM_DATABASE_URL, así que sin ella la URL queda válida a
# la vista y la API muere al conectar con un error que no nombra la variable.

gen_secret() {
  openssl rand -hex 32 2>/dev/null || (head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
}

is_placeholder() {
  # vacío, o los valores de ejemplo (cambia-esto…, your_…_here), o PINs triviales
  case "$1" in
    "" | cambia-esto* | your_*_here) return 0 ;;
    1234 | 0000 | 1111 | 2222 | 3333 | 4444 | 5555 | 6666 | 7777 | 8888 | 9999 | 4321 | 2345 | 3456 | 4567 | 5678 | 6789) return 0 ;;
    *) return 1 ;;
  esac
}

get_val() {
  grep -E "^$1=" "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-
}

if [ ! -f "$ENV_FILE" ]; then
  cp "$EXAMPLE" "$ENV_FILE"
  secret="$(gen_secret)"
  sed -i.bak "s|^JWT_SECRET=.*|JWT_SECRET=$secret|" "$ENV_FILE" && rm -f "$ENV_FILE.bak"
  # Dos secretos distintos, generados por separado: la consola de plataforma y el negocio no
  # pueden compartir firma, y copiar el mismo valor a los dos es justo lo que la API rechaza.
  sed -i.bak "s|^PLATFORM_JWT_SECRET=.*|PLATFORM_JWT_SECRET=$(gen_secret)|" "$ENV_FILE" && rm -f "$ENV_FILE.bak"
  printf "${YEL}Se creó deploy/.env desde el ejemplo (con JWT_SECRET y PLATFORM_JWT_SECRET generados).${NC}\n"
  printf "${RED}Falta configurarlo antes de continuar.${NC} Edita deploy/.env y define:\n"
  printf "  - POSTGRES_PASSWORD  (contraseña de la base de datos)\n"
  printf "  - ADMIN_PASSWORD     (contraseña del usuario admin inicial)\n"
  printf "Luego vuelve a correr el comando.\n"
  exit 1
fi

missing=()
for key in "${REQUIRED[@]}"; do
  val="$(get_val "$key")"
  if is_placeholder "$val"; then
    missing+=("$key")
  fi
done

# JWT_SECRET además debe ser largo (la API exige 32+ y se niega a arrancar si no)
jwt="$(get_val JWT_SECRET)"
if [ -n "$jwt" ] && [ "${#jwt}" -lt 32 ] && ! is_placeholder "$jwt"; then
  printf "${RED}✗${NC} JWT_SECRET es muy corto (<32); genera uno con: openssl rand -base64 48\n"
  missing+=("JWT_SECRET")
fi

platform_jwt="$(get_val PLATFORM_JWT_SECRET)"
if [ -n "$platform_jwt" ] && [ "${#platform_jwt}" -lt 32 ] && ! is_placeholder "$platform_jwt"; then
  printf "${RED}✗${NC} PLATFORM_JWT_SECRET es muy corto (<32); genera uno con: openssl rand -base64 48\n"
  missing+=("PLATFORM_JWT_SECRET")
fi
# Iguales = un solo secreto: un token de la consola valdría en el negocio y al revés. La API se
# niega a arrancar así; se dice aquí para no descubrirlo en el arranque.
if [ -n "$jwt" ] && [ "$jwt" = "$platform_jwt" ]; then
  printf "${RED}✗${NC} PLATFORM_JWT_SECRET es IGUAL a JWT_SECRET: la consola y el negocio dejarían de estar separados\n"
  missing+=("PLATFORM_JWT_SECRET")
fi

if [ ${#missing[@]} -ne 0 ]; then
  printf "${RED}Variables de entorno sin configurar en deploy/.env:${NC}\n"
  for key in "${missing[@]}"; do printf "  ${RED}✗${NC} %s\n" "$key"; done
  printf "${YEL}→ Edita deploy/.env con valores reales y vuelve a intentar.${NC}\n"
  exit 1
fi

printf "${GREEN}✓${NC} deploy/.env configurado.\n"
