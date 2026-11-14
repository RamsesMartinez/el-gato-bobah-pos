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

type Aviso =
  | { tipo: 'hecho'; nombre: string; id: number; confirmacion: boolean; hasta: number }
  | { tipo: 'no-lo-encuentro'; texto: string }
  | { tipo: 'error'; texto: string };

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
  const [aviso, setAviso] = useState<Aviso | null>(null);
  const [ahora, setAhora] = useState(() => Date.now());
  // Una clave por reporte: si la red reintenta el mismo envío, el servidor devuelve el primero.
  const llave = useRef(uuid());

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
    if (aviso?.tipo !== 'hecho') return;
    const t = setInterval(() => setAhora(Date.now()), 1000);
    return () => clearInterval(t);
  }, [aviso]);

  const { data: sug } = useQuery({ queryKey: ['se-acabo', 'sugerencias'], queryFn: stockOutApi.suggestions });
  const { data: resultados } = useQuery({
    queryKey: ['se-acabo', 'buscar', buscado], queryFn: () => stockOutApi.search(buscado), enabled: buscado !== '',
  });

  const error = (e: unknown) => setAviso({ tipo: 'error', texto: e instanceof ApiError ? e.message : 'No se pudo avisar; vuelve a intentar.' });
  const reportar = useMutation({
    mutationFn: (it: OutItem) => stockOutApi.report(it, llave.current),
    onSuccess: (res, it) => {
      medirAccion('se-acabo', res.kind === 'confirmation' ? 'confirmar' : 'reportar');
      llave.current = uuid();
      setQ('');
      setAviso({ tipo: 'hecho', nombre: it.name, id: res.id, confirmacion: res.kind === 'confirmation', hasta: Date.parse(res.undoUntil) });
      void qc.invalidateQueries({ queryKey: ['se-acabo'] });
      void qc.invalidateQueries({ queryKey: ['avisos'] });
    },
    onError: error,
  });
  const noLoEncuentro = useMutation({
    mutationFn: (texto: string) => stockOutApi.notFound(texto, llave.current),
    onSuccess: (_, texto) => {
      medirAccion('se-acabo', 'no-lo-encuentro');
      llave.current = uuid();
      setQ('');
      setAviso({ tipo: 'no-lo-encuentro', texto });
      void qc.invalidateQueries({ queryKey: ['se-acabo'] });
    },
    onError: error,
  });
  const deshacer = useMutation({
    mutationFn: (id: number) => stockOutApi.undo(id),
    onSuccess: () => { medirAccion('se-acabo', 'deshacer'); setAviso(null); void qc.invalidateQueries({ queryKey: ['se-acabo'] }); },
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
        variant={listo ? 'solid' : 'outline'} colorPalette={listo ? 'red' : 'gray'} disabled={reportar.isPending}
        onClick={(e) => tocar(clave, e.timeStamp, () => reportar.mutate(it))}>
        <VStack align="start" gap={0} minW={0}>
          <Text fontWeight="700" fontSize="md" lineClamp={1}>{it.name}</Text>
          <Text fontSize="xs" color={listo ? 'white' : 'fg.muted'} minH="16px">
            {listo ? 'Toca otra vez para avisar que se acabó' : detalle ?? ''}
          </Text>
        </VStack>
      </Button>
    );
  };

  const restantes = aviso?.tipo === 'hecho' ? Math.max(0, Math.ceil((aviso.hasta - ahora) / 1000)) : 0;
  const lista = q.trim() ? (
    <VStack align="stretch" gap={2}>
      <Titulo>{resultados ? (resultados.items.length ? `${resultados.items.length} coinciden` : 'Sin coincidencias') : 'Buscando…'}</Titulo>
      {(resultados?.items ?? []).map((it) => mosaico(it, it.openOut ? 'Ya está reportado: suma tu confirmación' : undefined))}
      <Box h="24px" />
      <Button data-row minH="64px" borderStyle="dashed" justifyContent="flex-start" disabled={noLoEncuentro.isPending}
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
            {aviso?.tipo === 'hecho' && (
              <HStack role="status" bg="green.subtle" borderRadius="lg" px={4} minH="64px" gap={3}>
                <LuCircleCheck size={24} />
                <Box flex="1" minW={0}>
                  <Text fontWeight="700">{aviso.confirmacion ? `Ya estaba reportado: sumamos tu confirmación de ${aviso.nombre}` : `Listo: ${aviso.nombre} quedó en 0 y se avisó a quien compra`}</Text>
                  {!aviso.confirmacion && <Text fontSize="sm" color="fg.muted">Le llega en la campana y por correo en unos minutos</Text>}
                </Box>
                {restantes > 0 && (
                  <Button minH="52px" variant="outline" bg="bg" disabled={deshacer.isPending} onClick={() => deshacer.mutate(aviso.id)}>
                    <LuUndo2 /> Deshacer · {restantes} s
                  </Button>
                )}
              </HStack>
            )}
            {aviso?.tipo === 'no-lo-encuentro' && (
              <HStack role="status" bg="orange.subtle" borderRadius="lg" px={4} minH="64px" gap={3}>
                <LuCircleCheck size={24} />
                <Box flex="1"><Text fontWeight="700">Avisamos «{aviso.texto}» a quien compra</Text>
                  <Text fontSize="sm" color="fg.muted">No se tocó ninguna existencia. Quien compra decide si lo da de alta.</Text></Box>
              </HStack>
            )}
            {aviso?.tipo === 'error' && <Text role="alert" color="fg.error" fontWeight="600" px={1}>{aviso.texto}</Text>}
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
