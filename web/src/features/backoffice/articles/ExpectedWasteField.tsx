import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { HStack, Input, Text, VStack } from '@chakra-ui/react';
import { backofficeApi } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { Segmented } from '../../../components/Segmented';
import { InfoButton } from '../../../components/InfoButton';
import { pctTecleado, pctTexto } from '../../../shared/merma/pct';

type Mode = 'sin' | 'con';

// ExpectedWasteField es «Merma al preparar» en la ficha del artículo (spec 096, maqueta A2). «Sin
// capturar» deja que mande el producto o la general; «Capturada» fija la del artículo, 0 incluido. Se
// guarda sola: al elegir «Sin capturar» y al salir del campo, como el resto de la ficha.
export function ExpectedWasteField({ id, value, generalPct, canEdit }: {
  id: number;
  value: string | null | undefined;
  generalPct: string | null | undefined;
  canEdit: boolean;
}) {
  const qc = useQueryClient();
  const [mode, setMode] = useState<Mode>(value == null ? 'sin' : 'con');
  const [text, setText] = useState(pctTexto(value));
  const [saved, setSaved] = useState<number | null>(value == null ? null : Number(value));
  const [info, setInfo] = useState(false);
  const [error, setError] = useState('');
  const save = useMutation({
    mutationFn: (pct: number | null) => backofficeApi.setArticleExpectedWaste(id, pct),
    onSuccess: (_r, pct) => {
      setSaved(pct);
      setError('');
      void qc.invalidateQueries({ queryKey: ['article', id] });
      void qc.invalidateQueries({ queryKey: ['admin'] });
    },
    onError: (e) => setError(e instanceof ApiError && e.status === 409
      ? 'Se está recalculando el costo; intenta en un momento'
      : 'No se pudo guardar'),
  });
  const typed = pctTecleado(text);
  const commit = () => {
    if (typed === null || typed === 'invalido' || typed === saved) return;
    save.mutate(typed);
  };
  const pick = (m: Mode) => {
    setMode(m);
    if (m === 'sin' && saved !== null) {
      setText('');
      save.mutate(null);
    }
  };
  const general = pctTexto(generalPct);

  return (
    <VStack align="stretch" gap={2} bg="bg.muted" borderRadius="lg" p={3}>
      <HStack justify="space-between">
        <Text fontWeight="600">Merma al preparar</Text>
        <InfoButton label="Qué es la merma al preparar" open={info} onToggle={() => setInfo(!info)} />
      </HStack>
      {info && (
        <Text fontSize="sm" color="fg.muted">
          Lo que se pierde al usarlo: el rabo de la fresa, lo que queda en el envase. Se suma al costo de
          cada producto que lo lleva.
        </Text>
      )}
      <Segmented label="Merma al preparar" value={mode} onChange={pick} disabled={!canEdit}
        options={[{ value: 'sin', label: 'Sin capturar' }, { value: 'con', label: 'Capturada' }]} />
      {mode === 'con' ? (
        <HStack gap={2}>
          <Input aria-label="Merma del artículo, en porcentaje" maxW="110px" minH="44px" bg="bg" inputMode="decimal"
            value={text} disabled={!canEdit} aria-invalid={typed === 'invalido'}
            onChange={(e) => { setText(e.target.value); setError(''); }}
            onBlur={commit} onKeyDown={(e) => { if (e.key === 'Enter') commit(); }} />
          <Text>%</Text>
          {typed === 0 && <Text fontSize="sm" color="fg.muted">Aquí no se pierde nada</Text>}
        </HStack>
      ) : (
        <Text fontSize="sm" color="fg.muted">Usa la del producto o la general{general ? ` (${general} %)` : ''}</Text>
      )}
      {typed === 'invalido' && <Text fontSize="sm" color="fg.error">Escribe de 0 a 99.99</Text>}
      {error && <Text fontSize="sm" color="fg.error">{error}</Text>}
    </VStack>
  );
}
