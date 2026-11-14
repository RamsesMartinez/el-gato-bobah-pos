import { Box, Text, Button, VStack, HStack, Table, Input, SimpleGrid } from '@chakra-ui/react';
import { type MethodTotal, type ConteosDelTurno, type ArqueoDelCajon } from '../../api/backoffice';
import type { ResultadoDelConteo } from './conteo';
import { money } from '../../utils/format';
import { Diferencia } from './diferencia';
import { montoOSinDato, montoDeCaja } from './montosDeCaja';
import { diferenciaDelCajon, type DiferenciasDelCierre, etiquetaDeclarado, type FaltanteDelCierre } from './cierreDeCaja';
import { round2 } from '../../domain/numeros';
import { Plegable } from './reporte';

// ArqueoDelCorte: lo que el cajón debía tener, lo que se contó y la diferencia — una sola.
//
// Sin esto, un corte cerrado con faltante se ve CUADRADO: desde la spec 015 los métodos que
// comparten el cajón guardan su declarado igual a su esperado, así que la tabla por método reporta
// cero en todos y el faltante vive solo aquí. Es la pantalla donde alguien audita.
//
// Nil = corte anterior a la 015, o una caja que no maneja efectivo. No se pinta nada y no se
// inventa un cero.
export function ArqueoDelCorte({ drawer, currency }: {
  drawer?: ArqueoDelCajon | null; currency: string;
}) {
  if (!drawer || drawer.counted === null) return null;
  const dif = drawer.difference;
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} bg="bg.panel">
      <Text fontWeight="700" mb={2}>Arqueo del cajón</Text>
      <SimpleGrid columns={3} gap={2} fontSize="sm">
        <Text color="fg.muted">Esperado</Text>
        <Text color="fg.muted">Contado</Text>
        <Text color="fg.muted">Diferencia</Text>
        <Text aria-label="Esperado del cajón">{montoOSinDato(drawer.expected, currency)}</Text>
        <Text aria-label="Contado del cajón" fontWeight="600">{montoDeCaja(drawer.counted, currency)}</Text>
        <Diferencia valor={dif} currency={currency} etiqueta="Diferencia del arqueo del corte" />
      </SimpleGrid>
    </Box>
  );
}

// DesgloseDelConteo: cuántas piezas de cada denominación se declararon, o por qué no se contó.
//
// Es la razón de guardar el desglose (US3): un corte con faltante y sin él es un número sin
// historia, y no hay forma de distinguir "faltan dos billetes de $500" de "falta dinero". Eso costó
// un turno con $1,662 que nadie pudo explicar.
//
// UN CORTE ANTERIOR A ESTA FUNCIONALIDAD NO PINTA NADA, sin avisos ni explicaciones: son todos los
// que ya existen, y decirle al operador que ese corte "no tiene desglose" es contarle una historia
// del sistema que él no puede accionar.
export function DesgloseDelConteo({ counts, currency }: {
  counts?: ConteosDelTurno | null; currency: string;
}) {
  const momentos = ([['apertura', 'Al abrir'], ['cierre', 'Al cerrar']] as const)
    .map(([k, titulo]) => ({ titulo, conteo: counts?.[k] ?? null }))
    .filter((m) => m.conteo !== null);
  if (momentos.length === 0) return null;
  return (
    <Plegable title="Efectivo contado">
      <VStack align="stretch" gap={4}>
        {momentos.map(({ titulo, conteo }) => (
          <Box key={titulo}>
            <HStack justify="space-between" mb={1}>
              <Text fontWeight="700" fontSize="sm">{titulo}</Text>
              <Text fontWeight="700">{montoDeCaja(conteo!.total, currency)}</Text>
            </HStack>
            {conteo!.manualReason !== null ? (
              // Sin piezas: lo que hay es el porqué, y es lo que vuelve auditable este arqueo.
              <Text fontSize="sm" color="fg.muted">Capturado a mano: {conteo!.manualReason}</Text>
            ) : (
              <Table.Root size="sm">
                <Table.Header><Table.Row>
                  <Table.ColumnHeader>Denominación</Table.ColumnHeader>
                  <Table.ColumnHeader textAlign="end">Piezas</Table.ColumnHeader>
                  <Table.ColumnHeader textAlign="end">Subtotal</Table.ColumnHeader>
                </Table.Row></Table.Header>
                <Table.Body>
                  {conteo!.lines.map((l) => (
                    <Table.Row key={l.value}>
                      <Table.Cell>{etiquetaDeDenominacion(l.value, currency)}</Table.Cell>
                      <Table.Cell textAlign="end">{l.pieces}</Table.Cell>
                      {/* Del servidor: quien lee esto está comparando contra su cajón. */}
                      <Table.Cell textAlign="end">{montoDeCaja(l.subtotal, currency)}</Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table.Root>
            )}
          </Box>
        ))}
      </VStack>
    </Plegable>
  );
}

// Las piezas de menos de un peso se nombran en centavos: "$0.5" no es como se llama esa moneda.
function etiquetaDeDenominacion(value: string, currency: string): string {
  const n = Number(value);
  return n < 1 ? `${Math.round(n * 100)}¢` : money(n, currency);
}

// AvisoDeFaltantes dice qué falta capturar para cerrar y lleva a cada campo con un toque. Era una
// lista de texto: el equipo no supo dónde capturar y leyó otra cosa como la causa (#251).
export function AvisoDeFaltantes({ faltantes, onContar }: { faltantes: FaltanteDelCierre[]; onContar: () => void }) {
  if (faltantes.length === 0) return null;
  const ir = (campo: string) => {
    if (campo === 'cajon') return onContar();
    const el = document.querySelector<HTMLElement>(`[aria-label="${CSS.escape(campo)}"]`);
    el?.scrollIntoView({ block: 'center', behavior: 'smooth' });
    el?.focus({ preventScroll: true });
  };
  return (
    <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle">
      <Text fontSize="sm" fontWeight="600" mb={2}>Para cerrar falta capturar lo contado en:</Text>
      <HStack gap={2} flexWrap="wrap">
        {/* Con el índice en la llave: dos terminales pueden llamarse igual en un turno (una archivada y
            su reemplazo, las dos con cobros). */}
        {faltantes.map((f, i) => (
          <Button key={`${f.campo}-${i}`} size="sm" minH="44px" variant="outline" colorPalette="orange"
            aria-label={`Capturar ${f.texto}`} onClick={() => ir(f.campo)}>
            {f.texto}
          </Button>
        ))}
      </HStack>
    </Box>
  );
}

// TablaDelCierre: qué se espera, qué se declaró y CUÁNTO FALTA (spec 015).
//
// EL CAJÓN ES UN RENGLÓN, NO CUATRO. Los métodos cuyo dinero cae en el mismo montón de billetes
// —el efectivo del mostrador y los de plataforma en efectivo— se siguen listando porque el corte
// informa por canal, pero sin campo y sin diferencia propia: un turno real quedó con «Efectivo» en
// $0.00 y «Didi efectivo» en −$64.80, dos diferencias del mismo dinero, y nadie podía saber cuál
// era el faltante real.
//
// El tercer estado de la columna «Declarado» dice **«Va al cajón»** y NO reusa «Automático»: un
// método auto-declarado lo resuelve el servidor, y uno de cajón se cuenta físicamente. Confundirlos
// es la clase de ambigüedad que ya costó $4,500.
export function TablaDelCierre({ totals, currency, declared, onDeclared, cajon, conteo, onContar, diferencias }: {
  totals: MethodTotal[];
  currency: string;
  declared: Record<string, string>;
  onDeclared: (d: Record<string, string>) => void;
  cajon: ArqueoDelCajon | null;
  conteo: ResultadoDelConteo | null;
  onContar: () => void;
  diferencias: DiferenciasDelCierre;
}) {
  const delCajon = new Set(cajon?.methodIds ?? []);
  const difCajon = diferenciaDelCajon(cajon, conteo);
  // ARQUEO CIEGO: si el servidor no mandó ningún esperado, la columna entera sobra. No es solo que
  // no haya cifras — a 1024×600 el ancho que libera se lo devuelve a lo que el operador vino a
  // leer, y una columna de rayas invita a preguntarse qué se rompió.
  const seVeElEsperado = totals.some((t) => t.expected !== null) || (cajon?.expected ?? null) !== null;
  return (
    <Box>
      <Text fontWeight="700" mb={2}>Cierre — contado por método</Text>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflow="hidden">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Método</Table.ColumnHeader>
            {seVeElEsperado && <Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader>}
            <Table.ColumnHeader>Contado</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Diferencia</Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {/* EL CAJÓN, PRIMERO Y UNA SOLA VEZ: es lo que el operador tiene enfrente. */}
            {cajon && (
              <Table.Row bg="bg.subtle">
                <Table.Cell fontWeight="700">Cajón</Table.Cell>
                {seVeElEsperado && (
                  <Table.Cell textAlign="end" fontWeight="700">{montoOSinDato(cajon.expected, currency)}</Table.Cell>
                )}
                <Table.Cell>
                  <Button size="sm" minH="44px" variant={conteo ? 'outline' : 'solid'} onClick={onContar}>
                    {conteo ? montoDeCaja(conteo.total, currency) : 'Contar efectivo'}
                  </Button>
                </Table.Cell>
                <Table.Cell>
                  <Diferencia valor={difCajon === undefined ? null : difCajon} currency={currency} etiqueta="Diferencia del cajón" />
                </Table.Cell>
              </Table.Row>
            )}
            {totals.map((t) => {
              const dif = diferencias.porMetodo[t.methodId];
              const enElCajon = delCajon.has(t.methodId);
              return (
                <Table.Row key={t.methodId}>
                  <Table.Cell color={enElCajon ? 'fg.muted' : undefined}>{t.name}</Table.Cell>
                  {seVeElEsperado && (
                    <Table.Cell textAlign="end">{montoOSinDato(t.expected, currency)}</Table.Cell>
                  )}
                  <Table.Cell>
                    {enElCajon ? (
                      // Ni campo ni botón: su dinero ya se contó arriba, con el cajón entero.
                      <Text fontSize="sm" color="fg.muted">Va al cajón</Text>
                    ) : t.autoDeclare ? (
                      <Text fontSize="sm" color="fg.muted">Automático</Text>
                    ) : (
                      <Input size="sm" minH="44px" w="130px" inputMode="decimal" placeholder="0"
                        aria-label={etiquetaDeclarado(t.name)}
                        value={declared[t.methodId] ?? ''}
                        onChange={(e) => onDeclared({ ...declared, [t.methodId]: e.target.value })} />
                    )}
                  </Table.Cell>
                  <Table.Cell>
                    <Diferencia valor={dif === undefined ? null : dif} currency={currency} etiqueta={`Diferencia de ${t.name}`} />
                  </Table.Cell>
                </Table.Row>
              );
            })}
          </Table.Body>
        </Table.Root>
      </Box>
    </Box>
  );
}

// DiferenciaDelCierre: lo que el arqueo va a reportar, ANTES de confirmarlo.
//
// Va junto al botón de cerrar y con el mismo peso visual que el aviso de "falta por contar": las dos
// cosas deciden si el operador debe tocar el botón o volver a contar.
export function DiferenciaDelCierre({ diferencias, cajon, conteo, currency }: {
  diferencias: DiferenciasDelCierre;
  cajon: ArqueoDelCajon | null;
  conteo: ResultadoDelConteo | null;
  currency: string;
}) {
  // Mientras falte capturar algo, la cifra sería parcial. Un número que se lee como el resultado del
  // arqueo sin serlo manda a buscar dinero que sí está: el aviso de lo que falta ya está arriba.
  if (!diferencias.completo) return null;
  const difCajon = diferenciaDelCajon(cajon, conteo);
  // Con el cajón sin contar —o con el arqueo ciego, donde el esperado no viaja— no hay total que
  // mostrar. Sumar cero por el cajón diría que cuadra, y eso nadie lo comprobó.
  if (cajon !== null && difCajon === undefined) return null;
  const total = round2(diferencias.total + (difCajon ?? 0));
  // El borde marca solo el faltante: un sobrante dentro de un recuadro de error se leía como falla
  // (#290, hallazgo 20).
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} bg="bg.panel"
      borderColor={total < -0.005 ? 'fg.shortage' : 'border'}>
      <HStack justify="space-between" flexWrap="wrap" gap={2}>
        <Text fontWeight="700">Resultado del arqueo</Text>
        <Diferencia valor={total} currency={currency} grande etiqueta="Diferencia del arqueo" />
      </HStack>
    </Box>
  );
}

