import { useState } from 'react';
import { Button, HStack, Input, Text, VStack } from '@chakra-ui/react';

import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { Field } from '../../components/ui/field';
import { toaster } from '../../components/ui/toaster';
import { usePlatformPrice } from '../../hooks/usePlatformPrice';
import { useUiStore } from '../../stores/ui';
import { money } from '../../utils/format';
import type { DesglosePrecio } from './precioPlataforma';

// Corregir el precio de UN producto en UNA plataforma, desde la pantalla de venta.
//
// El desglose no es adorno: el operador está corrigiendo un número que el sistema calculó, y sin
// ver de dónde salió corrige a ciegas. Por eso se muestran el de mostrador y el calculado, ambos
// como referencia y ninguno editable — el precio de mostrador se cambia en el catálogo, y
// confundir las dos listas es el error que esta pantalla no puede permitir.
export function PlatformPriceDialog({ productId, productName, plataforma, plataformaId, desglose, isOpen, onClose }: {
  productId: number;
  productName: string;
  plataforma: string;
  plataformaId: number;
  desglose: DesglosePrecio;
  isOpen: boolean;
  onClose: () => void;
}) {
  const palette = useUiStore((s) => s.palette);
  const { guardar, quitar } = usePlatformPrice();
  const [precio, setPrecio] = useState(String(desglose.vigente));

  const valor = parseFloat(precio);
  const valido = Number.isFinite(valor) && valor > 0;

  const onGuardar = () =>
    guardar.mutate({ productId, platformId: plataformaId, price: valor }, {
      onSuccess: () => {
        toaster.create({ title: `${productName}: ${money(valor)} en ${plataforma}`, type: 'success' });
        onClose();
      },
      onError: (e) => toaster.create({ title: 'No se pudo guardar', description: String(e), type: 'error' }),
    });

  const onQuitar = () =>
    quitar.mutate({ productId, platformId: plataformaId }, {
      onSuccess: () => {
        toaster.create({ title: `${productName} vuelve a ${money(desglose.calculado)}`, type: 'success' });
        onClose();
      },
      onError: (e) => toaster.create({ title: 'No se pudo quitar', description: String(e), type: 'error' }),
    });

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader>
          <DialogTitle>{productName} en {plataforma}</DialogTitle>
        </DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          <VStack align="stretch" gap={4}>
            <VStack align="stretch" gap={1}>
              <HStack justify="space-between">
                <Text color="fg.muted">Precio de mostrador</Text>
                <Text fontWeight="600">{money(desglose.base)}</Text>
              </HStack>
              <HStack justify="space-between">
                <Text color="fg.muted">Precio calculado</Text>
                <Text fontWeight="600">{money(desglose.calculado)}</Text>
              </HStack>
            </VStack>
            <Field label={`Precio en ${plataforma}`}>
              <Input
                type="number" inputMode="decimal" step="0.01" min="0"
                size="lg" fontSize="xl" fontWeight="700"
                value={precio}
                onChange={(e) => setPrecio(e.target.value)}
                autoFocus
              />
            </Field>
            {desglose.esManual && (
              <Text fontSize="sm" color="fg.muted">
                Este producto tiene un precio capturado a mano. Quítalo para que vuelva a {money(desglose.calculado)}.
              </Text>
            )}
          </VStack>
        </DialogBody>
        <DialogFooter>
          {desglose.esManual && (
            <Button variant="outline" colorPalette="red" mr="auto" minH="48px"
              loading={quitar.isPending} onClick={onQuitar}>
              Quitar precio
            </Button>
          )}
          <Button variant="ghost" mr={3} minH="48px" onClick={onClose}>Cancelar</Button>
          <Button minH="48px" loading={guardar.isPending} disabled={!valido} onClick={onGuardar}>
            Guardar
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
