import { useState } from 'react';
import {
  Box, Heading, SimpleGrid, Table, Center, Spinner, Text, Stat, HStack,
} from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { backofficeApi, type ReportPreset } from '../../api/backoffice';
import { money } from '../../utils/format';
import { Page } from '../../components/Page';
import { RangoDeFechas } from '../../components/RangoDeFechas';
import { validarRango } from '../../domain/rangoDeFechas';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';

const PRESETS = [
  { id: '30d', label: '30 días' },
  { id: 'semana', label: 'Semana' },
  { id: 'mes', label: 'Mes' },
];

export function ReportsPage() {
  const horaNegocio = useHoraDelNegocio();
  const [preset, setPreset] = useState<ReportPreset>('30d');
  const [desde, setDesde] = useState('');
  const [hasta, setHasta] = useState('');

  // Las fechas viajan SOLO con el rango libre: el servidor rechaza un `from` que el preset no va a
  // usar, porque aceptarlo en silencio es contestar un periodo que nadie pidió.
  const esRango = preset === 'rango';
  const rangoInvalido = esRango ? validarRango(desde, hasta) : null;
  const periodo = { preset, ...(esRango ? { from: desde, to: hasta } : {}) };
  const puedeConsultar = rangoInvalido === null;

  // Las tres consultas comparten la MISMA llave de periodo. Es lo que impide que la pantalla acabe
  // mezclando dos rangos: si una se quedara con el suyo, sus cifras seguirían pintadas junto a las
  // de la otra sin nada que lo delate.
  const sales = useQuery({
    queryKey: ['report', 'sales', periodo],
    queryFn: () => backofficeApi.reportSales(periodo),
    enabled: puedeConsultar,
  });
  const margins = useQuery({
    queryKey: ['report', 'margins', periodo],
    queryFn: () => backofficeApi.reportMargins(periodo),
    enabled: puedeConsultar,
  });
  const tips = useQuery({
    queryKey: ['report', 'tips', periodo],
    queryFn: () => backofficeApi.reportTips(periodo),
    enabled: puedeConsultar,
  });

  const rango = sales.data?.range;

  const totalRevenue = sales.data?.byDay.reduce((s, d) => s + Number(d.revenue), 0) ?? 0;
  const totalOrders = sales.data?.byDay.reduce((s, d) => s + d.orders, 0) ?? 0;
  const totalTips = tips.data?.byEmployee.reduce((s, e) => s + Number(e.tips), 0) ?? 0;

  return (
    <Page maxW="1150px">
      <HStack justify="space-between" align="baseline" mb={2} flexWrap="wrap">
        <Heading size="lg">Reportes</Heading>
        {/* El periodo que el SERVIDOR consultó, no el que la pantalla cree haber pedido. El
            encabezado decía "últimos 30 días" fijo, y lo seguía diciendo con cualquier rango. */}
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
          onPreset={(id) => setPreset(id as ReportPreset)}
          desde={desde}
          hasta={hasta}
          onRango={(d, h) => { setDesde(d); setHasta(h); }}
          hoy={horaNegocio.diaDelNegocio(new Date())}
        />
      </Box>

      {sales.isLoading && <Center py={10}><Spinner size="xl" /></Center>}

      <HStack mb={4} flexWrap="wrap">
        <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px"><Stat.Label>Ventas</Stat.Label><Stat.ValueText>{money(totalRevenue)}</Stat.ValueText></Stat.Root>
        <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px"><Stat.Label>Pedidos</Stat.Label><Stat.ValueText>{totalOrders}</Stat.ValueText></Stat.Root>
        <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px"><Stat.Label>Propinas</Stat.Label><Stat.ValueText>{money(totalTips)}</Stat.ValueText></Stat.Root>
      </HStack>

      <SimpleGrid columns={{ base: 1, lg: 2 }} gap={4}>
        <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4}>
          <Text fontWeight="700" mb={2}>Por medio de pago</Text>
          <Table.Root size="sm">
            <Table.Header><Table.Row><Table.ColumnHeader>Método</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Pagos</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Total</Table.ColumnHeader></Table.Row></Table.Header>
            <Table.Body>
              {sales.data?.byMethod.map((m) => (
                <Table.Row key={m.method}><Table.Cell>{m.method}</Table.Cell><Table.Cell textAlign="end">{m.payments}</Table.Cell><Table.Cell textAlign="end">{money(m.total)}</Table.Cell></Table.Row>
              ))}
            </Table.Body>
          </Table.Root>
        </Box>

        <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4} overflowX="auto">
          <Text fontWeight="700" mb={2}>Utilidad por producto</Text>
          <Table.Root size="sm">
            <Table.Header><Table.Row><Table.ColumnHeader>Producto</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Cant.</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Venta</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Margen</Table.ColumnHeader></Table.Row></Table.Header>
            <Table.Body>
              {margins.data?.items.slice(0, 20).map((m) => (
                <Table.Row key={m.product_name}>
                  <Table.Cell>{m.product_name}</Table.Cell>
                  <Table.Cell textAlign="end">{m.qty}</Table.Cell>
                  <Table.Cell textAlign="end">{money(m.revenue)}</Table.Cell>
                  <Table.Cell textAlign="end" color={Number(m.margin) < 0 ? 'red.600' : 'green.600'}>{money(m.margin)}</Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table.Root>
        </Box>

        {/* Propinas (pass-through): para repartir entre el personal. */}
        <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4} overflowX="auto">
          <Text fontWeight="700" mb={2}>Propinas por empleado</Text>
          {(tips.data?.byEmployee.length ?? 0) === 0 ? (
            <Text fontSize="sm" color="fg.muted">Sin propinas en el periodo.</Text>
          ) : (
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Empleado</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Cobros</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Propina</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {tips.data?.byEmployee.map((e) => (
                  <Table.Row key={e.employee}>
                    <Table.Cell>{e.employee}</Table.Cell>
                    <Table.Cell textAlign="end">{e.payments}</Table.Cell>
                    <Table.Cell textAlign="end" fontWeight="600">{money(e.tips)}</Table.Cell>
                  </Table.Row>
                ))}
                <Table.Row fontWeight="700">
                  <Table.Cell colSpan={2}>Total</Table.Cell>
                  <Table.Cell textAlign="end">{money(totalTips)}</Table.Cell>
                </Table.Row>
              </Table.Body>
            </Table.Root>
          )}
        </Box>

        <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4} overflowX="auto">
          <Text fontWeight="700" mb={2}>Propinas por día</Text>
          {(tips.data?.byDay.length ?? 0) === 0 ? (
            <Text fontSize="sm" color="fg.muted">Sin propinas en el periodo.</Text>
          ) : (
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Día</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Propina</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {tips.data?.byDay.map((d) => (
                  <Table.Row key={d.business_date}>
                    <Table.Cell>{d.business_date}</Table.Cell>
                    <Table.Cell textAlign="end">{money(d.tips)}</Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Root>
          )}
        </Box>
      </SimpleGrid>
    </Page>
  );
}
