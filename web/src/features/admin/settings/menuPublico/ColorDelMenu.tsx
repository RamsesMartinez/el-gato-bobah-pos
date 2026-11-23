import { Box, Button, HStack, Text } from '@chakra-ui/react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { LuCheck } from 'react-icons/lu';
import { adminApi, type PublicMenuSettings } from '../../../../api/admin';
import { PALETTES } from '../../../../stores/ui';
import { toaster } from '../../../../components/ui/toaster';

// ColorDelMenu: las mismas paletas que la pantalla Interfaz, para el menú que ven los clientes. Se
// guarda al tocar, con aviso, como un interruptor.
export function ColorDelMenu({ menu }: { menu: PublicMenuSettings }) {
  const qc = useQueryClient();
  const guardar = useMutation({
    scope: { id: 'public-menu' },
    mutationFn: (palette: string) => adminApi.updatePublicMenu({ palette }),
    onSuccess: (m) => {
      qc.setQueryData(['admin', 'public-menu'], m);
      toaster.create({ title: 'Color guardado', description: 'El menú en línea lo toma en un par de minutos.', type: 'success' });
    },
    onError: () => toaster.create({ title: 'No se pudo guardar el color', description: 'Intenta de nuevo.', type: 'error' }),
  });
  const elegida = guardar.isPending ? guardar.variables : menu.palette;
  const nombre = PALETTES.find((p) => p.id === elegida)?.label ?? '';
  const idx = Math.max(0, PALETTES.findIndex((p) => p.id === elegida));
  // Patrón de grupo de opciones: una sola parada de teclado y flechas para moverse.
  const teclas = (e: React.KeyboardEvent) => {
    const paso = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
    if (!paso) return;
    e.preventDefault();
    const sig = PALETTES[(idx + paso + PALETTES.length) % PALETTES.length];
    guardar.mutate(sig.id);
    (e.currentTarget.parentElement?.children[(idx + paso + PALETTES.length) % PALETTES.length] as HTMLElement | undefined)?.focus();
  };

  return (
    <Box>
      <HStack justify="space-between" mb={2}>
        <Text id="color-del-menu" fontWeight="600">Color del menú</Text>
        <Text fontSize="sm" color="fg.muted">{nombre}</Text>
      </HStack>
      <HStack gap={2} flexWrap="wrap" role="radiogroup" aria-labelledby="color-del-menu">
        {PALETTES.map((p) => {
          const on = p.id === elegida;
          return (
            <Button key={p.id} role="radio" aria-checked={on} aria-label={p.label} colorPalette={p.id}
              tabIndex={on ? 0 : -1} onKeyDown={teclas}
              minW="44px" h="44px" p={0} borderRadius="full" variant="solid"
              outline={on ? '3px solid' : undefined} outlineColor="fg" outlineOffset="2px"
              _focusVisible={{ outline: '3px solid', outlineColor: 'fg', outlineOffset: '2px' }}
              onClick={() => { if (!on) guardar.mutate(p.id); }}>
              {on && <LuCheck />}
            </Button>
          );
        })}
      </HStack>
    </Box>
  );
}
