import { useState } from 'react';
import { Stack, Text } from '@chakra-ui/react';
import { Picker } from '../../../components/Picker';
import { ConfirmSheet } from '../../../components/ConfirmSheet';
import { useLocations } from '../useLocations';

// ArticlePlaceField es la «Ubicación» de la ficha: el lugar donde se cuenta el artículo. Es UNO, de
// cualquier almacén (decisión de diseño de la spec 094, ver docs/mecanica/almacen.md): el artículo
// entra al recorrido del conteo del almacén de ese lugar y a ningún otro. Con dos o más almacenes, el
// campo dice de cuál es, y elegir un lugar de otro almacén se confirma, porque saca al artículo del
// recorrido del primero.
export function ArticlePlaceField({ name, value, disabled, onChange }: {
  name: string; value: string; disabled?: boolean; onChange: (v: string) => void;
}) {
  const locations = useLocations(undefined, { all: true });
  const [pending, setPending] = useState<string | null>(null);
  const current = locations.warehouseOf(value);
  const choose = (v: string) => {
    const to = locations.warehouseOf(v);
    if (v !== '' && current && to && to !== current) {
      // En la vuelta siguiente: la hoja del Picker se está cerrando en esta misma actualización, y una
      // hoja que nace abierta mientras otra se cierra no se monta (AGENTS.md §3, CobrarSheet).
      setTimeout(() => setPending(v), 0);
      return;
    }
    onChange(v);
  };
  const to = pending === null ? undefined : locations.warehouseOf(pending);
  return (
    <Stack gap={1}>
      <Text>Ubicación</Text>
      <Picker {...locations.pickerProps} disabled={disabled} value={value} options={locations.options} onChange={choose} />
      {current && <Text fontSize="sm" color="fg.muted">Se cuenta en {current}</Text>}
      <ConfirmSheet isOpen={pending !== null} title={`¿Contar ${name} en ${to ?? ''}?`}
        description={`${name} pasa al recorrido del conteo de ${to ?? ''} y sale del de ${current ?? ''}. Un artículo se cuenta en un solo lugar.`}
        confirmLabel="Cambiar de almacén" confirmPalette="brand"
        onConfirm={() => { if (pending !== null) onChange(pending); setPending(null); }}
        onCancel={() => setPending(null)} />
    </Stack>
  );
}
