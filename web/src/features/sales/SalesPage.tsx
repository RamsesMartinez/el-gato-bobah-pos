import { useState } from 'react';
import { Box, Button, HStack, Input, Table, Text, VStack } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';

import { salesApi, type SalesPreset, type SalesSort, type SaleRow } from '../../api/sales';
import { settlementsApi } from '../../api/settlements';
import { Page } from '../../components/Page';
import { Picker, type PickerOption } from '../../components/Picker';
import { RangoDeFechas } from '../../components/RangoDeFechas';
import { validarRango } from '../../domain/rangoDeFechas';
import { SortHead } from '../../components/SortHead';
import { money } from '../../utils/format';
import { SaleDetailDialog } from './SaleDetailDialog';
import { SalesSummaryTiles } from './SalesSummaryTiles';
import { etiquetaEstado, etiquetaTipo } from './etiquetas';
import { soloHora } from '../../utils/horaDelNegocio';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';

const PRESETS = [
  { id: 'hoy', label: 'Hoy' },
  { id: 'ayer', label: 'Ayer' },
  { id: 'semana', label: 'Semana' },
  { id: 'mes', label: 'Mes' },
];

const OPCIONES_ESTADO: PickerOption[] = ['abierta', 'lista', 'entregada', 'cancelada', 'reembolsada']
  .map((s) => ({ value: s, label: etiquetaEstado(s) }));
const OPCIONES_TIPO: PickerOption[] = ['mostrador', 'para_llevar', 'domicilio']
  .map((s) => ({ value: s, label: etiquetaTipo(s) }));

const PAGE_SIZE = 20;

// Pantalla de Ventas: qué se vendió, cuánto entró y por qué medio.
//
// Es distinta del tablero de pedidos. Aquí no se opera un turno, se mira lo que ya pasó: por eso no
// hay acciones de dinero —cancelar, reembolsar— sobre la tabla. Meterlas aquí duplicaría el permiso
// y el rastro que ya viven en el tablero, y un tap equivocado en una tabla densa cuesta caro.
export function SalesPage() {
  const horaNegocio = useHoraDelNegocio();
  const [preset, setPreset] = useState<SalesPreset>('hoy');
  const [desde, setDesde] = useState('');
  const [hasta, setHasta] = useState('');
  const [status, setStatus] = useState('');
  const [serviceType, setServiceType] = useState('');
  const [sort, setSort] = useState<SalesSort>('fecha');
  const [dir, setDir] = useState<'asc' | 'desc'>('desc');
  const [page, setPage] = useState(0);
  // Dos controles nuevos y separados: buscar UN pedido por su folio, y quedarse con los que no lo
  // tienen. NO se combinan: buscar contesta ESE pedido, así que cualquier otro filtro se tiraría y
  // la pantalla se vería filtrada contestando algo que no cumple el filtro. El servidor lo rechaza
  // con un 400, y aquí se apaga el otro control para que el operador no llegue a ese rechazo.
  const [folio, setFolio] = useState('');
  const [soloPendientes, setSoloPendientes] = useState(false);
  const [detalle, setDetalle] = useState<SaleRow | null>(null);

  // Las fechas viajan SOLO con el rango libre. El servidor rechaza un `from` que el preset no va a
  // usar, y con razón: aceptarlo en silencio deja pedir "hoy, del 1 al 31 de enero" y contestar hoy
  // con la pantalla viéndose perfecta.
  const esRango = preset === 'rango';
  const hoyDelNegocio = horaNegocio.diaDelNegocio(new Date());
  const rangoInvalido = esRango ? validarRango(desde, hasta, hoyDelNegocio) : null;
  const filtros = {
    preset, status, serviceType,
    ...(esRango ? { from: desde, to: hasta } : {}),
    ...(soloPendientes ? { folioPlataforma: 'pendiente' as const } : {}),
    // Se manda RECORTADO: el servidor rechaza puros espacios, y descubrirlo con un 400 después de
    // teclear es peor que no mandar nada.
    ...(folio.trim() ? { folio: folio.trim() } : {}),
  };
  // Un rango a medias NO se manda: la pantalla conserva el periodo anterior —que es el que su
  // encabezado sigue nombrando— hasta que las dos fechas estén completas.
  const puedeConsultar = rangoInvalido === null;

  const lista = useQuery({
    queryKey: ['sales', 'list', filtros, sort, dir, page],
    queryFn: () => salesApi.list({ ...filtros, sort, dir, page, pageSize: PAGE_SIZE }),
    placeholderData: (previa) => previa,
    enabled: puedeConsultar,
  });
  // La llave del resumen NO lleva página ni orden: no cambian con ellos, y meterlos haría que se
  // vuelva a pedir en cada tap del paginador.
  const resumen = useQuery({
    queryKey: ['sales', 'summary', { preset, serviceType, soloPendientes, folio: folio.trim(),
      desde: esRango ? desde : '', hasta: esRango ? hasta : '' }],
    // El resumen lleva los MISMOS filtros que la lista, sin excepción. Si uno de los dos se queda
    // sin el filtro de pendientes, las cifras de arriba dejan de ser del conjunto de abajo.
    queryFn: () => salesApi.summary(filtros),
    placeholderData: (previa) => previa,
    enabled: puedeConsultar,
  });

  // Las tres cifras de plataformas van en su PROPIA consulta y no dentro del resumen de ventas:
  // mezclarlas pondría la comisión al lado del total de ventas, que es exactamente la invitación a
  // restarlas que esta feature evita.
  // Las cifras de plataformas son del PERIODO COMPLETO: ese endpoint solo usa el rango. Por eso
  // desaparecen en cuanto la tabla se acota con cualquier filtro — si se quedaran, la fila de tiles
  // estaría describiendo un conjunto (todo el mes) y la tabla de abajo otro (lo filtrado), a un
  // borde de 1 px de distancia. Ya costó un turno con $4,500 de faltante sin explicación.
  const tablaAcotada = soloPendientes || folio.trim() !== '' || status !== '' || serviceType !== '';
  const plataformas = useQuery({
    queryKey: ['platform-money', { preset, desde: esRango ? desde : '', hasta: esRango ? hasta : '' }],
    queryFn: () => settlementsApi.summary({ preset, ...(esRango ? { from: desde, to: hasta } : {}) }),
    placeholderData: (previa) => previa,
    enabled: puedeConsultar && !tablaAcotada,
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

      <Box mb={3}>
        <RangoDeFechas
          presets={PRESETS}
          preset={preset}
          onPreset={(id) => cambiar(setPreset)(id as SalesPreset)}
          desde={desde}
          hasta={hasta}
          onRango={(d, h) => { setDesde(d); setHasta(h); setPage(0); }}
          hoy={hoyDelNegocio}
        />
      </Box>

      <SalesSummaryTiles resumen={resumen.data}
        plataformas={tablaAcotada ? undefined : plataformas.data}
        cargando={resumen.isLoading} />

      {/* Pickers táctiles, no <select> nativos: en una tablet de 7" el desplegable del sistema
          tapa la pantalla con renglones de 20px. Ver la constitución. */}
      <HStack gap={2} my={3} flexWrap="wrap">
        <Box flex="1 1 170px" minW="150px" maxW="240px">
          <Picker size="sm" value={status} onChange={cambiar(setStatus)}
            options={OPCIONES_ESTADO} placeholder="Todos los estados"
            title="Filtrar por estado" clearable clearLabel="Todos los estados" />
        </Box>
        <Box flex="1 1 170px" minW="150px" maxW="240px">
          <Picker size="sm" value={serviceType} onChange={cambiar(setServiceType)}
            options={OPCIONES_TIPO} placeholder="Todos los tipos"
            title="Filtrar por tipo de venta" clearable clearLabel="Todos los tipos" />
        </Box>
        {/* Buscar pegando el folio del documento de pago. El rótulo nombra "de la plataforma"
            porque en esta misma pantalla la columna "Folio" es otra cosa: el número del turno. */}
        <Box flex="1 1 200px" minW="170px" maxW="280px">
          <Input size="sm" minH="44px" aria-label="Buscar folio de la plataforma"
            placeholder="Folio de la plataforma"
            value={folio}
            onChange={(e) => { setFolio(e.target.value); if (e.target.value.trim()) setSoloPendientes(false); setPage(0); }}
            autoComplete="off" autoCapitalize="off" spellCheck={false} />
        </Box>
        {/* Un TOGGLE y no un Picker: el valor es booleano, y con un Picker encenderlo y apagarlo
            cuesta cuatro toques contra dos. La vara del POS es minimizar taps. */}
        <Button size="sm" minH="44px" px={4}
          variant={soloPendientes ? 'solid' : 'outline'}
          colorPalette={soloPendientes ? 'orange' : 'gray'}
          aria-pressed={soloPendientes}
          onClick={() => { setSoloPendientes((v) => !v); setFolio(''); setPage(0); }}>
          Pendientes de folio de plataforma
        </Button>
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
                <Table.Cell>
                  <Text fontWeight="700" lineHeight="1.2">{v.folioName || `#${v.dailyNumber}`}</Text>
                  {v.folioName && <Text fontSize="xs" color="fg.muted">#{v.dailyNumber}</Text>}
                </Table.Cell>
                <Table.Cell whiteSpace="nowrap">{hora(v.openedAt, horaNegocio.zona)}</Table.Cell>
                <Table.Cell>{etiquetaEstado(v.status)}</Table.Cell>
                <Table.Cell maxW="150px">
                  <Text lineHeight="1.2">{v.platform || etiquetaTipo(v.serviceType)}</Text>
                  {/* Truncado y en una sola línea: el folio llega hasta 64 caracteres y esta celda
                      es angosta, así que sin elipsis envolvería a dos o tres líneas en casi todos
                      los renglones de la vista filtrada — que es justo su caso de uso. El completo
                      vive en el detalle, que es donde además se corrige. */}
                  {v.platformOrderRef && (
                    <Text fontSize="xs" color="fg.muted" whiteSpace="nowrap"
                      overflow="hidden" textOverflow="ellipsis">
                      {v.platformOrderRef}
                    </Text>
                  )}
                </Table.Cell>
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
          {/* Se apagan mientras el rango está a medias: `paginas` sale del periodo ANTERIOR, así
              que avanzar movía el contador sobre filas que no son del filtro que se está capturando.
              44 px, que es el mínimo con el que un dedo acierta a la primera. */}
          <Button size="sm" minH="44px" px={4} variant="outline" disabled={page === 0 || !puedeConsultar}
            onClick={() => setPage((p) => Math.max(0, p - 1))}>Anterior</Button>
          <Text fontSize="sm">{page + 1} / {paginas}</Text>
          <Button size="sm" minH="44px" px={4} variant="outline" disabled={page + 1 >= paginas || !puedeConsultar}
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
// La zona llega como parámetro: esta es una función de módulo.
function hora(iso: string, zona: string): string {
  return soloHora(iso, zona);
}
