import { useEffect, useState } from 'react';
import { Box, Button, HStack, Input, Text, VStack } from '@chakra-ui/react';

import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerCloseTrigger } from '../../components/ui/drawer';
import { Picker } from '../../components/Picker';
import { montoDevolvible, porQueNoSeDevuelve, sePuedeDevolver } from '../../domain/devolucion';
import { parseMonto, round2 } from '../../domain/numeros';
import { money } from '../../utils/format';
import type { BoardOrder } from '../../types/pos';

// 44 px es el mínimo con el que un dedo acierta a la primera.
const TAP = '44px';

const MOTIVOS = [
  'Producto en mal estado',
  'Se equivocó el pedido',
  'El cliente se arrepintió',
  'Demora en la entrega',
  'Cobro duplicado',
];

interface Props {
  pedido: BoardOrder;
  // cancelando: la devolución es parte de cancelar el pedido, no una devolución suelta. Cambia el
  // texto, no las reglas: el dinero se resuelve igual.
  cancelando?: boolean;
  enviando: boolean;
  onCerrar: () => void;
  onConfirmar: (monto: number, motivo: string) => void;
}

// La hoja con la que se devuelve dinero.
//
// Reemplaza al `window.prompt` que pedía el motivo. Ese diálogo lo pinta el sistema operativo: caja
// de texto muy por debajo de 44 px, los motivos listados entre paréntesis sin poder tocarlos, y —lo
// peor— tras varios diálogos Chrome ofrece suprimirlos, y a partir de ahí `prompt` devuelve null y
// la acción deja de hacer nada, en silencio.
//
// El monto se puede editar porque devolver una PARTE es un caso real: un platillo de tres. Arranca
// con todo lo que queda, que es lo de casi siempre.
export function DevolucionSheet({ pedido, cancelando, enviando, onCerrar, onConfirmar }: Props) {
  // Monta cerrada y se abre en el render siguiente: sin una transición de cerrado a abierto de
  // verdad, una hoja que nace con `open` puesto no se monta si otra se está cerrando en la misma
  // actualización (ver el comentario de CobrarSheet).
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(true); }, []);

  const cobrado = round2(Number(pedido.total) - Number(pedido.outstanding));
  const yaDevuelto = round2(Number(pedido.refund ?? 0));
  const queda = montoDevolvible(cobrado, yaDevuelto);

  const [monto, setMonto] = useState(queda > 0 ? String(queda) : '');
  const [motivo, setMotivo] = useState(MOTIVOS[0]);

  const m = parseMonto(monto);
  const valor = m.estado === 'valido' ? m.valor : NaN;
  // Cancelar un pedido SIN cobros no devuelve nada: solo pide el motivo. Exigirle un monto sería
  // pedirle al cajero que devuelva un dinero que nunca entró.
  const soloCancelar = cancelando === true && cobrado <= 0;
  const impedimento = soloCancelar
    ? (motivo.trim() === '' ? 'sin-motivo' as const : null)
    : sePuedeDevolver(valor, cobrado, yaDevuelto, motivo);

  return (
    <DrawerRoot open={visible} placement="bottom" size="md"
      onOpenChange={(e) => { if (!e.open) onCerrar(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerCloseTrigger />
        <VStack align="stretch" gap={3} p={4} pb={5}>
          <Box>
            <Text fontWeight="800" fontSize="lg">
              {soloCancelar ? 'Cancelar pedido' : cancelando ? 'Cancelar y devolver' : 'Devolver dinero'}
            </Text>
            <Text fontSize="sm" color="fg.muted">
              {pedido.folioName || `#${pedido.number}`}
            </Text>
          </Box>

          {!soloCancelar && (
          <>
          {/* De dónde sale el tope, a la vista. Un monto que se rechaza sin decir contra qué se
              comparó manda al operador a adivinar. */}
          <HStack justify="space-between" fontSize="sm">
            <Text color="fg.muted">Se cobró</Text>
            <Text fontWeight="700">{money(cobrado, pedido.currency)}</Text>
          </HStack>
          {yaDevuelto > 0 && (
            <HStack justify="space-between" fontSize="sm">
              <Text color="fg.muted">Ya se devolvió</Text>
              <Text fontWeight="700">{money(yaDevuelto, pedido.currency)}</Text>
            </HStack>
          )}

          <Box>
            <Text fontSize="sm" color="fg.muted" mb={1}>Cuánto se devuelve</Text>
            <Input inputMode="decimal" minH={TAP} fontSize="lg" fontWeight="700"
              aria-label="Cuánto se devuelve"
              value={monto} onChange={(e) => setMonto(e.target.value)} />
            {queda > 0 && round2(valor) !== queda && (
              <Button mt={2} size="sm" minH={TAP} px={4} variant="outline" colorPalette="gray"
                onClick={() => setMonto(String(queda))}>
                Todo lo que queda · {money(queda, pedido.currency)}
              </Button>
            )}
          </Box>

          </>
          )}

          <Box>
            <Text fontSize="sm" color="fg.muted" mb={1}>Por qué</Text>
            <Picker value={motivo} onChange={setMotivo} title="Motivo"
              options={MOTIVOS.map((v) => ({ value: v, label: v }))} />
          </Box>

          {/* Por qué está apagado el botón. Un botón muerto sin razón visible es la peor forma de
              rechazar algo: el operador toca, no pasa nada, y vuelve a tocar. */}
          {impedimento && (
            <Text fontSize="sm" color="fg.error" role="status">
              {porQueNoSeDevuelve(impedimento)}
            </Text>
          )}

          <Button size="lg" h="56px" colorPalette="red" fontWeight="800"
            loading={enviando} disabled={impedimento !== null}
            onClick={() => onConfirmar(soloCancelar ? 0 : round2(valor), motivo)}>
            {soloCancelar
              ? 'Cancelar pedido'
              : `${cancelando ? 'Cancelar y devolver' : 'Devolver'} ${impedimento ? '' : money(round2(valor), pedido.currency)}`}
          </Button>
        </VStack>
      </DrawerContent>
    </DrawerRoot>
  );
}
