import { useCallback, useEffect, useMemo, useState } from 'react';
import { Box, Button, Flex, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { LuLock, LuUndo2 } from 'react-icons/lu';
import { ApiError } from '../../api/client';
import { moneyExact, normalize } from '../../utils/format';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import {
  borrarPareja,
  candidatos,
  confirmarLote,
  guardarPareja,
  marcarSoloEnPlataforma,
  quitarSoloEnPlataforma,
  tablero,
  type CandidatoDelPOS,
  type ClaseDeItem,
  type GrupoDeEmparejamiento,
  type RenglonDelTablero,
  type TableroDeEmparejamiento,
} from '../../api/plataformas';

/**
 * Emparejar la tienda conectada con el catálogo del POS (spec 026, diseño B).
 *
 * La lista a la izquierda y el panel a la derecha, a pantalla completa: el dueño eligió esta forma
 * porque aprovecha todo el ancho. La lista tiene su propio scroll (si no, a 600 px de alto solo se
 * verían 7 renglones) y el panel cambia según el grupo: decidir, revisar, corregir o deshacer una
 * decisión. Nada aquí escribe en la plataforma.
 */

interface Props {
  conexionId: number;
}

// Un solo alto para todo lo que se toca: la constitución pide 44 px como mínimo.
const TOQUE = '44px';

const NOMBRE_DEL_GRUPO: Record<GrupoDeEmparejamiento, string> = {
  unpaired: 'Sin pareja',
  toReview: 'Por revisar',
  done: 'Listos',
  excluded: 'Solo en',
};


interface Captura {
  item: RenglonDelTablero;
  destino: CandidatoDelPOS;
  /** Los platillos que ya van a ese producto, con el que da el precio hoy. */
  hermanos: RenglonDelTablero[];
  elegido: string;
}

export function EmparejarPage({ conexionId }: Props) {
  const { fechaYHora } = useHoraDelNegocio();
  const [datos, setDatos] = useState<TableroDeEmparejamiento | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [grupo, setGrupo] = useState<GrupoDeEmparejamiento>('unpaired');
  const [nivel, setNivel] = useState<ClaseDeItem>('platillo');
  const [filtro, setFiltro] = useState('');
  const [seleccion, setSeleccion] = useState<string | null>(null);
  const [modo, setModo] = useState<'uno' | 'lote'>('uno');
  const [desmarcados, setDesmarcados] = useState<Set<string>>(new Set());
  // Lo último que se confirmó, para deshacerlo con UN toque aunque haya sido un lote de doce:
  // corregir doce parejas una por una cuesta más que el error.
  const [ultimas, setUltimas] = useState<RenglonDelTablero[]>([]);
  const [captura, setCaptura] = useState<Captura | null>(null);
  const [buscarOtro, setBuscarOtro] = useState(false);
  const [verCambios, setVerCambios] = useState(false);
  const [ocupado, setOcupado] = useState(false);

  const cargar = useCallback(() => {
    tablero(conexionId)
      .then((d) => {
        setDatos(d);
        setError(null);
      })
      .catch(() => setError('No se pudo abrir el emparejamiento.'));
  }, [conexionId]);
  useEffect(cargar, [cargar]);

  const items = useMemo(() => datos?.items ?? [], [datos]);
  const plataforma = (datos?.platformName ?? 'la plataforma').split(' ')[0];

  const visibles = useMemo(() => {
    const n = normalize(filtro.trim());
    return items.filter(
      (i) =>
        i.group === grupo &&
        i.kind === nivel &&
        (!n ||
          normalize(i.name).includes(n) ||
          normalize(i.link?.localName ?? '').includes(n) ||
          normalize(i.proposal?.localName ?? '').includes(n)),
    );
  }, [items, grupo, nivel, filtro]);

  const actual = visibles.find((i) => i.externalId === seleccion) ?? visibles[0];

  const elegirGrupo = (g: GrupoDeEmparejamiento) => {
    setGrupo(g);
    setSeleccion(null);
    setCaptura(null);
    setBuscarOtro(false);
  };

  const conError = async (fn: () => Promise<unknown>, mensaje: string) => {
    if (ocupado) return;
    setOcupado(true);
    try {
      await fn();
      cargar();
    } catch (e) {
      if (!(e instanceof ApiError && e.code === 'CAPTURE_PRICE_REQUIRED')) setError(mensaje);
      throw e;
    } finally {
      setOcupado(false);
    }
  };

  // Ligar puede pedir el precio de captura: el servidor dice CAPTURE_PRICE_REQUIRED y la pantalla
  // abre la elección (tablero B5) en vez de adivinar. Quien configura decide; quien opera, nunca.
  const ligar = async (item: RenglonDelTablero, destino: CandidatoDelPOS, reemplazar = false) => {
    try {
      await conError(
        () => guardarPareja(conexionId, item.externalId, {
          localId: destino.id, localKind: destino.localKind, kind: item.kind, replace: reemplazar,
        }),
        'No se pudo guardar la pareja.',
      );
      setUltimas([item]);
      setSeleccion(null);
      setBuscarOtro(false);
    } catch (e) {
      if (e instanceof ApiError && e.code === 'CAPTURE_PRICE_REQUIRED') {
        const hermanos = items.filter(
          (i) => i.link && i.link.localId === destino.id && i.link.localKind === destino.localKind && i.externalId !== item.externalId,
        );
        const actualCaptura = hermanos.find((h) => h.link?.isCapturePrice)?.externalId ?? item.externalId;
        setCaptura({ item, destino, hermanos, elegido: actualCaptura });
      }
    }
  };

  const confirmarCaptura = async () => {
    if (!captura) return;
    const { item, destino, elegido } = captura;
    await conError(async () => {
      await guardarPareja(conexionId, item.externalId, {
        localId: destino.id, localKind: destino.localKind, kind: item.kind,
        capturePrice: elegido === item.externalId,
      });
      const otro = captura.hermanos.find((h) => h.externalId === elegido);
      if (otro && !otro.link?.isCapturePrice) {
        await guardarPareja(conexionId, otro.externalId, {
          localId: destino.id, localKind: destino.localKind, kind: otro.kind, capturePrice: true,
        });
      }
    }, 'No se pudo guardar la pareja.')
      .then(() => setUltimas([item]))
      .catch(() => undefined);
    setCaptura(null);
    setSeleccion(null);
  };

  const botonDeshacer =
    ultimas.length === 0 ? null : (
      <Button
        minH={TOQUE}
        variant="ghost"
        onClick={() =>
          conError(async () => {
            for (const u of ultimas) await borrarPareja(conexionId, u.externalId);
          }, 'No se pudo deshacer.')
            .then(() => setUltimas([]))
            .catch(() => undefined)
        }
      >
        <LuUndo2 /> {ultimas.length === 1 ? `Deshacer «${ultimas[0].name}»` : `Deshacer ${ultimas.length}`}
      </Button>
    );

  if (error) return <Text color="red.600">{error}</Text>;
  if (!datos) return <Text color="fg.muted">Cargando…</Text>;

  const total = items.length;
  const cambios = datos.priceChanges ?? [];
  const porRevisar = visibles.filter((i) => i.group === 'toReview');
  const marcados = porRevisar.filter((i) => !desmarcados.has(i.externalId));

  return (
    <Flex direction="column" gap={2} flex="1" minH={0}>
      <HStack minH={TOQUE} gap={3} wrap="wrap">
        <Text fontWeight="bold" fontSize="lg">
          {datos.platformName} · {datos.storeLabel}
        </Text>
        <Text fontSize="sm" color="fg.muted" flex="1">
          Leído {fechaYHora(datos.readAt)} · {datos.counts.done} de {total} emparejados ·{' '}
          <LuLock style={{ display: 'inline', verticalAlign: '-2px' }} aria-hidden /> Precios: los pone {plataforma}
        </Text>
      </HStack>

      {cambios.length > 0 && (
        <Box borderWidth="1px" borderColor="blue.200" bg="blue.50" borderRadius="md" px={3} py={1}>
          <HStack>
            <Text fontSize="sm" flex="1">
              {cambios.length === 1 ? '1 precio cambió' : `${cambios.length} precios cambiaron`} en la última lectura.
            </Text>
            <Button minH={TOQUE} variant="ghost" size="sm" onClick={() => setVerCambios((v) => !v)}>
              {verCambios ? 'Ocultar' : 'Ver cuáles'}
            </Button>
          </HStack>
          {verCambios &&
            cambios.map((c) => (
              <Text key={c.name} fontSize="sm">
                {c.name}: {c.old ? `${moneyExact(c.old)} → ` : ''}
                {moneyExact(c.new)}
              </Text>
            ))}
        </Box>
      )}

      <HStack gap={2} wrap="wrap">
        {(['unpaired', 'toReview', 'done', 'excluded'] as GrupoDeEmparejamiento[]).map((g) => {
          const n = datos.counts[g];
          const etiqueta = g === 'excluded' ? `Solo en ${plataforma} ${n}` : `${NOMBRE_DEL_GRUPO[g]} ${n}`;
          return (
            <Button
              key={g}
              minH={TOQUE}
              borderRadius="full"
              variant={grupo === g ? 'solid' : 'outline'}
              onClick={() => elegirGrupo(g)}
            >
              {etiqueta}
            </Button>
          );
        })}
        <Box flex="1" />
        <HStack role="group" aria-label="Nivel" gap={0} borderWidth="1px" borderRadius="md" overflow="hidden">
          {(['platillo', 'opcion'] as ClaseDeItem[]).map((k) => (
            <Button
              key={k}
              minH={TOQUE}
              borderRadius="0"
              variant={nivel === k ? 'subtle' : 'ghost'}
              onClick={() => {
                setNivel(k);
                setSeleccion(null);
              }}
            >
              {k === 'platillo' ? 'Platillos' : 'Opciones'}
            </Button>
          ))}
        </HStack>
      </HStack>

      <Flex direction={{ base: 'column', md: 'row' }} gap={4} flex="1" minH={0}>
        <Flex direction="column" gap={2} w={{ base: '100%', md: '330px' }} flexShrink={0} minH={0}>
          <Input
            minH={TOQUE}
            type="search"
            aria-label={`Buscar en ${plataforma}`}
            placeholder={`Buscar en ${plataforma}…`}
            value={filtro}
            onChange={(e) => setFiltro(e.target.value)}
          />
          <Box data-testid="lista-de-la-tienda" flex="1" minH={0} overflowY="auto" maxH={{ base: '35dvh', md: 'none' }}>
            {visibles.length === 0 && (
              <Text color="fg.muted" p={3}>
                No hay nada aquí.
              </Text>
            )}
            {visibles.map((i) => (
              <Button
                key={i.externalId}
                w="100%"
                minH="56px"
                justifyContent="flex-start"
                variant="ghost"
                borderRadius="0"
                borderBottomWidth="1px"
                borderLeftWidth="4px"
                borderLeftColor={actual?.externalId === i.externalId ? 'red.500' : 'transparent'}
                bg={actual?.externalId === i.externalId ? 'red.50' : undefined}
                onClick={() => {
                  setSeleccion(i.externalId);
                  setCaptura(null);
                  setBuscarOtro(false);
                }}
              >
                <VStack align="start" gap={0} overflow="hidden">
                  <Text fontWeight="medium" truncate maxW="280px">
                    {i.name}
                  </Text>
                  <Text fontSize="xs" color="fg.muted">
                    {moneyExact(i.price)}
                    {i.link ? ` → ${i.link.localName}` : ''}
                  </Text>
                </VStack>
              </Button>
            ))}
          </Box>
        </Flex>

        <Box as="section" aria-label="Decidir" flex="1" minW={0} minH={0} overflowY="auto">
          {captura ? (
            <PanelDeCaptura
              captura={captura}
              onElegir={(elegido) => setCaptura({ ...captura, elegido })}
              onCancelar={() => setCaptura(null)}
              onLigar={confirmarCaptura}
            />
          ) : !actual ? (
            <Text color="fg.muted">Nada que hacer en este grupo.</Text>
          ) : (grupo === 'unpaired' || buscarOtro) ? (
            <PanelDeCandidatos
              conexionId={conexionId}
              item={actual}
              plataforma={plataforma}
              reemplazar={buscarOtro && grupo === 'done'}
              onLigar={(c) => ligar(actual, c, buscarOtro && grupo === 'done')}
              onSoloEnPlataforma={() =>
                conError(() => marcarSoloEnPlataforma(conexionId, actual.externalId), 'No se pudo guardar la decisión.').catch(() => undefined)
              }
              onSaltar={() => {
                const idx = visibles.findIndex((v) => v.externalId === actual.externalId);
                setSeleccion(visibles[(idx + 1) % visibles.length]?.externalId ?? null);
              }}
            />
          ) : grupo === 'toReview' ? (
            <VStack align="stretch" gap={3}>
              <HStack role="group" aria-label="Cómo revisar" gap={0} borderWidth="1px" borderRadius="md" overflow="hidden">
                <Button flex="1" minH={TOQUE} borderRadius="0" variant={modo === 'uno' ? 'solid' : 'ghost'} onClick={() => setModo('uno')}>
                  Uno por uno
                </Button>
                <Button flex="1" minH={TOQUE} borderRadius="0" variant={modo === 'lote' ? 'solid' : 'ghost'} onClick={() => setModo('lote')}>
                  En lote
                </Button>
              </HStack>
              {modo === 'uno' && actual.proposal ? (
                <>
                  <Comparacion item={actual} destino={actual.proposal.localName} plataforma={plataforma} />
                  <HStack gap={2}>
                    <Button
                      flex="1"
                      minH="52px"
                      colorPalette="green"
                      onClick={() =>
                        ligar(actual, { localKind: actual.proposal!.localKind, id: actual.proposal!.localId, name: actual.proposal!.localName, linkedCount: 0 })
                      }
                    >
                      ✓ Es el mismo
                    </Button>
                    <Button minH="52px" variant="outline" onClick={() => setBuscarOtro(true)}>
                      Es otro…
                    </Button>
                  </HStack>
                  <Text fontSize="sm" color="fg.muted">
                    Al confirmar pasa sola a la siguiente.
                  </Text>
                  <HStack>
                    {botonDeshacer}
                    <Box flex="1" />
                    <Button
                      minH={TOQUE}
                      variant="ghost"
                      onClick={() => {
                        const idx = visibles.findIndex((v) => v.externalId === actual.externalId);
                        setSeleccion(visibles[(idx + 1) % visibles.length]?.externalId ?? null);
                      }}
                    >
                      Saltar →
                    </Button>
                  </HStack>
                </>
              ) : (
                <>
                  <Text fontSize="sm" color="fg.muted">
                    Se confirman las marcadas. Las desmarcadas se quedan sin pareja.
                  </Text>
                  {porRevisar.map((i) => (
                    <HStack as="label" key={i.externalId} minH={TOQUE} gap={3} borderBottomWidth="1px" cursor="pointer">
                      <input
                        type="checkbox"
                        style={{ width: 22, height: 22 }}
                        checked={!desmarcados.has(i.externalId)}
                        aria-label={`${i.name} → ${i.proposal?.localName ?? ''}`}
                        onChange={() =>
                          setDesmarcados((s) => {
                            const n = new Set(s);
                            if (n.has(i.externalId)) n.delete(i.externalId);
                            else n.add(i.externalId);
                            return n;
                          })
                        }
                      />
                      <Text flex="1">{i.name}</Text>
                      <Text color="fg.muted">→ {i.proposal?.localName}</Text>
                    </HStack>
                  ))}
                  <HStack justify="flex-end">
                    {botonDeshacer}
                    <Box flex="1" />
                    <Text fontSize="sm">
                      {marcados.length} de {porRevisar.length} marcadas
                    </Text>
                    <Button
                      minH="48px"
                      colorPalette="green"
                      disabled={marcados.length === 0}
                      onClick={() =>
                        conError(async () => {
                          const r = await confirmarLote(conexionId, marcados.map((m) => m.externalId));
                          const hechas = new Set(r.confirmed ?? []);
                          setUltimas(marcados.filter((m) => hechas.has(m.externalId)));
                        }, 'No se pudieron confirmar.').catch(() => undefined)
                      }
                    >
                      ✓ Confirmar {marcados.length}
                    </Button>
                  </HStack>
                </>
              )}
            </VStack>
          ) : grupo === 'done' ? (
            <VStack align="stretch" gap={3}>
              <Text fontSize="xl" fontWeight="bold">
                {actual.name}
              </Text>
              <Text>
                ligado a <b>{actual.link?.localName}</b>
                {actual.link?.isCapturePrice ? ' · da el precio de la captura a mano' : ''}
              </Text>
              <HStack color="blue.700" fontSize="sm">
                <LuLock aria-hidden />
                <Text>
                  {moneyExact(actual.price)} en {plataforma} · lo pone {plataforma}
                </Text>
              </HStack>
              <HStack>
                <Button minH={TOQUE} variant="outline" onClick={() => setBuscarOtro(true)}>
                  Cambiar de producto…
                </Button>
                <Box flex="1" />
                <Button
                  minH={TOQUE}
                  variant="outline"
                  colorPalette="red"
                  onClick={() =>
                    conError(() => borrarPareja(conexionId, actual.externalId), 'No se pudo quitar la pareja.').catch(() => undefined)
                  }
                >
                  Quitar la pareja
                </Button>
              </HStack>
            </VStack>
          ) : (
            <VStack align="stretch" gap={3}>
              <Text fontSize="xl" fontWeight="bold">
                {actual.name}
              </Text>
              <Text>Marcado como que solo existe en {plataforma}.</Text>
              <Button
                minH={TOQUE}
                alignSelf="flex-start"
                variant="outline"
                onClick={() =>
                  conError(() => quitarSoloEnPlataforma(conexionId, actual.externalId), 'No se pudo deshacer la decisión.').catch(() => undefined)
                }
              >
                Volver a Sin pareja
              </Button>
            </VStack>
          )}
        </Box>
      </Flex>
    </Flex>
  );
}

function Comparacion({ item, destino, plataforma }: { item: RenglonDelTablero; destino: string; plataforma: string }) {
  return (
    <Box borderWidth="1px" borderColor="red.100" bg="red.50" borderRadius="lg" p={4}>
      <HStack align="center" gap={3}>
        <Box flex="1" minW={0}>
          <Text fontSize="xs" color="fg.muted">
            En {plataforma}
          </Text>
          <Text fontSize="xl" fontWeight="bold">
            {item.name}
          </Text>
          <Text fontSize="sm">{moneyExact(item.price)}</Text>
        </Box>
        <Text color="fg.muted" fontSize="xl" aria-hidden>
          →
        </Text>
        <Box flex="1" minW={0}>
          <Text fontSize="xs" color="fg.muted">
            En el POS
          </Text>
          <Text fontSize="xl" fontWeight="bold">
            {destino}
          </Text>
        </Box>
      </HStack>
    </Box>
  );
}

interface PanelDeCandidatosProps {
  conexionId: number;
  item: RenglonDelTablero;
  plataforma: string;
  reemplazar: boolean;
  onLigar: (c: CandidatoDelPOS) => void;
  onSoloEnPlataforma: () => void;
  onSaltar: () => void;
}

function PanelDeCandidatos({ conexionId, item, plataforma, onLigar, onSoloEnPlataforma, onSaltar }: PanelDeCandidatosProps) {
  const [q, setQ] = useState('');
  const [lista, setLista] = useState<CandidatoDelPOS[] | null>(null);

  useEffect(() => {
    let vigente = true;
    const t = setTimeout(() => {
      candidatos(conexionId, item.externalId, q)
        .then((c) => vigente && setLista(c))
        .catch(() => vigente && setLista([]));
    }, q ? 250 : 0);
    return () => {
      vigente = false;
      clearTimeout(t);
    };
  }, [conexionId, item.externalId, q]);

  return (
    <VStack align="stretch" gap={2}>
      <HStack align="baseline" gap={3}>
        <Text fontSize="xl" fontWeight="bold">
          {item.name}
        </Text>
        <Text color="fg.muted">
          {moneyExact(item.price)} en {plataforma}
        </Text>
      </HStack>
      <Text fontSize="xs" fontWeight="bold" color="fg.muted" letterSpacing="wide">
        ¿CON CUÁL DEL POS? · LOS MÁS PARECIDOS
      </Text>
      {lista === null && <Text color="fg.muted">Buscando…</Text>}
      {lista?.slice(0, 4).map((c) => (
        <Button key={c.id} minH="56px" variant="outline" justifyContent="space-between" onClick={() => onLigar(c)}>
          <VStack align="start" gap={0}>
            <Text fontWeight="medium">{c.name}</Text>
            {c.linkedCount > 0 && (
              <Text fontSize="xs" color="fg.muted">
                ya ligado a {c.linkedCount} {c.linkedCount === 1 ? 'platillo' : 'platillos'} de {plataforma}
              </Text>
            )}
          </VStack>
        </Button>
      ))}
      <Input
        minH={TOQUE}
        type="search"
        aria-label="Buscar en todo el POS"
        placeholder="Buscar en todo el POS…"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      <HStack pt={2} borderTopWidth="1px">
        <Button minH={TOQUE} variant="outline" onClick={onSoloEnPlataforma}>
          Solo existe en {plataforma}
        </Button>
        <Box flex="1" />
        <Button minH={TOQUE} variant="ghost" onClick={onSaltar}>
          Saltar →
        </Button>
      </HStack>
    </VStack>
  );
}

function PanelDeCaptura({
  captura,
  onElegir,
  onCancelar,
  onLigar,
}: {
  captura: Captura;
  onElegir: (externalId: string) => void;
  onCancelar: () => void;
  onLigar: () => void;
}) {
  const opciones = [...captura.hermanos, captura.item];
  return (
    <VStack align="stretch" gap={3}>
      <HStack align="baseline" gap={3}>
        <Text fontSize="xl" fontWeight="bold">
          {captura.item.name}
        </Text>
        <Text color="fg.muted">→ {captura.destino.name}</Text>
      </HStack>
      <Text>
        {captura.destino.name} queda con {opciones.length} platillos. ¿Con qué precio se cobra cuando un pedido se
        captura a mano?
      </Text>
      <VStack role="radiogroup" aria-label="Precio para capturar a mano" align="stretch" gap={2}>
        {opciones.map((o) => {
          const elegido = o.externalId === captura.elegido;
          return (
            <Button
              key={o.externalId}
              role="radio"
              aria-checked={elegido}
              minH="52px"
              justifyContent="space-between"
              variant="outline"
              borderWidth={elegido ? '2px' : '1px'}
              borderColor={elegido ? 'red.500' : undefined}
              onClick={() => onElegir(o.externalId)}
            >
              <Text>
                {o.name}
                {o.externalId === captura.item.externalId ? ' (esta)' : ''}
              </Text>
              <Text color="fg.muted">{moneyExact(o.price)}</Text>
            </Button>
          );
        })}
      </VStack>
      <HStack pt={2} borderTopWidth="1px">
        <Button minH={TOQUE} variant="outline" onClick={onCancelar}>
          Elegir otro producto
        </Button>
        <Box flex="1" />
        <Button minH="48px" colorPalette="green" onClick={onLigar}>
          ✓ Ligar
        </Button>
      </HStack>
    </VStack>
  );
}
