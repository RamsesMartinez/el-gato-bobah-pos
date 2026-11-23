import { useState } from 'react';
import { Box, Button, HStack, Input, Text } from '@chakra-ui/react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi, type PublicMenuSettings } from '../../../../api/admin';
import { ApiError } from '../../../../api/client';
import { slugDelMenu } from './slug';
import { toaster } from '../../../../components/ui/toaster';

const base = (m: PublicMenuSettings) => m.url.replace(/^https?:\/\//, '').slice(0, -m.name.length);

function textoDeError(e: unknown, m: PublicMenuSettings): string {
  if (e instanceof ApiError && e.code === 'MENU_NAME_UNAVAILABLE') return 'Ese nombre no está disponible. Prueba con otro.';
  if (e instanceof ApiError && e.code === 'MENU_NAME_LIMIT') {
    return `Ya cambiaste el nombre muchas veces. Puedes volver a uno anterior: ${m.previousNames.slice(0, 5).join(', ')}.`;
  }
  if (e instanceof ApiError && e.code === 'VALIDATION') return 'Usa de 3 a 40 letras o números.';
  return 'No se pudo guardar el nombre. Intenta de nuevo.';
}

// NombreDelMenu cambia la parte final de la dirección. Lo que se escribe se ve al momento como
// quedará; la dirección de arriba solo cambia al guardar.
export function NombreDelMenu({ menu }: { menu: PublicMenuSettings }) {
  const qc = useQueryClient();
  const [texto, setTexto] = useState(menu.name);
  const slug = slugDelMenu(texto);
  const corto = slug.length < 3;
  // Mismo `scope` que el color: los dos cambian el mismo menú y sus respuestas no pueden llegar
  // cruzadas (la última en llegar dejaría en pantalla un estado viejo).
  const guardar = useMutation({
    scope: { id: 'public-menu' },
    mutationFn: (name: string) => adminApi.updatePublicMenu({ name }),
    onSuccess: (m) => {
      qc.setQueryData(['admin', 'public-menu'], m);
      toaster.create({ title: 'Nombre guardado', description: 'La dirección anterior y el código QR siguen llevando a tu menú.', type: 'success' });
    },
  });
  const error = guardar.isError ? textoDeError(guardar.error, menu) : corto ? 'Usa al menos 3 letras o números.' : '';

  return (
    <Box>
      <Text asChild fontWeight="600" display="block" mb={1}><label htmlFor="nombre-del-menu">Nombre en la dirección</label></Text>
      <HStack gap={2} align="start">
        <Box flex="1" minW={0}>
          <Input id="nombre-del-menu" minH="44px" value={texto} autoCapitalize="none" autoComplete="off" maxLength={60}
            aria-invalid={!!error} aria-describedby="nombre-del-menu-ayuda"
            onChange={(e) => { setTexto(e.target.value); if (!guardar.isPending) guardar.reset(); }} />
          <Text fontSize="sm" color="fg.muted" mt={1}>Quedará: {base(menu)}<b>{slug}</b></Text>
        </Box>
        <Button minH="44px" disabled={corto || slug === menu.name} loading={guardar.isPending} onClick={() => guardar.mutate(texto)}>Guardar</Button>
      </HStack>
      <Text id="nombre-del-menu-ayuda" aria-live="polite" fontSize="sm" color={error ? 'red.fg' : 'fg.muted'} mt={2} minH="20px">
        {error || 'Si lo cambias, la dirección anterior y el QR impreso siguen llevando a tu menú.'}
      </Text>
    </Box>
  );
}
