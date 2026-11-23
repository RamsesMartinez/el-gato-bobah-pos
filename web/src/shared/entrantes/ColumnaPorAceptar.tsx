import { useEffect, useState } from 'react';
import { Badge, Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { toaster } from '../../components/ui/toaster';
import { ApiError } from '../../api/client';
import { posApi } from '../../api/pos';
import {
  aceptarEntrante, rechazarEntrante, MOTIVOS_DE_RECHAZO, type MotivoDeRechazo, type PedidoEntrante,
} from '../../api/pedidosEntrantes';
import type { OrderView } from '../../types/pos';
import { KitchenTicket } from '../tickets/AutoPrintTicket';
import { useColumnaPorAceptar } from './estado';
import { TarjetaEntrante } from './TarjetaEntrante';
import { CLAVE_ENTRANTES, useEntrantes } from './useEntrantes';

const otraTabletaGano = (e: unknown) => e instanceof ApiError && e.status === 409 && e.code === 'ALREADY_DECIDED';

/**
 * ColumnaPorAceptar es la columna de Pedidos con lo que espera decisión (M-PorAceptar, opción B2):
 * a la izquierda del tablero, con la misma forma que «En preparación». El más urgente va abierto y
 * los demás plegados, para que en hora pico quepan los botones de más de uno.
 *
 * Mientras se pinta, la franja de arriba se quita en Pedidos: diría lo mismo dos veces.
 */
export function ColumnaPorAceptar() {
  const { pedidos } = useEntrantes();
  const qc = useQueryClient();
  const [decidiendo, setDecidiendo] = useState<number | null>(null);
  const [aceptandoId, setAceptandoId] = useState<number | null>(null);
  const [rechazando, setRechazando] = useState<PedidoEntrante | null>(null);
  const [motivo, setMotivo] = useState<MotivoDeRechazo | null>(null);
  // La comanda la imprime SOLO la tableta que aceptó; las demás se enteran por el aviso.
  const [comanda, setComanda] = useState<OrderView | null>(null);
  const [ahora, setAhora] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setAhora(Date.now()), 15_000);
    return () => clearInterval(t);
  }, []);

  const hay = pedidos.length > 0;
  useEffect(() => {
    useColumnaPorAceptar.setState({ visible: hay });
    return () => useColumnaPorAceptar.setState({ visible: false });
  }, [hay]);

  const despues = () => {
    void qc.invalidateQueries({ queryKey: CLAVE_ENTRANTES });
    void qc.invalidateQueries({ queryKey: ['orders'] });
  };

  const aceptar = async (p: PedidoEntrante) => {
    setDecidiendo(p.id);
    setAceptandoId(p.id);
    let creado;
    try {
      creado = await aceptarEntrante(p);
      toaster.create({ title: `Pedido #${creado.number} aceptado`, description: 'Ya está en cocina.', type: 'success' });
    } catch (e) {
      // Que otra tableta ganara es el funcionamiento normal con varias: se dice sin alarmar.
      toaster.create(otraTabletaGano(e)
        ? { title: 'Otra tableta ya lo atendió', type: 'info' }
        : { title: 'No se pudo aceptar', description: `${p.sourceName} no lo confirmó. Inténtalo otra vez o atiéndelo en su aplicación.`, type: 'error' });
    } finally {
      setDecidiendo(null);
      setAceptandoId(null);
      despues();
    }
    if (!creado) return;
    // La comanda sale del pedido que quedó en el POS: es la misma que se reimprime desde Pedidos.
    try {
      setComanda(await posApi.order(creado.id));
    } catch {
      toaster.create({ title: 'No salió la comanda', description: 'El pedido ya está aceptado. Imprímela desde su tarjeta.', type: 'warning' });
    }
  };

  const rechazar = async () => {
    if (!rechazando || !motivo) return;
    const p = rechazando;
    setRechazando(null);
    setDecidiendo(p.id);
    try {
      await rechazarEntrante(p, motivo);
      toaster.create({ title: 'Pedido rechazado', type: 'success' });
    } catch (e) {
      toaster.create(otraTabletaGano(e)
        ? { title: 'Otra tableta ya lo atendió', type: 'info' }
        : { title: 'No se pudo rechazar', description: `${p.sourceName} no lo confirmó. Inténtalo otra vez o atiéndelo en su aplicación.`, type: 'error' });
    } finally {
      setDecidiendo(null);
      setMotivo(null);
      despues();
    }
  };

  return (
    <>
      {/* SIEMPRE EN EL MISMO LUGAR DEL ÁRBOL: aceptar el último pendiente vacía la columna, y una
          comanda que se vuelve a montar olvida que ya salió y se imprime dos veces. */}
      <KitchenTicket order={comanda} />
      {hay && (
        <Box data-por-aceptar w="300px" flexShrink={0} bg="bg.muted" borderRadius="lg" p={2}>
          <HStack mb={2} gap={2}>
            <Text fontWeight="700">Por aceptar</Text>
            <Badge borderRadius="full" px={2}>{pedidos.length}</Badge>
          </HStack>
          {/* Con varios, la columna desplaza sola y no empuja al tablero. */}
          <VStack align="stretch" gap={2} maxH="calc(100dvh - 140px)" overflowY="auto">
            {pedidos.map((p, i) => (
              <TarjetaEntrante key={`${p.source}-${p.id}`} p={p} ahora={ahora} plegada={i > 0}
                aceptando={aceptandoId === p.id} ocupada={decidiendo !== null}
                onAceptar={() => void aceptar(p)} onRechazar={() => setRechazando(p)} />
            ))}
          </VStack>

          <ConfirmSheet
            isOpen={rechazando !== null}
            title={`Rechazar ${rechazando?.displayId ?? ''} de ${rechazando?.sourceName ?? ''}`}
            description={`${rechazando?.sourceName ?? 'La plataforma'} le avisa al cliente con este motivo.`}
            confirmLabel="Rechazar pedido"
            destructive
            confirmDisabled={!motivo}
            onConfirm={() => void rechazar()}
            onCancel={() => { setRechazando(null); setMotivo(null); }}
          >
            <VStack align="stretch" gap={2} role="radiogroup" aria-label="Motivo">
              {MOTIVOS_DE_RECHAZO.map((m) => (
                <Button key={m.code} role="radio" aria-checked={motivo === m.code} aria-label={m.label} minH="52px"
                  justifyContent="flex-start" variant={motivo === m.code ? 'solid' : 'outline'}
                  colorPalette={motivo === m.code ? 'red' : 'gray'} onClick={() => setMotivo(m.code)}>
                  {m.label}
                </Button>
              ))}
            </VStack>
          </ConfirmSheet>
        </Box>
      )}
    </>
  );
}
