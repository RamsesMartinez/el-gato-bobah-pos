import { useState } from 'react';
import { Button, HStack, Input, Text, VStack } from '@chakra-ui/react';

import { Field } from '../../components/ui/field';
import { toaster } from '../../components/ui/toaster';
import { usePlatformOptionPrice } from '../../hooks/usePlatformPrice';
import { money } from '../../utils/format';
import type { DesglosePrecio } from './precioPlataforma';

// Corregir el cargo de UN extra en UNA plataforma, dentro del diálogo que ya existía para
// gestionar la opción. No abre uno propio: el gesto para llegar aquí —mantener presionado— ya
// llevaba a ese diálogo, y dos diálogos distintos según el rol serían dos cosas que aprender.
//
// Se muestran el cargo de mostrador y el calculado como referencia y ninguno es editable: el de
// mostrador se cambia en el catálogo, y confundir las dos listas es el error que esta pantalla no
// puede permitir.
export function OptionPriceFields({ optionId, optionName, plataforma, plataformaId, desglose, onDone }: {
  optionId: number;
  optionName: string;
  plataforma: string;
  plataformaId: number;
  desglose: DesglosePrecio;
  onDone: () => void;
}) {
  const { guardar, quitar } = usePlatformOptionPrice();
  const [delta, setDelta] = useState(String(desglose.vigente));

  const valor = parseFloat(delta);
  // >= 0 y no > 0 como en los productos: un extra sin costo ("sin cebolla") es normal y su cargo
  // es 0. Lo que no vale es negativo. Es la misma regla que el check de la tabla.
  const valido = Number.isFinite(valor) && valor >= 0;

  const onGuardar = () =>
    guardar.mutate({ optionId, platformId: plataformaId, priceDelta: valor }, {
      onSuccess: () => {
        toaster.create({ title: `${optionName}: ${money(valor)} en ${plataforma}`, type: 'success' });
        onDone();
      },
      onError: (e) => toaster.create({ title: 'No se pudo guardar', description: String(e), type: 'error' }),
    });

  const onQuitar = () =>
    quitar.mutate({ optionId, platformId: plataformaId }, {
      onSuccess: () => {
        toaster.create({ title: `${optionName} vuelve a ${money(desglose.calculado)}`, type: 'success' });
        onDone();
      },
      onError: (e) => toaster.create({ title: 'No se pudo quitar', description: String(e), type: 'error' }),
    });

  return (
    <VStack align="stretch" gap={3}>
      <VStack align="stretch" gap={1}>
        <HStack justify="space-between">
          <Text fontSize="sm" color="fg.muted">Cargo de mostrador</Text>
          <Text fontSize="sm" fontWeight="600">{money(desglose.base)}</Text>
        </HStack>
        <HStack justify="space-between">
          <Text fontSize="sm" color="fg.muted">Cargo calculado</Text>
          <Text fontSize="sm" fontWeight="600">{money(desglose.calculado)}</Text>
        </HStack>
      </VStack>
      <Field label={`Cargo en ${plataforma}`}>
        <Input
          type="number" inputMode="decimal" step="0.01" min="0"
          size="lg" fontSize="xl" fontWeight="700"
          value={delta}
          onChange={(e) => setDelta(e.target.value)}
        />
      </Field>
      <Button size="lg" minH="48px" loading={guardar.isPending} disabled={!valido} onClick={onGuardar}>
        Guardar cargo
      </Button>
      {desglose.esManual && (
        <Button size="lg" minH="48px" variant="outline" loading={quitar.isPending} onClick={onQuitar}>
          Quitar cargo capturado
        </Button>
      )}
    </VStack>
  );
}
