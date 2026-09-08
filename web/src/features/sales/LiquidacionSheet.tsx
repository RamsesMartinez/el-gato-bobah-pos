import { useState } from 'react';
import { Box, Button, Grid, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { settlementsApi, type Settlement, type SettlementInput } from '../../api/settlements';
import { money } from '../../utils/format';

interface Props {
  orderId: number;
  plataforma: string;
  isOpen: boolean;
  onClose: () => void;
}

// Lo que el documento de pago de la plataforma dice de un pedido.
//
// SE COPIA, NO SE CALCULA: todo lo que se teclea aquí sale del estado de cuenta. El sistema no
// propone una comisión "del 30%" porque las tasas cambian y las promociones las alteran, y un número
// estimado que se parece a uno medido termina sumándose a algo.
//
// LOS SIETE CAMPOS DE DINERO VAN EN DOS COLUMNAS. Apilados son ~630 px y la pantalla mide 600, antes
// del encabezado y de los ~250 px que se lleva el teclado numérico al abrirse.
export function LiquidacionSheet({ orderId, plataforma, isOpen, onClose }: Props) {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ['settlement', orderId],
    queryFn: () => settlementsApi.get(orderId),
    enabled: isOpen,
    // Un 404 es la respuesta legítima de "todavía no llega el documento", no un fallo que reintentar.
    retry: false,
  });

  const [f, setF] = useState<SettlementInput>(vacia);
  const [tocado, setTocado] = useState(false);
  const [error, setError] = useState('');

  // El formulario arranca con lo capturado, si ya hay. No se usa un efecto: `data` cambia una sola
  // vez al cargar, y sembrar el estado desde un efecto es un render en cascada.
  const valores = tocado ? f : (data ? deLaLiquidacion(data) : vacia);

  const cambiar = (k: keyof SettlementInput) => (v: string) => {
    setTocado(true);
    setF({ ...valores, [k]: v });
    setError('');
  };

  const guardar = useMutation({
    mutationFn: () => settlementsApi.save(orderId, valores),
    onSuccess: () => {
      setTocado(false);
      void qc.invalidateQueries({ queryKey: ['settlement', orderId] });
      void qc.invalidateQueries({ queryKey: ['platform-money'] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  // Lo que puso el restaurante es DERIVADO y se recalcula al teclear: el operador ve de inmediato
  // cuánto le costó la promoción, que es la cifra por la que existe esta pantalla.
  const restaurante = resta(valores.discountTotal, valores.discountPlatform);

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      {/* dvh: con el teclado abierto la ventana visual se encoge y `vh` seguiría midiendo la
          pantalla completa, dejando el botón de guardar debajo del teclado. */}
      <DrawerContent borderTopRadius="l3" maxH="92dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <Text fontSize="lg" fontWeight="700">Liquidación de {plataforma}</Text>
          <Text fontSize="sm" color="fg.muted">
            Copia los importes tal como vienen en el documento de pago.
          </Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={2}>
          {isLoading ? (
            <Text color="fg.muted">Cargando…</Text>
          ) : (
            <VStack align="stretch" gap={3}>
              <Grid templateColumns="repeat(2, 1fr)" gap={3}>
                <Campo etiqueta="Venta que reporta" valor={valores.reportedGross} onChange={cambiar('reportedGross')} />
                <Campo etiqueta="Comisión" valor={valores.commissionAmount} onChange={cambiar('commissionAmount')} />
                <Campo etiqueta="Tasa de comisión (%)" valor={valores.commissionPct ?? ''}
                  onChange={(v) => { setTocado(true); setF({ ...valores, commissionPct: v === '' ? null : v }); setError(''); }}
                  ayuda="Vacío si el documento no la dice" />
                <Campo etiqueta="Retenciones" valor={valores.withholdings} onChange={cambiar('withholdings')} />
                <Campo etiqueta="Descuento total" valor={valores.discountTotal} onChange={cambiar('discountTotal')} />
                <Campo etiqueta="Lo puso la plataforma" valor={valores.discountPlatform} onChange={cambiar('discountPlatform')} />
                <Campo etiqueta="Depositado (neto)" valor={valores.netAmount} onChange={cambiar('netAmount')}
                  ayuda="Puede ser negativo" />
                <Box>
                  <Text fontSize="xs" color="fg.muted" mb={1}>Lo puso el restaurante</Text>
                  {/* Derivado, no capturado: es descuento total menos lo que puso la plataforma.
                      Se muestra porque es la cifra que dice cuánto costó la promoción. */}
                  <Box minH="44px" display="flex" alignItems="center" px={3}
                    borderWidth="1px" borderRadius="md" bg="bg.subtle">
                    <Text fontWeight="700" aria-label="Lo puso el restaurante">{money(restaurante)}</Text>
                  </Box>
                </Box>
              </Grid>
              <Campo etiqueta="Referencia del depósito" valor={valores.payoutReference}
                onChange={cambiar('payoutReference')} ancho />
              <Campo etiqueta="Documento" valor={valores.documentRef} onChange={cambiar('documentRef')} ancho />
              {error && <Text fontSize="sm" color="red.fg">{error}</Text>}
              {data && (
                <Text fontSize="xs" color="fg.muted">
                  Capturada por {data.capturedBy || 'alguien'} el {data.capturedAt.slice(0, 10)}
                </Text>
              )}
            </VStack>
          )}
        </DrawerBody>
        {/* Footer FIJO: con el teclado abierto es lo único que mantiene el botón a la vista. */}
        <DrawerFooter borderTopWidth="1px" pt={3}>
          <HStack gap={2} w="100%">
            <Button flex="1" minH="52px" variant="outline" onClick={onClose}>Cancelar</Button>
            <Button flex="2" minH="52px" colorPalette="orange"
              disabled={guardar.isPending}
              onClick={() => guardar.mutate()}>
              Guardar liquidación
            </Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

function Campo({ etiqueta, valor, onChange, ayuda, ancho }: {
  etiqueta: string; valor: string; onChange: (v: string) => void; ayuda?: string; ancho?: boolean;
}) {
  return (
    <Box gridColumn={ancho ? '1 / -1' : undefined}>
      <Text fontSize="xs" color="fg.muted" mb={1}>{etiqueta}</Text>
      <Input size="sm" minH="44px" aria-label={etiqueta} value={valor}
        onChange={(e) => onChange(e.target.value)}
        // inputMode y no type="number": el teclado numérico sale igual y el campo no hereda las
        // flechitas ni el redondeo del navegador, que en una tableta son dos formas de cambiar un
        // importe sin querer.
        inputMode="decimal" autoComplete="off" spellCheck={false} />
      {ayuda && <Text fontSize="xs" color="fg.muted" mt={1}>{ayuda}</Text>}
    </Box>
  );
}

const vacia: SettlementInput = {
  reportedGross: '', commissionAmount: '', commissionPct: null,
  discountTotal: '', discountPlatform: '', withholdings: '', netAmount: '',
  payoutReference: '', documentRef: '',
};

function deLaLiquidacion(s: Settlement): SettlementInput {
  return {
    reportedGross: s.reportedGross, commissionAmount: s.commissionAmount,
    commissionPct: s.commissionPct, discountTotal: s.discountTotal,
    discountPlatform: s.discountPlatform, withholdings: s.withholdings,
    netAmount: s.netAmount, payoutReference: s.payoutReference, documentRef: s.documentRef,
  };
}

// Resta de dos importes tecleados. Un campo vacío vale 0 aquí —todavía se está capturando— y quien
// decide si la combinación es válida es el SERVIDOR: si la parte de la plataforma excede el total,
// lo rechaza con un 400 que dice exactamente eso.
function resta(total: string, parte: string): number {
  return (Number(total) || 0) - (Number(parte) || 0);
}
