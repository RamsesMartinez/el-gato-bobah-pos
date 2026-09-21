#!/usr/bin/env bash
# Pide aprobación antes de que un agente toque la VM de PRODUCCIÓN.
#
# POR QUÉ EXISTE. El 2026-09-19 se revisó el historial de sesiones buscando si algún agente había
# escrito dentro de `pos-vps`. No había ninguna escritura —solo lecturas del `.env` y respaldos—,
# pero la respuesta salió de auditar doce transcripciones a mano: nada impedía que la siguiente vez
# sí ocurriera. Esto convierte la regla en un mecanismo.
#
# Lo que hace: mira el comando ANTES de correrlo y pide confirmación si nombra la VM de producción.
# Los respaldos pasan sin preguntar, que es la excepción que el dueño autorizó: un `pg_dump` no
# cambia nada y es justo lo que se quiere poder hacer rápido antes de una migración.
#
# El ambiente de PRUEBAS (`pos-vps-dev`) no se toca: ahí se prueba, y frenar cada comando volvería
# inútil la verificación contra el ambiente desplegado.
set -uo pipefail

comando=$(jq -r '.tool_input.command // ""' 2>/dev/null)
[ -z "$comando" ] && exit 0

# `pos-vps` seguido de algo que NO sea un guion: así `pos-vps-dev` queda fuera sin listarlo aparte.
if ! printf '%s' "$comando" | grep -qE 'pos-vps([^-]|$)|34\.68\.178\.107'; then
  exit 0
fi

# La excepción autorizada: respaldar. `pg_dump` y traerse el archivo que dejó.
if printf '%s' "$comando" | grep -qE 'pg_dump|compute scp +pos-vps:'; then
  printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"Respaldo de produccion: autorizado sin preguntar."}}'
  exit 0
fi

printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"Este comando toca la VM de PRODUCCION (pos-vps). Solo los respaldos pasan solos; todo lo demas necesita tu aprobacion explicita."}}'
