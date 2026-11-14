import { useEffect, useRef, useState } from 'react';
import { Badge, Box, Button, Flex, Grid, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LuCircleCheck, LuCircleHelp, LuSearch, LuUndo2 } from 'react-icons/lu';
import { ApiError } from '../../api/client';
import { medirAccion } from '../../api/uso';
import { stockOutApi, type OutItem } from '../../api/stockOut';
import { uuid } from '../../utils/uuid';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';

// «Se acabó» (spec 084, maqueta M2, opción B v2): dos columnas, confirmación con un segundo toque sobre
// el mismo mosaico y lo reportado hoy siempre a la vista. Nunca pide cantidad: el servidor deja la
// existencia en 0. El segundo toque se queda aunque haya deshacer: deshacer no retira el aviso que las
// otras tabletas ya vieron.

// Gracia entre el primer y el segundo toque: un doble toque rápido sobre el mismo mosaico no confirma.
const GRACIA_MS = 400;
// El mosaico armado se desarma solo si nadie lo confirma.
const ARMADO_MS = 4000;
const BUSQUEDA_MS = 250;
const NO_LO_ENCUENTRO = '__no-lo-encuentro';

type Hecho = { nombre: string; id: number; confirmacion: boolean; hasta: number };
type Mensaje = { tipo: 'no-lo-encuentro'; texto: string } | { tipo: 'error'; texto: string };

const claveDe = (it: OutItem) => `${it.itemType}:${it.id}`;

const Titulo = ({ children }: { children: React.ReactNode }) => (
  <Text fontSize="xs" fontWeight="700" color="fg.muted" textTransform="uppercase" letterSpacing="0.05em">{children}</Text>
);

export function SeAcaboPage() {
  const qc = useQueryClient();
  const hora = useHoraDelNegocio();
  const [q, setQ] = useState('');
  const [buscado, setBuscado] = useState('');
  const [armado, setArmado] = useState<{ clave: string; desde: number } | null>(null);
  // El reporte que todavía se puede deshacer vive aparte del último mensaje: un error al deshacer o al
  // reportar otro no se lleva el botón de deshacer que sigue vigente.
  const [hecho, setHecho] = useState<Hecho | null>(null);
  const [mensaje, setMensaje] = useState<Mensaje | null>(null);
  const [ahora, setAhora] = useState(() => Date.now());
  // Una clave por destino: reintentar el MISMO artículo o texto tras un error la reusa (si el servidor
  // ya lo guardó, devuelve el primero); cualquier otro destino estrena la suya. Si se reusara para otro
  // artículo, el servidor devolvería el reporte del anterior como si fuera de éste.
  const llave = useRef<{ destino: string; clave: string } | null>(null);
  const llaveDe = (destino: string) => {
    if (llave.current?.destino !== destino) llave.current = { destino, clave: uuid() };
    return llave.current.clave;
  };

  useEffect(() => {
    const t = setTimeout(() => setBuscado(q.trim()), BUSQUEDA_MS);
    return () => clearTimeout(t);
  }, [q]);
  useEffect(() => {
    if (!armado) return;
    const t = setTimeout(() => setArmado(null), ARMADO_MS);
    return () => clearTimeout(t);
  }, [armado]);
  useEffect(() => {
    if (!hecho) return;
    const t = setInterval(() => setAhora(Date.now()), 1000);
    return () => clearInterval(t);
  }, [hecho]);

  const { data: sug } = useQuery({ queryKey: ['se-acabo', 'sugerencias'], queryFn: stockOutApi.suggestions });
  const { data: resultados } = useQuery({
    queryKey: ['se-acabo', 'buscar', buscado], queryFn: () => stockOutApi.search(buscado), enabled: buscado !== '',
  });

  const error = (e: unknown) => setMensaje({ tipo: 'error', texto: e instanceof ApiError ? e.message : 'No se pudo avisar; vuelve a intentar.' });
  const reportar = useMutation({
    mutationFn: (it: OutItem) => stockOutApi.report(it, llaveDe(`articulo:${claveDe(it)}`)),
    onSuccess: (res, it) => {
      medirAccion('se-acabo', res.kind === 'confirmation' ? 'confirmar' : 'reportar');
      llave.current = null;
      setQ('');
      setMensaje(null);
      // La cuenta sale del reloj de la tableta y de los segundos que da el servidor, no de la hora
      // límite de la base: con la tableta atrasada prometía más de lo que el servidor acepta. Y arranca
      // con la hora de ahora, no con la del último tic.
      const ahoraMismo = Date.now();
      setAhora(ahoraMismo);
      setHecho({ nombre: it.name, id: res.id, confirmacion: res.kind === 'confirmation', hasta: ahoraMismo + (sug?.undoSeconds ?? 90) * 1000 });
      void qc.invalidateQueries({ queryKey: ['se-acabo'] });
      void qc.invalidateQueries({ queryKey: ['avisos'] });
    },
    onError: error,
  });
  const noLoEncuentro = useMutation({
    mutationFn: (texto: string) => stockOutApi.notFound(texto, llaveDe(`texto:${texto}`)),
    onSuccess: (_, texto) => {
      medirAccion('se-acabo', 'no-lo-encuentro');
      llave.current = null;
      setQ('');
      setHecho(null);
      setMensaje({ tipo: 'no-lo-encuentro', texto });
      void qc.invalidateQueries({ queryKey: ['se-acabo'] });
    },
    onError: error,
  });
  // Mientras uno de los dos avisos va en camino no sale el otro: los dos llevarían la misma intención.
  const enviando = reportar.isPending || noLoEncuentro.isPending;
  const deshacer = useMutation({
    mutationFn: (id: number) => stockOutApi.undo(id),
    onSuccess: () => { medirAccion('se-acabo', 'deshacer'); setHecho(null); setMensaje(null); void qc.invalidateQueries({ queryKey: ['se-acabo'] }); },
    onError: error,
  });

  // Primer toque: arma. Segundo toque sobre el mismo, pasada la gracia: confirma. Tocar otro mueve el armado.
  // La hora del toque sale del propio evento: así no se lee el reloj al pintar.
  const tocar = (clave: string, t: number, confirmar: () => void) => {
    if (armado?.clave === clave) {
      if (t - armado.desde < GRACIA_MS) return;
      setArmado(null);
      confirmar();
      return;
    }
    setArmado({ clave, desde: t });
  };

  const mosaico = (it: OutItem, detalle?: string) => {
    const clave = claveDe(it);
    const listo = armado?.clave === clave;
    return (
      <Button key={clave} data-row minH="64px" h="auto" py={2} whiteSpace="normal" textAlign="left" justifyContent="flex-start"
        variant={listo ? 'solid' : 'outline'} colorPalette={listo ? 'red' : 'gray'} disabled={enviando}
        onClick={(e) => tocar(clave, e.timeStamp, () => reportar.mutate(it))}>
        <VStack align="start" gap={0} minW={0}>
          <Text fontWeight="700" fontSize="md" lineClamp={2}>{it.name}</Text>
          <Text fontSize="xs" color={listo ? 'white' : 'fg.muted'} minH="16px">
            {listo ? 'Toca otra vez para avisar que se acabó' : detalle ?? ''}
          </Text>
        </VStack>
      </Button>
    );
  };

  const restantes = hecho ? Math.max(0, Math.ceil((hecho.hasta - ahora) / 1000)) : 0;
  // Los resultados son del texto que se buscó; mientras el texto nuevo espera su búsqueda, no se
  // enseñan los del anterior con su «N coinciden».
  const vigentes = buscado === q.trim() ? resultados : undefined;
  const lista = q.trim() ? (
    <VStack align="stretch" gap={2}>
      <Titulo>{vigentes ? (vigentes.items.length ? `${vigentes.items.length} coinciden` : 'Sin coincidencias') : 'Buscando…'}</Titulo>
      {(vigentes?.items ?? []).map((it) => mosaico(it, it.openOut ? 'Ya está reportado: suma tu confirmación' : undefined))}
      <Box h="24px" />
      <Button data-row minH="64px" borderStyle="dashed" justifyContent="flex-start" disabled={enviando}
        variant={armado?.clave === NO_LO_ENCUENTRO ? 'solid' : 'outline'} colorPalette={armado?.clave === NO_LO_ENCUENTRO ? 'orange' : 'gray'}
        onClick={(e) => tocar(NO_LO_ENCUENTRO, e.timeStamp, () => noLoEncuentro.mutate(q.trim()))}>
        <LuCircleHelp /> <Text fontWeight="700">No lo encuentro</Text>
        <Text>{armado?.clave === NO_LO_ENCUENTRO ? '· toca otra vez para avisar a quien compra' : `· avisar «${q.trim()}» a quien compra`}</Text>
      </Button>
    </VStack>
  ) : (
    <VStack align="stretch" gap={2}>
      <Titulo>Lo que más se acaba</Titulo>
      {sug && sug.frequent.length === 0 && <Text color="fg.muted">Escribe arriba lo que se acabó.</Text>}
      <Grid templateColumns="repeat(auto-fill, minmax(220px, 1fr))" gap={2}>
        {(sug?.frequent ?? []).map((it) => mosaico(it, it.times ? `Reportado ${it.times} ${it.times === 1 ? 'vez' : 'veces'} este mes` : undefined))}
      </Grid>
    </VStack>
  );

  return (
    <Flex direction="column" h="100%" minH={0}>
      <Flex h="52px" align="center" px={3} borderBottomWidth="1px" flexShrink={0}>
        <Text fontWeight="700" fontSize="lg">Se acabó</Text>
      </Flex>
      <Grid templateColumns={{ base: 'minmax(0, 1fr)', md: 'minmax(0, 1fr) minmax(0, 340px)' }} gap={4} p={3} flex="1" minH={0}>
        <VStack align="stretch" gap={3} minH={0} overflowY="auto">
          <HStack borderWidth="2px" borderColor="brand.solid" borderRadius="lg" px={3} minH="64px" bg="bg" flexShrink={0}>
            <LuSearch size={24} />
            <Input aria-label="¿Qué se acabó?" placeholder="¿Qué se acabó? Escribe parte del nombre" border="none" fontSize="xl" minH="60px"
              value={q} onChange={(e) => { setQ(e.target.value); setArmado(null); }} _focus={{ boxShadow: 'none' }} />
            {q && <Button minH="44px" variant="ghost" onClick={() => setQ('')}>Borrar</Button>}
          </HStack>
          {/* Hueco de alto fijo: el aviso aparece y se va sin mover los mosaicos bajo el dedo de quien
              reporta varios seguidos. */}
          <Box data-testid="hueco-del-aviso" data-alto="76" minH="76px" display="flex" flexDirection="column" justifyContent="center" flexShrink={0}>
            {hecho && (
              <HStack role="status" bg="green.subtle" borderRadius="lg" px={4} minH="64px" gap={3}>
                <LuCircleCheck size={24} />
                <Box flex="1" minW={0}>
                  <Text fontWeight="700">{hecho.confirmacion ? `Ya estaba reportado: sumamos tu confirmación de ${hecho.nombre}` : `Listo: ${hecho.nombre} quedó en 0 y se avisó a quien compra`}</Text>
                  {/* Un error posterior se dice aquí, sin quitar el deshacer que sigue vigente. */}
                  {mensaje?.tipo === 'error'
                    ? <Text role="alert" fontSize="sm" color="fg.error" fontWeight="600">{mensaje.texto}</Text>
                    : !hecho.confirmacion && <Text fontSize="sm" color="fg.muted">Le llega en la campana y por correo en unos minutos</Text>}
                </Box>
                {restantes > 0 && (
                  <Button minH="52px" variant="outline" bg="bg" disabled={deshacer.isPending} onClick={() => deshacer.mutate(hecho.id)}>
                    <LuUndo2 /> Deshacer · {restantes} s
                  </Button>
                )}
              </HStack>
            )}
            {!hecho && mensaje?.tipo === 'no-lo-encuentro' && (
              <HStack role="status" bg="orange.subtle" borderRadius="lg" px={4} minH="64px" gap={3}>
                <LuCircleCheck size={24} />
                <Box flex="1"><Text fontWeight="700">Avisamos «{mensaje.texto}» a quien compra</Text>
                  <Text fontSize="sm" color="fg.muted">No se tocó ninguna existencia. Quien compra decide si lo da de alta.</Text></Box>
              </HStack>
            )}
            {!hecho && mensaje?.tipo === 'error' && <Text role="alert" color="fg.error" fontWeight="600" px={1}>{mensaje.texto}</Text>}
          </Box>
          {lista}
        </VStack>
        <Box minH={0} overflowY="auto" borderLeftWidth={{ base: 0, md: '1px' }} pl={{ base: 0, md: 4 }}>
          <Titulo>Reportado hoy</Titulo>
          {sug && sug.today.length === 0 && <Text color="fg.muted" py={2}>Hoy nadie ha reportado nada.</Text>}
          {(sug?.today ?? []).map((r) => (
            <Flex key={r.id} data-row minH="56px" align="center" borderBottomWidth="1px" gap={2} opacity={r.undoneAt ? 0.6 : 1}>
              <Box flex="1" minW={0}>
                <Text fontWeight="600" lineClamp={1}>{r.name}</Text>
                <Text fontSize="sm" color="fg.muted">
                  {hora.soloHora(r.reportedAt)} · {r.reportedBy}
                  {r.kind === 'confirmation' ? ' · lo confirmó' : ''}{r.undoneAt ? ' · deshecho' : ''}
                </Text>
              </Box>
              {r.kind === 'not_found' && r.open && <Badge colorPalette="orange">Por revisar</Badge>}
            </Flex>
          ))}
        </Box>
      </Grid>
    </Flex>
  );
}
