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
REQUIRED=(POSTGRES_PASSWORD JWT_SECRET ADMIN_PASSWORD)

gen_secret() {
  openssl rand -hex 32 2>/dev/null || (head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
}

is_placeholder() {
  # vacío, o los valores de ejemplo (cambia-esto…, your_…_here)
  case "$1" in
    "" | cambia-esto* | your_*_here) return 0 ;;
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
  printf "${YEL}Se creó deploy/.env desde el ejemplo (con un JWT_SECRET generado).${NC}\n"
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

# JWT_SECRET además debe ser razonablemente largo
jwt="$(get_val JWT_SECRET)"
if [ -n "$jwt" ] && [ "${#jwt}" -lt 16 ] && ! is_placeholder "$jwt"; then
  printf "${YEL}!${NC} JWT_SECRET es muy corto; usa uno de 32+ caracteres (ej. openssl rand -hex 32)\n"
fi

if [ ${#missing[@]} -ne 0 ]; then
  printf "${RED}Variables de entorno sin configurar en deploy/.env:${NC}\n"
  for key in "${missing[@]}"; do printf "  ${RED}✗${NC} %s\n" "$key"; done
  printf "${YEL}→ Edita deploy/.env con valores reales y vuelve a intentar.${NC}\n"
  exit 1
fi

printf "${GREEN}✓${NC} deploy/.env configurado.\n"
