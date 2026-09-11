import { useState } from 'react';
import { Box, Button, Grid, HStack, Input, Tabs, Text, VStack } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { backofficeApi, type Denomination } from '../../api/backoffice';
import { montoTecleado, round2 } from '../../domain/numeros';
import { money } from '../../utils/format';
import {
  armarConteo, leerPiezas, type PiezasTecleadas, type ResultadoDelConteo,
} from './conteo';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  titulo: string;
  currency: string;
  etiquetaConfirmar: string;
  guardando: boolean;
  onConfirmar: (r: ResultadoDelConteo) => void;
}

type Camino = 'contar' | 'total';

// Lo que espera confirmación, con el texto que lo explica en pantalla.
type Pendiente = { que: 'camino'; a: Camino } | { que: 'cerrar' };

// La hoja donde se cuenta el cajón.
//
// ES UNA HOJA PROPIA Y NO UN BLOQUE EN `/caja`. Medido a 1024×600: esa pantalla mide 1,494 px de
// alto y el lugar donde iría la rejilla arranca en y=796, casi 200 px debajo del fold. Inline,
// FR-013 no se cumple por más compacta que sea la rejilla — no es un problema de la rejilla, es que
// ese lugar de la pantalla ya no existe.
//
// EL CAMPO ES EL CONTROL PRINCIPAL de cada denominación: así se cuenta en la operación diaria —se
// agarra el montón de monedas, se cuenta y se escribe. El `+`/`−` es el ajuste de una pieza, no el
// camino: con tap = +1 como gesto principal, 40 monedas son 40 taps y un cajón típico no cabe en
// los 2 minutos de SC-003.
export function ContadorDeEfectivo({
  isOpen, onClose, titulo, currency, etiquetaConfirmar, guardando, onConfirmar,
}: Props) {
  const { data, isLoading } = useQuery({
    queryKey: ['cash', 'denominations', currency],
    queryFn: () => backofficeApi.cashDenominations(currency),
    enabled: isOpen,
    // El catálogo cambia como operación deliberada de owner (la app no lo escribe), así que no hace
    // falta refrescarlo mientras alguien cuenta.
    staleTime: 60 * 60 * 1000,
  });
  const catalogo = data?.items ?? [];

  const [camino, setCamino] = useState<Camino>('contar');
  const [tecleadas, setTecleadas] = useState<PiezasTecleadas>({});
  const [totalAMano, setTotalAMano] = useState('');
  const [motivo, setMotivo] = useState('');
  // Lo que se pidió y todavía no se confirma, porque las dos cosas BORRAN lo capturado: cambiar de
  // camino (FR-015: los dos no pueden coexistir) y salir de la hoja. Un solo estado para las dos
  // porque la pérdida es la misma —hasta once denominaciones recontadas— y protegerla en un camino
  // y no en el otro es peor que no protegerla: enseña a confiar en el aviso.
  const [pendiente, setPendiente] = useState<Pendiente | null>(null);

  const conteo = armarConteo(catalogo, tecleadas);
  const totalManual = montoTecleado(totalAMano);
  const total = camino === 'contar' ? conteo.total : (totalManual ?? 0);

  const hayCaptura = camino === 'contar'
    ? Object.values(tecleadas).some((v) => v.trim() !== '')
    : totalAMano.trim() !== '' || motivo.trim() !== '';

  const limpiar = () => {
    setTecleadas({});
    setTotalAMano('');
    setMotivo('');
    setPendiente(null);
  };

  const pedirCamino = (c: Camino) => {
    if (c === camino) return;
    if (hayCaptura) {
      setPendiente({ que: 'camino', a: c });
      return;
    }
    setCamino(c);
  };
  const pedirSalida = () => {
    if (hayCaptura) {
      setPendiente({ que: 'cerrar' });
      return;
    }
    onClose();
    limpiar();
  };
  const confirmarPendiente = () => {
    if (pendiente === null) return;
    limpiar();
    if (pendiente.que === 'camino') {
      setCamino(pendiente.a);
      return;
    }
    onClose();
  };

  // Por qué el botón está apagado, en el orden en que el operador lo puede resolver. Se apaga en vez
  // de dejarlo llegar al rechazo con el cajón contado: un conteo perdido se vuelve a contar a mano.
  const falta = camino === 'contar'
    ? (conteo.invalidas.length > 0 ? 'piezas' : null)
    : (totalManual === undefined ? 'total' : (motivo.trim() === '' ? 'motivo' : null));

  const confirmar = () => {
    if (falta !== null) return;
    onConfirmar(camino === 'contar'
      ? { counts: conteo.renglones, total: conteo.total }
      : { total: totalManual ?? 0, manualReason: motivo.trim() });
  };

  const billetes = catalogo.filter((d) => !d.isCoin);
  const monedas = catalogo.filter((d) => d.isCoin);

  return (
    <DrawerRoot open={isOpen} placement="bottom"
      onOpenChange={(e) => { if (!e.open) pedirSalida(); }}>
      <DrawerBackdrop />
      {/* dvh y no vh: con el teclado numérico abierto la ventana visual se encoge ~250 px y `vh`
          seguiría midiendo la pantalla completa, dejando el total y el botón debajo del teclado. */}
      <DrawerContent borderTopRadius="l3" maxH="92dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <Text fontSize="lg" fontWeight="700">{titulo}</Text>
          {/* El interruptor va AQUÍ, lejos de la rejilla: un tap accidental pegado a las teclas que
              más se tocan descarta el conteo entero, y la confirmación no arregla el susto. */}
          <Tabs.Root mt={2} value={camino} onValueChange={(e) => pedirCamino(e.value as Camino)}>
            <Tabs.List>
              <Tabs.Trigger value="contar" minH="44px">Contar piezas</Tabs.Trigger>
              <Tabs.Trigger value="total" minH="44px">Escribir el total</Tabs.Trigger>
            </Tabs.List>
          </Tabs.Root>
          {pendiente !== null && (
            <HStack mt={3} gap={2} p={3} borderWidth="1px" borderRadius="md" bg="bg.subtle" flexWrap="wrap">
              <Text fontSize="sm" flex="1" minW="200px">
                {pendiente.que === 'camino'
                  ? 'Al cambiar se borra lo que ya capturaste.'
                  : 'Si sales ahora se borra lo que ya capturaste.'}
              </Text>
              <Button size="sm" minH="44px" variant="outline" onClick={() => setPendiente(null)}>
                {pendiente.que === 'camino' ? 'Seguir aquí' : 'Seguir contando'}
              </Button>
              <Button size="sm" minH="44px" colorPalette="orange" onClick={confirmarPendiente}>
                {pendiente.que === 'camino' ? 'Borrar y cambiar' : 'Salir sin guardar'}
              </Button>
            </HStack>
          )}
        </DrawerHeader>

        <DrawerBody flex="1" minH={0} overflowY="auto" pb={2}>
          {camino === 'contar' ? (
            isLoading ? (
              <Text color="fg.muted">Cargando…</Text>
            ) : (
              <VStack align="stretch" gap={3}>
                <Grupo titulo="Billetes" piezas={billetes} currency={currency}
                  tecleadas={tecleadas} onChange={setTecleadas} />
                <Grupo titulo="Monedas" piezas={monedas} currency={currency}
                  tecleadas={tecleadas} onChange={setTecleadas} />
                {conteo.invalidas.length > 0 && (
                  <Text fontSize="sm" color="red.fg">
                    Revisa las piezas de {conteo.invalidas
                      .map((id) => etiqueta(catalogo.find((d) => d.id === id)!, currency))
                      .join(', ')}: escribe solo el número de piezas.
                  </Text>
                )}
              </VStack>
            )
          ) : (
            <VStack align="stretch" gap={3} maxW="480px">
              <Box>
                <Text fontSize="xs" color="fg.muted" mb={1}>Efectivo</Text>
                <Input minH="44px" aria-label="Efectivo" value={totalAMano} inputMode="decimal"
                  autoComplete="off" spellCheck={false}
                  onChange={(e) => setTotalAMano(e.target.value)} />
              </Box>
              <Box>
                <Text fontSize="xs" color="fg.muted" mb={1}>Por qué no se contó pieza por pieza</Text>
                <Input minH="44px" aria-label="Por qué no se contó pieza por pieza" value={motivo}
                  autoComplete="off" onChange={(e) => setMotivo(e.target.value)} />
              </Box>
              {falta === 'total' && totalAMano.trim() !== '' && (
                <Text fontSize="sm" color="red.fg">Escribe el efectivo con números, sin comas.</Text>
              )}
              {falta === 'motivo' && (
                <Text fontSize="sm" color="fg.muted">
                  Escribe por qué no se contó: es lo que deja explicar este arqueo después.
                </Text>
              )}
            </VStack>
          )}
        </DrawerBody>

        {/* Footer FIJO: es lo único que mantiene el total y el botón a la vista con el teclado
            abierto, que es justo cuando se está capturando. */}
        <DrawerFooter borderTopWidth="1px" pt={3}>
          <HStack gap={3} w="100%" flexWrap="wrap">
            <VStack align="start" gap={0} flex="1" minW="120px">
              <Text fontSize="xs" color="fg.muted">Total</Text>
              <Text fontSize="xl" fontWeight="700" aria-label="Total contado">
                {money(total, currency)}
              </Text>
            </VStack>
            <Button minH="52px" variant="outline" onClick={pedirSalida}>
              Cancelar
            </Button>
            <Button minH="52px" colorPalette="orange" disabled={guardando || falta !== null}
              onClick={confirmar}>
              {etiquetaConfirmar}
            </Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

// DOS COLUMNAS, por el mismo reparto que la hoja de liquidación: once denominaciones apiladas no
// caben en el cuerpo de una hoja a 1024×600 con encabezado y footer puestos.
//
// SIN MEDIR TODAVÍA, y por eso no hay una cifra aquí: el alto real lo dan las clases de Chakra y
// hace falta un navegador de verdad ([docs/presupuesto-de-pantalla-1024x600.md]). La aritmética de
// los defaults del recipe deja el cuerpo al filo —del orden de 5 px— así que el renglón de las
// monedas chicas es el candidato a quedar fuera. Lo mide T029 con Playwright a 1024×600; si no
// cabe, lo más barato de recortar es el alto del interruptor del encabezado.
function Grupo({ titulo, piezas, currency, tecleadas, onChange }: {
  titulo: string;
  piezas: Denomination[];
  currency: string;
  tecleadas: PiezasTecleadas;
  onChange: (t: PiezasTecleadas) => void;
}) {
  if (piezas.length === 0) return null;
  return (
    <Box>
      <Text fontSize="xs" color="fg.muted" mb={1}>{titulo}</Text>
      <Grid templateColumns="repeat(2, 1fr)" gap={2}>
        {piezas.map((d) => (
          <Renglon key={d.id} d={d} currency={currency} valor={tecleadas[d.id] ?? ''}
            onChange={(v) => onChange({ ...tecleadas, [d.id]: v })} />
        ))}
      </Grid>
    </Box>
  );
}

function Renglon({ d, currency, valor, onChange }: {
  d: Denomination;
  currency: string;
  valor: string;
  onChange: (v: string) => void;
}) {
  const nombre = etiqueta(d, currency);
  const campo = leerPiezas(valor);
  const piezas = campo.estado === 'valido' ? campo.piezas : 0;
  const malo = campo.estado === 'invalido';
  // El ajuste se apaga con el campo mal capturado: sumarle uno a lo que no es un número lo
  // descartaría en silencio, que es exactamente lo que no puede pasar con el dinero del cajón.
  const ajustar = (delta: number) => onChange(String(Math.max(0, piezas + delta)));

  // EL `−` Y EL `+` VAN A LOS DOS LADOS DEL CAMPO, no juntos: pegados quedaban a 4 px uno del otro
  // con efectos opuestos, y un dedo que apunta a sumar y cae corto RESTA en silencio — en el control
  // que existe justo para corregir un dedazo. Separarlos con el campo en medio es lo que hace que
  // errarle a uno no active el otro.
  //
  // Y el campo es el más grande de la fila a propósito: es el control principal, y con las dos
  // teclas ocupando más superficie que él el ojo iba primero a ellas, que es el camino de los 40
  // taps.
  return (
    <HStack gap={2}>
      <Text w="56px" fontWeight="600" fontSize="sm">{nombre}</Text>
      <Button minH="44px" minW="44px" px={0} variant="ghost" aria-label={`Una menos de ${nombre}`}
        disabled={malo || piezas === 0} onClick={() => ajustar(-1)}>−</Button>
      <Input w="96px" minH="48px" textAlign="center" fontSize="lg" fontWeight="600"
        value={valor} inputMode="numeric"
        autoComplete="off" spellCheck={false} aria-label={`Piezas de ${nombre}`}
        aria-invalid={malo} borderColor={malo ? 'red.solid' : undefined}
        onChange={(e) => onChange(e.target.value)} />
      <Button minH="44px" minW="44px" px={0} variant="ghost" aria-label={`Una más de ${nombre}`}
        disabled={malo} onClick={() => ajustar(1)}>+</Button>
      <Text flex="1" textAlign="right" fontSize="sm" color="fg.muted" aria-label={`Subtotal de ${nombre}`}>
        {piezas > 0 ? money(round2(Number(d.value) * piezas), currency) : ''}
      </Text>
    </HStack>
  );
}

// Las de menos de un peso se nombran en centavos: "$0.5" no es como se llama esa moneda.
function etiqueta(d: Denomination, currency: string): string {
  const n = Number(d.value);
  return n < 1 ? `${Math.round(n * 100)}¢` : money(n, currency);
}
