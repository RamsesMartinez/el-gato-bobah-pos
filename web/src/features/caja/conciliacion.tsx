import { Box, Table } from '@chakra-ui/react';
import { type MethodTotal, type ArqueoDelCajon, type CutDiff } from '../../api/backoffice';
import { Diferencia } from './diferencia';
import { montoOSinDato, montoDeCaja } from './montosDeCaja';
import { round2 } from '../../domain/numeros';

// Conciliación por método: Esperado · Contado · Diferencia (#290, FR-003). Los métodos del cajón
// no tienen diferencia propia cuando hay arqueo: su dinero se contó junto, y desde la spec 015 su
// renglón guarda contado = esperado. Se pintan con «ver arqueo» y no entran a la fila Total, que
// suma solo lo que se concilia aquí — sumar el cajón en una columna y no en las otras hacía que la
// fila Total dijera «sistema = declarado, dif. $X» (#290).
export function TotalsTable({ totals, currency, withTotalRow, drawerMethodIds, difference, drawer }: {
  totals: MethodTotal[]; currency: string; withTotalRow?: boolean;
  drawerMethodIds?: number[]; difference?: CutDiff | null;
  // Respaldo para un servidor anterior que no manda `difference`: la misma regla que
  // domain.CutDifference (hay arqueo con esperado y contado ⇒ el cajón se lee en el arqueo).
  drawer?: ArqueoDelCajon | null;
}) {
  if (!totals?.length) return null;
  const hayArqueo = difference ? difference.drawerCovered : (drawer?.expected != null && drawer?.counted != null);
  const cubierto = new Set(hayArqueo ? (drawerMethodIds ?? drawer?.methodIds ?? []) : []);
  const conciliados = totals.filter((t) => !cubierto.has(t.methodId));
  const sum = (pick: (t: MethodTotal) => string) => round2(conciliados.reduce((s, t) => s + (Number(pick(t)) || 0), 0));
  // Un esperado que no viajó (arqueo ciego) no se suma como cero: la fila diría un parcial.
  const esperadoTotal = conciliados.some((t) => t.expected === null) ? null : String(sum((t) => t.expected ?? '0'));
  const difTotal = difference ? difference.otherMethods : String(sum((t) => t.difference));
  return (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm">
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Método</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Contado</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Diferencia</Table.ColumnHeader>
        </Table.Row></Table.Header>
        <Table.Body>
          {totals.map((t) => (
            <Table.Row key={t.methodId}>
              <Table.Cell>{t.name}</Table.Cell>
              <Table.Cell textAlign="end">{montoOSinDato(t.expected, currency)}</Table.Cell>
              {cubierto.has(t.methodId) ? (<>
                <Table.Cell textAlign="end" color="fg.muted">—</Table.Cell>
                {/* Texto, no acción: el arqueo está en la sección de abajo. */}
                <Table.Cell textAlign="end" color="fg.muted" fontStyle="italic">ver arqueo</Table.Cell>
              </>) : (<>
                <Table.Cell textAlign="end">{montoDeCaja(t.declared, currency)}</Table.Cell>
                <Table.Cell><Diferencia valor={t.difference} currency={currency} /></Table.Cell>
              </>)}
            </Table.Row>
          ))}
          {withTotalRow && (
            <Table.Row fontWeight="700" bg="bg.muted">
              {/* Con el cajón aparte, esta fila NO es el Total del corte: se nombra distinto para
                  que dos cifras distintas no se llamen igual en la misma pantalla (#290). */}
              <Table.Cell>{cubierto.size > 0 ? 'Total sin el cajón' : 'Total'}</Table.Cell>
              <Table.Cell textAlign="end">{montoOSinDato(esperadoTotal, currency)}</Table.Cell>
              <Table.Cell textAlign="end">{montoDeCaja(sum((t) => t.declared), currency)}</Table.Cell>
              <Table.Cell><Diferencia valor={difTotal} currency={currency} etiqueta="Diferencia de la conciliación" /></Table.Cell>
            </Table.Row>
          )}
        </Table.Body>
      </Table.Root>
    </Box>
  );
}
