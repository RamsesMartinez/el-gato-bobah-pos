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
export function ColumnaPorAceptar({ oculta = false }: { oculta?: boolean }) {
  const { pedidos } = useEntrantes();
  const qc = useQueryClient();
  const [decidiendo, setDecidiendo] = useState<number | null>(null);
  const [aceptandoId, setAceptandoId] = useState<number | null>(null);
  const [rechazando, setRechazando] = useState<PedidoEntrante | null>(null);
  // El motivo se guarda con el pedido al que se eligió: no se arrastra a otro.
  const [eleccion, setEleccion] = useState<{ para: string; code: MotivoDeRechazo } | null>(null);
  // La comanda la imprime SOLO la tableta que aceptó; las demás se enteran por el aviso.
  const [comanda, setComanda] = useState<OrderView | null>(null);
  const [ahora, setAhora] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setAhora(Date.now()), 15_000);
    return () => clearInterval(t);
  }, []);

  const hay = pedidos.length > 0 && !oculta;
  useEffect(() => {
    useColumnaPorAceptar.setState({ visible: hay });
    return () => useColumnaPorAceptar.setState({ visible: false });
  }, [hay]);

  // La hoja de rechazo no sobrevive a su pedido: si otra tableta lo decidió, al llegar el siguiente
  // reaparecería ofreciendo rechazar el anterior con el motivo ya puesto. Se deriva, no se limpia.
  const llave = (p: PedidoEntrante) => `${p.source}-${p.id}`;
  const rechazo = rechazando && pedidos.some((p) => llave(p) === llave(rechazando)) ? rechazando : null;
  const motivo = rechazo && eleccion?.para === llave(rechazo) ? eleccion.code : null;
  const setMotivo = (code: MotivoDeRechazo | null) => setEleccion(code && rechazo ? { para: llave(rechazo), code } : null);

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
      // Que otra tableta ganara es el funcionamiento normal con varias: se dice sin alarmar. Sin
      // respuesta del servidor no se sabe: pudo aceptarse y perderse la respuesta, y entonces la
      // comanda no salió en ninguna tableta. Se dice dónde imprimirla en vez de invitar a reintentar.
      toaster.create(otraTabletaGano(e)
        ? { title: 'Otra tableta ya lo atendió', type: 'info' }
        : !(e instanceof ApiError)
          ? { title: 'No se supo si quedó aceptado', description: 'Si aparece en «En preparación», imprime su comanda desde su tarjeta.', type: 'warning' }
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
    if (!rechazo || !motivo) return;
    const p = rechazo;
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
          {/* Sin scroll propio: la página ya desplaza, y un scroll dentro de otro hacía que el dedo
              moviera uno u otro según dónde tocara. */}
          <VStack align="stretch" gap={2}>
            {pedidos.map((p, i) => (
              <TarjetaEntrante key={`${p.source}-${p.id}`} p={p} ahora={ahora} plegada={i > 0}
                aceptando={aceptandoId === p.id} ocupada={decidiendo !== null}
                onAceptar={() => void aceptar(p)} onRechazar={() => setRechazando(p)} />
            ))}
          </VStack>

          <ConfirmSheet
            isOpen={rechazo !== null}
            title={`Rechazar ${rechazo?.displayId ?? ''} de ${rechazo?.sourceName ?? ''}`}
            description={`${rechazo?.sourceName ?? 'La plataforma'} le avisa al cliente con este motivo.`}
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
