import { useEffect, useState } from 'react';
import { AltMatchTag } from '../../../components/AltMatchTag';
import { medirAccion } from '../../../api/uso';
import { Badge, Box, Button, HStack, Input, Stack, Table, Text } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { backofficeApi, type ArticleFilter, type ArticleRow } from '../../../api/backoffice';
import { Picker } from '../../../components/Picker';
import { can } from '../../../app/permissions';
import { useSessionStore } from '../../../stores/session';
import { formatBaseQty, formatUnitCost } from '../../../shared/units';
import { NO_LOCATION } from '../useLocations';
import { NewArticleSheet } from './NewArticleSheet';
import { presentationLabel } from '../../../shared/presentationLabel';
import { DuplicatesSheet } from './DuplicatesSheet';
import { AlertasPorLote } from './AlertasPorLote';
import { notificationsApi } from '../../../api/notifications';

// Almacén › Artículos (spec 042, maqueta «Lista de artículos», opción A con los ajustes de la
// revisión): en la fila del buscador van «Lugar», «Activos / Archivados» y «+ Nuevo»; en chips, los dos
// filtros de sí o no. Cada filtro es un toque y siempre se ve cuál está puesto.

interface Props {
  onOpen: (id: number) => void;
  // notice: el «No lo encuentro» que se va a dar de alta (spec 084); onNoticeDone limpia la liga.
  notice?: { name: string; id: number };
  onNoticeDone?: () => void;
  // La palabra de la empresa para lo que se guarda (spec 038), en singular: «artículo», «insumo».
  word?: string;
}

// Espera tras la última tecla antes de preguntar al servidor: escribir «hielo» no son cinco consultas.
const SEARCH_DELAY_MS = 250;

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

export function ArticlesTab({ onOpen, word = 'artículo', notice, onNoticeDone }: Props) {
  const Word = word.charAt(0).toUpperCase() + word.slice(1);
  const user = useSessionStore((s) => s.user);
  const canConfigure = can('inventory.configure', user);
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<Omit<ArticleFilter, 'q'>>({});
  // «Dar de alta» desde un aviso de «No lo encuentro» (spec 084): al crear, el aviso se cierra como «se
  // dio de alta».
  const fromNotice = notice?.name ?? null;
  const noticeID = notice?.id ?? 0;
  const [creating, setCreating] = useState(fromNotice !== null);
  // Un aviso nuevo con la pestaña ya abierta (se tocó «Dar de alta» estando aquí) solo cambia la liga:
  // se abre la hoja al pintar, ajustando el estado a la prop nueva.
  const [noticeVisto, setNoticeVisto] = useState(noticeID);
  if (noticeID !== noticeVisto) {
    setNoticeVisto(noticeID);
    if (noticeID) setCreating(true);
  }
  const [duplicates, setDuplicates] = useState(false);
  const [porLote, setPorLote] = useState(false);
  const debounced = useDebounced(q, SEARCH_DELAY_MS);
  const f: ArticleFilter = { ...filter, q: debounced };
  const list = useQuery({ queryKey: ['articles', f], queryFn: () => backofficeApi.articles(f) });
  const places = useQuery({ queryKey: ['count-places'], queryFn: backofficeApi.countPlaces });
  // Posibles duplicados (spec 056): el botón solo aparece con grupos y para quien puede unir.
  const groups = useQuery({ queryKey: ['duplicate-groups'], queryFn: backofficeApi.duplicateGroups, enabled: canConfigure });
  const groupCount = (groups.data?.groups ?? []).length;
  useEffect(() => { medirAccion('inventario', 'ver-articulos'); }, []);

  const placeValue = filter.place === undefined ? '' : String(filter.place);
  const placeOptions = [
    { value: 'none', label: NO_LOCATION },
    ...(places.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
  ];
  const clearNotice = () => onNoticeDone?.();
  const toggle = (k: 'noCost' | 'negative') => setFilter({ ...filter, [k]: !filter[k] });
  const rows = list.data ?? [];

  return (
    <Stack gap={2} flex="1" minH={0}>
      <HStack gap={2} flexShrink={0}>
        <Input flex="1" minH="44px" aria-label={`Buscar ${word} o empaque`} placeholder={`Buscar ${word} o empaque`}
          value={q} onChange={(e) => setQ(e.target.value)} />
        <Box flexShrink={0}>
          <Picker title="Ubicación" placeholder="Ubicación: Todas" clearable clearLabel="Todas" value={placeValue} options={placeOptions}
            onChange={(v) => setFilter({ ...filter, place: v === '' ? undefined : v === 'none' ? 'none' : Number(v) })} />
        </Box>
        <HStack gap={0} borderWidth="1px" borderRadius="full" overflow="hidden" flexShrink={0}>
          {([['Activos', false], ['Archivados', true]] as const).map(([label, archived]) => (
            <Button key={label} minH="44px" borderRadius="0" variant={!!filter.archived === archived ? 'solid' : 'ghost'}
              colorPalette={!!filter.archived === archived ? 'brand' : 'gray'}
              aria-pressed={!!filter.archived === archived} onClick={() => setFilter({ ...filter, archived })}>{label}</Button>
          ))}
        </HStack>
        {canConfigure && (
          <Button minH="44px" colorPalette="brand" flexShrink={0} onClick={() => setCreating(true)}>+ Nuevo</Button>
        )}
      </HStack>
      <HStack gap={2} flexShrink={0} flexWrap="wrap">
        {([['noCost', 'Sin costo'], ['negative', 'Existencia negativa']] as const).map(([k, label]) => (
          <Button key={k} minH="44px" borderRadius="full" variant={filter[k] ? 'subtle' : 'outline'} colorPalette={filter[k] ? 'brand' : 'gray'}
            aria-pressed={!!filter[k]} onClick={() => toggle(k)}>{label}</Button>
        ))}
        <Box flex="1" />
        {canConfigure && <Button minH="44px" variant="outline" onClick={() => setPorLote(true)}>Alertas por lote</Button>}
        {canConfigure && groupCount > 0 && (
          <Button minH="44px" variant="outline" onClick={() => setDuplicates(true)}>
            Posibles duplicados <Badge colorPalette="orange">{groupCount}</Badge>
          </Button>
        )}
      </HStack>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" flex="1" minH={0} overflowY="auto">
        <Table.Root size="sm" stickyHeader>
          <Table.Header>
            <Table.Row>
              <Table.ColumnHeader>{Word}</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end" w="190px">Existencia</Table.ColumnHeader>
              <Table.ColumnHeader>Ubicación</Table.ColumnHeader>
              <Table.ColumnHeader>Costo</Table.ColumnHeader>
              <Table.ColumnHeader>Empaques</Table.ColumnHeader>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {!list.isLoading && rows.length === 0 && (
              <Table.Row><Table.Cell colSpan={5}>
                <Text color="fg.muted" py={4}>Nada coincide. Revisa los filtros{canConfigure ? ' o crea uno con «+ Nuevo»' : ''}.</Text>
              </Table.Cell></Table.Row>
            )}
            {rows.map((a) => <ArticleLine key={a.id} a={a} onOpen={onOpen} />)}
          </Table.Body>
        </Table.Root>
      </Box>
      {porLote && <AlertasPorLote onClose={() => setPorLote(false)} />}
      {duplicates && (
        <DuplicatesSheet onClose={() => setDuplicates(false)} onMerged={() => { void list.refetch(); }} />
      )}
      {creating && (
        <NewArticleSheet word={word} initialName={fromNotice ?? ''}
          onClose={() => { setCreating(false); if (fromNotice !== null) clearNotice(); }}
          onCreated={(id) => {
            setCreating(false);
            if (noticeID) void notificationsApi.resolve(noticeID, 'created', id).catch(() => { /* el aviso se cierra a mano desde la campana */ });
            if (fromNotice !== null) clearNotice();
            onOpen(id);
          }} />
      )}
    </Stack>
  );
}

function ArticleLine({ a, onOpen }: { a: ArticleRow; onOpen: (id: number) => void }) {
  const negative = Number(a.onHand) < 0;
  const cost = formatUnitCost(a.currentCost, a.baseUnitCode);
  const packs = a.presentations ?? [];
  return (
    <Table.Row role="button" tabIndex={0} aria-label={a.name} cursor="pointer" h="44px" css={{ "& > td": { paddingBlock: "0" } }}
      onClick={() => onOpen(a.id)} onKeyDown={(e) => { if (e.key === 'Enter') onOpen(a.id); }}>
      <Table.Cell>
        {/* Una sola línea: con «por revisar» y la etiqueta en un mismo texto, la fila se partía en dos
            mientras se buscaba (revisión de tableta). */}
        <HStack gap={2} minW={0}>
          <Text fontWeight="600" truncate minW={0}>{a.name}</Text>
          {a.compositionStatus === 'estimated' && <Badge colorPalette="orange" flexShrink={0}>por revisar</Badge>}
          {a.matchedAlt && <AltMatchTag name={a.matchedAlt} maxW="40%" />}
        </HStack>
      </Table.Cell>
      <Table.Cell textAlign="end" whiteSpace="nowrap">
        <Text as="span" color={negative ? 'fg.error' : undefined} fontWeight={negative ? '700' : undefined}>
          {formatBaseQty(a.onHand, a.baseUnitCode)}
        </Text>
        {/* En el mismo renglón: uno aparte subiría la fila de 44 px y la lista dejaría de mostrar 6. */}
        {negative && (
          <Text as="span" ml={2} px={2} py="1px" borderRadius="md" fontWeight="700" color="fg.error" bg="bg.error">Revisar</Text>
        )}
      </Table.Cell>
      <Table.Cell><Text color="fg.muted">{a.countPlace ?? NO_LOCATION}</Text></Table.Cell>
      <Table.Cell>{cost ?? <Text as="span" fontSize="sm" color="fg.error" fontWeight="600">Sin costo</Text>}</Table.Cell>
      <Table.Cell>
        <Text fontSize="sm" color="fg.muted">
          {packs.length ? packs.map((p) => `${presentationLabel(p)} · ${formatBaseQty(p.qtyInBase, a.baseUnitCode)}`).join(' · ') : 'Sin empaques'}
        </Text>
      </Table.Cell>
    </Table.Row>
  );
}

