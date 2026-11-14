import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { Box, Button, HStack, Text } from '@chakra-ui/react';
import { useMutation } from '@tanstack/react-query';
import { LuCircleAlert, LuX } from 'react-icons/lu';
import { notificationsApi } from '../../api/notifications';
import { useAvisos } from './useAvisos';

// La franja de «se acabó» (spec 084, maqueta M4, opción B v2): una tarjeta roja de 560 px arriba al
// centro, en la misma capa que el aviso de pedido de plataforma (2000, sobre las hojas). Reglas:
//   - Una vez por aviso y persona: al mostrarse se guarda como vista en el servidor (no vuelve al
//     recargar). Quien lo reportó no la ve: ya vio su propio aviso verde.
//   - Espera mientras haya una hoja abierta (un cobro, un ticket): no le roba el toque a una captura.
//   - Se quita sola a los 90 s; el aviso sigue en la campana.
//   - Si hay un aviso de pedido de plataforma, va debajo de él: se mide su alto en vivo, porque crece
//     cuando su texto se parte en dos renglones.
export const CAPA_DE_LA_FRANJA = 2000;
export const DURA_MS = 90_000;
const SEPARACION = 8;

const hayHojaAbierta = () => document.querySelector('[role="dialog"]') !== null;
const altoDelPedido = () => {
  const el = document.querySelector('[data-testid="aviso-de-pedido-entrante"]');
  return el ? el.getBoundingClientRect().bottom : 0;
};

export function FranjaDeAgotado({ onVer }: { onVer: () => void }) {
  const { open } = useAvisos();
  const [actualID, setActualID] = useState<number | null>(null);
  const [vistas, setVistas] = useState<ReadonlySet<number>>(new Set());
  const [esperando, setEsperando] = useState(hayHojaAbierta);
  const [arriba, setArriba] = useState(altoDelPedido);
  const { mutate: marcarVista } = useMutation({ mutationFn: (id: number) => notificationsApi.bannerSeen(id) });

  // La hoja abierta y el alto del aviso de pedido cambian sin pasar por React: se observan.
  useEffect(() => {
    const mira = () => { setEsperando(hayHojaAbierta()); setArriba(altoDelPedido()); };
    const obs = new MutationObserver(mira);
    obs.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['role'] });
    window.addEventListener('resize', mira);
    return () => { obs.disconnect(); window.removeEventListener('resize', mira); };
  }, []);

  // El siguiente aviso con franja entra en cuanto no hay otra a la vista ni una hoja abierta. Se ajusta
  // al pintar (estado derivado de lo anterior), no en un efecto.
  const siguiente = open.find((n) => n.banner && !vistas.has(n.id)) ?? null;
  if (actualID === null && !esperando && siguiente) {
    setActualID(siguiente.id);
    setVistas(new Set(vistas).add(siguiente.id));
  }
  const actual = open.find((n) => n.id === actualID) ?? null;
  // Si el aviso se cerró mientras su franja estaba a la vista, deja el lugar para el siguiente.
  if (actualID !== null && !actual) setActualID(null);

  // Se guarda como vista en el servidor al mostrarse: no vuelve aunque se recargue.
  useEffect(() => {
    if (actualID !== null) marcarVista(actualID);
  }, [actualID, marcarVista]);
  useEffect(() => {
    if (actualID === null) return;
    const t = setTimeout(() => setActualID(null), DURA_MS);
    return () => clearTimeout(t);
  }, [actualID]);

  if (!actual || esperando) return null;
  return createPortal(
    <Box role="status" data-testid="franja-de-agotado" position="fixed" top={`${arriba + SEPARACION}px`} left="50%"
      transform="translateX(-50%)" w="560px" maxW="calc(100vw - 32px)" zIndex={CAPA_DE_LA_FRANJA}
      bg="red.solid" color="white" borderRadius="lg" boxShadow="lg" overflow="hidden">
      <HStack px={4} minH="56px" gap={3}>
        <LuCircleAlert size={24} />
        <Text flex="1" fontWeight="700" fontSize="lg" lineClamp={2}>Se acabó: {actual.itemName}</Text>
        <Button minH="44px" variant="outline" color="white" borderColor="whiteAlpha.700" _hover={{ bg: 'whiteAlpha.200' }}
          onClick={() => { setActualID(null); onVer(); }}>Ver</Button>
        <Button minH="44px" minW="44px" variant="ghost" color="white" aria-label="Quitar" _hover={{ bg: 'whiteAlpha.200' }}
          onClick={() => setActualID(null)}><LuX /></Button>
      </HStack>
      <Box h="4px" bg="whiteAlpha.400">
        <Box h="100%" bg="white" css={{ animation: `franja-084 ${DURA_MS}ms linear forwards`, '@keyframes franja-084': { from: { width: '100%' }, to: { width: '0%' } } }} />
      </Box>
    </Box>,
    document.body,
  );
}
