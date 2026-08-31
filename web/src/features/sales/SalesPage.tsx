import { useState } from 'react';
import { Box, Button, HStack, Table, Text, VStack, Wrap } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';

import { salesApi, type SalesPreset, type SalesSort, type SaleRow } from '../../api/sales';
import { Page } from '../../components/Page';
import { SortHead } from '../../components/SortHead';
import { money } from '../../utils/format';
import { SaleDetailDialog } from './SaleDetailDialog';
import { SalesSummaryTiles } from './SalesSummaryTiles';
import { etiquetaEstado, etiquetaTipo } from './etiquetas';

const PRESETS: Array<{ id: SalesPreset; label: string }> = [
  { id: 'hoy', label: 'Hoy' },
  { id: 'ayer', label: 'Ayer' },
  { id: 'semana', label: 'Semana' },
  { id: 'mes', label: 'Mes' },
];

const ESTADOS = ['', 'abierta', 'lista', 'entregada', 'cancelada', 'reembolsada'];
const TIPOS = ['', 'mostrador', 'para_llevar', 'domicilio'];

const PAGE_SIZE = 20;

// Pantalla de Ventas: qué se vendió, cuánto entró y por qué medio.
//
// Es distinta del tablero de pedidos. Aquí no se opera un turno, se mira lo que ya pasó: por eso no
// hay acciones de dinero —cancelar, reembolsar— sobre la tabla. Meterlas aquí duplicaría el permiso
// y el rastro que ya viven en el tablero, y un tap equivocado en una tabla densa cuesta caro.
export function SalesPage() {
  const [preset, setPreset] = useState<SalesPreset>('hoy');
  const [status, setStatus] = useState('');
  const [serviceType, setServiceType] = useState('');
  const [sort, setSort] = useState<SalesSort>('fecha');
  const [dir, setDir] = useState<'asc' | 'desc'>('desc');
  const [page, setPage] = useState(0);
  const [detalle, setDetalle] = useState<SaleRow | null>(null);

  const filtros = { preset, status, serviceType };

  const lista = useQuery({
    queryKey: ['sales', 'list', filtros, sort, dir, page],
    queryFn: () => salesApi.list({ ...filtros, sort, dir, page, pageSize: PAGE_SIZE }),
    placeholderData: (previa) => previa,
  });
  // La llave del resumen NO lleva página ni orden: no cambian con ellos, y meterlos haría que se
  // vuelva a pedir en cada tap del paginador.
  const resumen = useQuery({
    queryKey: ['sales', 'summary', { preset, serviceType }],
    queryFn: () => salesApi.summary({ preset, serviceType }),
    placeholderData: (previa) => previa,
  });

  const cambiar = <T,>(set: (v: T) => void) => (v: T) => { set(v); setPage(0); };
  const ordenar = (col: SalesSort) => {
    if (col === sort) setDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    else { setSort(col); setDir(col === 'fecha' || col === 'total' ? 'desc' : 'asc'); }
    setPage(0);
  };

  const items = lista.data?.items ?? [];
  const total = lista.data?.total ?? 0;
  const paginas = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const rango = lista.data?.range ?? resumen.data?.range;

  return (
    <Page fill maxW="1280px">
      <HStack justify="space-between" align="baseline" mb={1} flexWrap="wrap">
        <Text fontSize="2xl" fontWeight="800">Ventas</Text>
        {/* El rango va a la vista: es lo que evita leer una cifra sin saber de qué periodo es. */}
        {rango && (
          <Text fontSize="sm" color="fg.muted">
            {rango.from === rango.to ? rango.from : `${rango.from} al ${rango.to}`}
          </Text>
        )}
      </HStack>

      <Wrap gap={2} mb={3}>
        {PRESETS.map((p) => (
          <Button key={p.id} size="sm" minH="40px" px={4}
            variant={preset === p.id ? 'solid' : 'outline'}
            colorPalette={preset === p.id ? undefined : 'gray'}
            onClick={() => cambiar(setPreset)(p.id)}>
            {p.label}
          </Button>
        ))}
      </Wrap>

      <SalesSummaryTiles resumen={resumen.data} cargando={resumen.isLoading} />

      <HStack gap={2} my={3} flexWrap="wrap">
        <Box minW="170px">
          <select value={status} onChange={(e) => cambiar(setStatus)(e.target.value)}
            style={{ width: '100%', minHeight: '44px', padding: '0 10px', borderRadius: 8, borderWidth: 1 }}>
            {ESTADOS.map((s) => <option key={s} value={s}>{s === '' ? 'Todos los estados' : etiquetaEstado(s)}</option>)}
          </select>
        </Box>
        <Box minW="170px">
          <select value={serviceType} onChange={(e) => cambiar(setServiceType)(e.target.value)}
            style={{ width: '100%', minHeight: '44px', padding: '0 10px', borderRadius: 8, borderWidth: 1 }}>
            {TIPOS.map((s) => <option key={s} value={s}>{s === '' ? 'Todos los tipos' : etiquetaTipo(s)}</option>)}
          </select>
        </Box>
      </HStack>

      <Box flex="1" minH={0} overflowY="auto" borderWidth="1px" borderRadius="lg">
        <Table.Root size="sm" stickyHeader interactive>
          <Table.Header>
            <Table.Row>
              <SortHead label="Folio" col={'folio' as SalesSort} sort={sort} dir={dir} onSort={ordenar} />
              <SortHead label="Hora" col={'fecha' as SalesSort} sort={sort} dir={dir} onSort={ordenar} />
              <SortHead label="Estado" col={'estado' as SalesSort} sort={sort} dir={dir} onSort={ordenar} />
              <SortHead label="Tipo" col={'tipo' as SalesSort} sort={sort} dir={dir} onSort={ordenar} />
              <Table.ColumnHeader>Cliente</Table.ColumnHeader>
              <Table.ColumnHeader>Medio de pago</Table.ColumnHeader>
              <SortHead label="Total" col={'total' as SalesSort} sort={sort} dir={dir} onSort={ordenar} numeric align="end" />
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {items.map((v) => (
              <Table.Row key={v.id} cursor="pointer" onClick={() => setDetalle(v)}>
                <Table.Cell fontWeight="700">#{v.dailyNumber}</Table.Cell>
                <Table.Cell whiteSpace="nowrap">{hora(v.openedAt)}</Table.Cell>
                <Table.Cell>{etiquetaEstado(v.status)}</Table.Cell>
                <Table.Cell>{v.platform || etiquetaTipo(v.serviceType)}</Table.Cell>
                <Table.Cell color="fg.muted">{v.customer || '—'}</Table.Cell>
                <Table.Cell color="fg.muted">{v.methods || 'Sin cobrar'}</Table.Cell>
                <Table.Cell textAlign="end" fontWeight="700">{money(v.total)}</Table.Cell>
              </Table.Row>
            ))}
            {items.length === 0 && !lista.isLoading && (
              <Table.Row>
                <Table.Cell colSpan={7}>
                  <VStack py={8} gap={1}>
                    <Text color="fg.muted">Sin ventas en este periodo</Text>
                  </VStack>
                </Table.Cell>
              </Table.Row>
            )}
          </Table.Body>
        </Table.Root>
      </Box>

      <HStack justify="space-between" mt={3}>
        <Text fontSize="sm" color="fg.muted">{total} {total === 1 ? 'venta' : 'ventas'}</Text>
        <HStack gap={2}>
          <Button size="sm" minH="40px" variant="outline" disabled={page === 0}
            onClick={() => setPage((p) => Math.max(0, p - 1))}>Anterior</Button>
          <Text fontSize="sm">{page + 1} / {paginas}</Text>
          <Button size="sm" minH="40px" variant="outline" disabled={page + 1 >= paginas}
            onClick={() => setPage((p) => p + 1)}>Siguiente</Button>
        </HStack>
      </HStack>

      {detalle && (
        <SaleDetailDialog venta={detalle} isOpen onClose={() => setDetalle(null)} />
      )}
    </Page>
  );
}

// Solo la hora: la fecha ya la dice el rango de arriba, y repetirla en cada renglón gasta el ancho
// que en una tablet de 7 pulgadas hace falta para el medio de pago.
function hora(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString('es-MX', { hour: '2-digit', minute: '2-digit' });
}
