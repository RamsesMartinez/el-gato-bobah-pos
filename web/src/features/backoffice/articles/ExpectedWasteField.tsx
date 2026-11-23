import { useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
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
//
// Las peticiones van en fila y se compara contra lo último PEDIDO, no contra lo confirmado: tocar
// «Sin capturar» con el 5 % todavía en camino tiene que terminar en null (revisión de código). Un
// campo vacío al salir es «Sin capturar», nunca 0, y la pantalla nunca se queda en «Capturada» vacía.
export function ExpectedWasteField({ id, value, generalPct, canEdit }: {
  id: number;
  value: string | null | undefined;
  generalPct: string | null | undefined;
  canEdit: boolean;
}) {
  const qc = useQueryClient();
  const initial = value == null ? null : Number(value);
  const [mode, setMode] = useState<Mode>(initial === null ? 'sin' : 'con');
  const [text, setText] = useState(pctTexto(value));
  const [info, setInfo] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  const confirmed = useRef<number | null>(initial);
  const requested = useRef<number | null>(initial);
  const queue = useRef<Promise<unknown>>(Promise.resolve());

  const send = (pct: number | null) => {
    if (pct === requested.current) return;
    requested.current = pct;
    queue.current = queue.current.then(async () => {
      try {
        await backofficeApi.setArticleExpectedWaste(id, pct);
        confirmed.current = pct;
        setError('');
        setDone(true);
        void qc.invalidateQueries({ queryKey: ['article', id] });
        void qc.invalidateQueries({ queryKey: ['articles'] });
        void qc.invalidateQueries({ queryKey: ['article-audit', id] });
        void qc.invalidateQueries({ queryKey: ['admin'] });
      } catch (e) {
        // Lo que sigue guardado vuelve a la pantalla: decir «Sin capturar» con el % todavía sumando
        // al costo sería mentir, y además no se podría reintentar tocando el mismo segmento.
        requested.current = confirmed.current;
        setMode(confirmed.current === null ? 'sin' : 'con');
        setText(pctTexto(confirmed.current));
        setError(e instanceof ApiError && e.status < 500 ? e.message : 'No se pudo guardar');
      }
    });
  };
  const typed = pctTecleado(text);
  const commit = () => {
    if (typed === 'invalido') return;
    if (typed === null) {
      setMode('sin');
      send(null);
      return;
    }
    send(typed);
  };
  const pick = (m: Mode) => {
    setMode(m);
    if (m === 'sin') {
      setText('');
      send(null);
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
            placeholder="0 a 99.99" autoFocus={text === ''} value={text} disabled={!canEdit} aria-invalid={typed === 'invalido'}
            onChange={(e) => { setText(e.target.value); setError(''); setDone(false); }}
            onBlur={commit} onKeyDown={(e) => { if (e.key === 'Enter') commit(); }} />
          <Text>%</Text>
          {typed === 0 && <Text fontSize="sm" color="fg.muted">Aquí no se pierde nada</Text>}
        </HStack>
      ) : (
        <Text fontSize="sm" color="fg.muted">Usa la del producto o la general{general ? ` (${general} %)` : ''}</Text>
      )}
      {typed === 'invalido' && <Text fontSize="sm" color="fg.error">Escribe de 0 a 99.99</Text>}
      {/* Se guarda sola al salir del campo; sin decirlo, quien la probó buscó un botón Guardar. */}
      {done && !error && <Text fontSize="sm" color="fg.success">Guardado</Text>}
      {error && <Text fontSize="sm" color="fg.error">{error}</Text>}
    </VStack>
  );
}
