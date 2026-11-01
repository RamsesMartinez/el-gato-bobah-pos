import { Button, Checkbox } from '@chakra-ui/react';

import { ConfirmSheet } from '../../components/ConfirmSheet';
import {
  BEFORE_LAST_CLOSE_TEXT, INCLUDE_IN_COUNT_LABEL, OUTSIDE_COUNT_CONFIRM, OUTSIDE_COUNT_TEXT, type CountChoice, type OutsideAsk,
} from './cajaDelGasto';

// OutsideCountSheet es el aviso de un pago en efectivo que no entra al corte (spec 048, R9). Conserva
// sus dos botones de siempre; «sin incluir» va aparte, en el cuerpo, para que el dedo que confirma
// no lo toque por accidente. Si ningún corte puede cubrir el pago, lo único que queda es «sin incluir».
export function OutsideCountSheet({ ask, loading, onCancel, onChoose }: {
  ask: OutsideAsk | null;
  loading?: boolean;
  onCancel: () => void;
  onChoose: (choice: CountChoice) => void;
}) {
  const canInclude = ask?.canInclude ?? true;
  return (
    <ConfirmSheet isOpen={ask !== null} title={ask?.title ?? ''}
      description={canInclude ? OUTSIDE_COUNT_TEXT : BEFORE_LAST_CLOSE_TEXT}
      confirmLabel={canInclude ? 'Registrar para el siguiente corte' : OUTSIDE_COUNT_CONFIRM} cancelLabel="Cancelar" loading={loading}
      onCancel={onCancel} onConfirm={() => onChoose(canInclude ? 'pending' : 'exclude')}>
      {canInclude && (
        <Button mt={3} minH="44px" variant="ghost" disabled={loading} onClick={() => onChoose('exclude')}>
          {OUTSIDE_COUNT_CONFIRM}
        </Button>
      )}
    </ConfirmSheet>
  );
}

// IncludeInCountCheckbox es la casilla «Incluir en el arqueo» de un pago en efectivo (R10): va en la
// línea del pago, sin campo aparte.
export function IncludeInCountCheckbox({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <Checkbox.Root minH="44px" checked={checked} onCheckedChange={(e) => onChange(e.checked === true)}>
      <Checkbox.HiddenInput />
      <Checkbox.Control />
      <Checkbox.Label>{INCLUDE_IN_COUNT_LABEL}</Checkbox.Label>
    </Checkbox.Root>
  );
}
