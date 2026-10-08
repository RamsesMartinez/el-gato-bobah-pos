import { useMemo, useState } from 'react';
import { Box, Button, HStack, Heading, Input, Spinner, Text, VStack, Center } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { posApi } from '../../api/pos';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerCloseTrigger,
} from '../../components/ui/drawer';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import type { AccountGroup, AccountItem } from '../../types/pos';
import { money } from '../../utils/format';
import { ESTADO, antiguedad, nombreDeCuenta } from './estadosDeCuenta';

// TODAS LAS CUENTAS VIVAS, agrupadas (spec 030, US1; lienzo V2-2).
//
// Se abre desde «+N» de la fila. Al abrirse pide también las deudas de días anteriores: la fila,
// que se refresca cada 30 s, mira 90 días; esta hoja es una lectura por toque y puede mirar todo
// (research R-7). Así un fiado de la semana pasada no desaparece de la vista.

const GRUPOS: Array<{ g: AccountGroup; titulo: string }> = [
  { g: 'capturing', titulo: 'Capturando · aún no en cocina' },
  { g: 'in_kitchen', titulo: 'En cocina' },
  { g: 'delivered_owes', titulo: 'Entregadas que deben' },
  { g: 'previous_days', titulo: 'De días anteriores' },
];

// Con el teclado abierto la lista se queda en ~150 px: el buscador solo cuando hay de dónde buscar.
const CON_BUSCADOR = 8;

function aLaDerecha(c: AccountItem): string {
  if (c.state === 'paid_in_kitchen') return 'pagada';
  if (c.state === 'partly_paid' || c.state === 'delivered_owes') return money(Number(c.outstanding));
  return money(Number(c.total));
}

interface Props {
  isOpen: boolean;
  seleccionada: string | null;
  onElegir: (c: AccountItem) => void;
  onClose: () => void;
}

export function TodasLasCuentasSheet({ isOpen, seleccionada, onElegir, onClose }: Props) {
  const hora = useHoraDelNegocio();
  const [buscar, setBuscar] = useState('');
  const { data, isPending, dataUpdatedAt } = useQuery({
    queryKey: ['pos', 'accounts', 'todas'],
    queryFn: () => posApi.liveAccounts(true),
    enabled: isOpen,
    refetchOnMount: 'always',
  });
  const items = useMemo(() => data?.items ?? [], [data]);
  // La hora del SERVIDOR: el reloj de una tableta puede estar corrido.
  const ahora = data?.serverTime ? Date.parse(data.serverTime) : dataUpdatedAt;

  const filtradas = useMemo(() => {
    const q = buscar.trim().toLowerCase();
    if (!q) return items;
    return items.filter((c) => nombreDeCuenta(c).toLowerCase().includes(q)
      || (c.customerName ?? '').toLowerCase().includes(q)
      || (c.number !== null && String(c.number) === q.replace('#', '')));
  }, [items, buscar]);

  const salir = () => { setBuscar(''); onClose(); };

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) salir(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="85dvh" display="flex" flexDirection="column">
        <DrawerCloseTrigger />
        <DrawerHeader pb={2} pr={12}>
          <VStack align="stretch" gap={1}>
            <Text fontSize="lg" fontWeight="700">Cuentas</Text>
            {data && (
              <Text fontSize="sm" color="fg.muted">
                {items.length} vivas · por cobrar {money(Number(data.outstanding))}
              </Text>
            )}
            {items.length > CON_BUSCADOR && (
              <Input minH="44px" aria-label="Buscar cuenta" placeholder="Buscar cuenta"
                value={buscar} onChange={(e) => setBuscar(e.target.value)} />
            )}
          </VStack>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={4}>
          {isPending && <Center py={6}><Spinner /></Center>}
          {!isPending && items.length === 0 && (
            <Text color="fg.muted" textAlign="center" py={6}>No hay cuentas abiertas.</Text>
          )}
          <VStack align="stretch" gap={3}>
            {GRUPOS.map(({ g, titulo }) => {
              const del = filtradas.filter((c) => c.group === g);
              if (del.length === 0) return null;
              return (
                <Box key={g}>
                  <Heading as="h3" fontSize="xs" textTransform="uppercase" color="fg.muted" mb={1}>
                    {titulo} ({del.length})
                  </Heading>
                  <VStack align="stretch" gap={1}>
                    {del.map((c) => {
                      const e = ESTADO[c.state];
                      const nombre = nombreDeCuenta(c);
                      const cuando = g === 'previous_days'
                        ? hora.soloFecha(c.openedAt)
                        : `${hora.soloHora(c.openedAt)} · ${antiguedad(c.openedAt, ahora)}`;
                      const sub = [c.number !== null ? `#${c.number}` : null, cuando,
                        c.kind === 'draft' ? `${c.lineCount} productos` : null].filter(Boolean).join(' · ');
                      return (
                        <Button key={c.key} aria-label={`${nombre} · ${e.texto}`} minH="56px" h="auto" py={2} px={3}
                          variant={c.key === seleccionada ? 'subtle' : 'outline'} colorPalette="gray"
                          justifyContent="space-between" onClick={() => { onElegir(c); salir(); }}>
                          <VStack align="start" gap={0} minW={0}>
                            <HStack gap={2} minW={0}>
                              <Text fontWeight="700" truncate>{nombre}</Text>
                              <Text fontSize="xs" fontWeight="700" color={`${e.color}.fg`} flexShrink={0}>{e.texto}</Text>
                            </HStack>
                            <Text fontSize="xs" color="fg.muted" truncate>{sub}</Text>
                          </VStack>
                          <Text fontWeight="700" flexShrink={0}>{aLaDerecha(c)}</Text>
                        </Button>
                      );
                    })}
                  </VStack>
                </Box>
              );
            })}
          </VStack>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
