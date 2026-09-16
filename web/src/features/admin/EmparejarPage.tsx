import { useCallback, useEffect, useMemo, useState } from 'react';
import { Badge, Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuCheck, LuSearch, LuUndo2 } from 'react-icons/lu';
import { Picker, type PickerOption } from '../../components/Picker';
import { money } from '../../utils/format';
import {
  borrarPareja,
  emparejamiento,
  guardarPareja,
  type ClaseDeItem,
  type Emparejamiento,
  type ItemDePlataforma,
} from '../../api/plataformas';

/**
 * Emparejar lo publicado en la plataforma con el catálogo del POS.
 *
 * UNA DECISIÓN A LA VEZ, A ANCHO COMPLETO, y no dos listas lado a lado. A 1024 px el contenido
 * útil son ~900, y dos columnas dejan ~418 px cada una: con los nombres reales, «Chamoyada de
 * Mango», «de Mora» y «de Maracuyá» se truncan las tres a «Chamoyada de M…», que en una pantalla
 * cuyo único trabajo es distinguir platillos la inutiliza.
 *
 * Y el buscador no se diseña aquí: el `Picker` ya abre una hoja con filas grandes y filtro. Sin él,
 * encontrar un producto entre 174 son hasta 19 pantallas de scroll por cada pareja manual, y son
 * decenas.
 */

interface Props {
  conexionId: number;
  nivel?: ClaseDeItem;
  onListo?: () => void;
}

// El precio llega en centavos, como lo entrega la plataforma; el formateo va por el único
// formateador de dinero del front.
const pesos = (centavos: number) => money(centavos / 100);

export function EmparejarPage({ conexionId, nivel = 'platillo', onListo }: Props) {
  const [datos, setDatos] = useState<Emparejamiento | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [guardando, setGuardando] = useState(false);

  const cargar = useCallback(() => {
    emparejamiento(conexionId, nivel)
      .then((d) => {
        setDatos(d);
        setError(null);
      })
      .catch(() => setError('No se pudo abrir el emparejamiento.'));
  }, [conexionId, nivel]);

  useEffect(cargar, [cargar]);

  // Lo que alguien dejó para después. Vive solo en esta sesión y a propósito: saltar no es una
  // decisión que valga la pena guardar, y al volver a entrar conviene que lo vuelva a preguntar.
  const [saltados, setSaltados] = useState<string[]>([]);

  // El siguiente pendiente: sin pareja, o con una propuesta que nadie ha confirmado. Lo ya
  // confirmado no vuelve a preguntarse, y lo saltado va al final en vez de bloquear la fila.
  const pendientes = useMemo(() => {
    const sin = datos?.items.filter((i) => !i.link?.confirmed) ?? [];
    return [...sin.filter((i) => !saltados.includes(i.externalId)),
            ...sin.filter((i) => saltados.includes(i.externalId))];
  }, [datos, saltados]);
  const resueltos = (datos?.items.length ?? 0) - pendientes.length;
  const actual: ItemDePlataforma | undefined = pendientes[0];

  // La ÚLTIMA confirmada de verdad, por su marca de tiempo. `find()` sobre la lista devolvía la
  // primera ALFABÉTICA: el operador se equivocaba en la #40, tocaba deshacer, y el sistema borraba
  // en silencio la #2 — un error nuevo en vez de la corrección que pidió.
  const ultimaConfirmada = useMemo(() => {
    const confirmadas = (datos?.items ?? []).filter((i) => i.link?.confirmed && i.link.confirmedAt);
    if (confirmadas.length === 0) return undefined;
    return confirmadas.reduce((a, b) =>
      (a.link!.confirmedAt ?? '') >= (b.link!.confirmedAt ?? '') ? a : b,
    );
  }, [datos]);

  const opciones: PickerOption[] = useMemo(
    () => (datos?.unlinkedLocal ?? []).map((p) => ({ value: String(p.id), label: p.name })),
    [datos],
  );

  const confirmar = async (localId: number) => {
    if (!actual || guardando) return;
    setGuardando(true);
    try {
      await guardarPareja(conexionId, actual.externalId, {
        localId,
        localKind: nivel === 'opcion' ? 'opcion_de_modificador' : 'producto',
        kind: nivel,
      });
      cargar(); // avanza sola al siguiente pendiente: un tap de «Siguiente» sobra
    } catch {
      setError('No se pudo guardar la pareja.');
    } finally {
      setGuardando(false);
    }
  };

  const deshacer = async (externalId: string) => {
    try {
      await borrarPareja(conexionId, externalId);
      cargar();
    } catch {
      setError('No se pudo deshacer.');
    }
  };

  if (error) return <Text color="red.600">{error}</Text>;
  if (!datos) return <Text color="fg.muted">Cargando…</Text>;

  if (!actual) {
    return (
      <VStack align="stretch" gap={4}>
        <Text fontWeight="semibold">Ya no queda nada por emparejar.</Text>
        <Text color="fg.muted">
          {resueltos} de {datos.items.length} resueltos.
        </Text>
        {onListo && (
          <Button onClick={onListo} minH="44px" alignSelf="flex-start">
            Ver las diferencias
          </Button>
        )}
      </VStack>
    );
  }

  return (
    <VStack align="stretch" gap={4}>
      <HStack justify="space-between">
        <Text fontWeight="semibold">
          {resueltos} de {datos.items.length} resueltos
        </Text>
        <Text color="fg.muted" fontSize="sm">
          {pendientes.length} por resolver
        </Text>
      </HStack>

      {/* El nombre a DOS LÍNEAS y nunca truncado: es lo único que distingue un platillo de otro. */}
      <Box borderWidth="1px" borderRadius="md" p={4}>
        <Text fontSize="lg" fontWeight="bold" lineClamp={2}>
          {actual.name}
        </Text>
        <HStack gap={3} mt={1}>
          <Text color="fg.muted">{pesos(actual.priceCents)}</Text>
          {!actual.available && <Badge colorPalette="orange">Agotado en la app</Badge>}
        </HStack>
      </Box>

      {actual.link ? (
        <VStack align="stretch" gap={2}>
          <HStack>
            <Text color="fg.muted">Se parece a</Text>
            {/* FR-011: una propuesta se ve DISTINTA de un hecho. Aceptada en silencio produce
                comparaciones falsas que nadie puede auditar. */}
            <Badge colorPalette="yellow">Propuesta</Badge>
          </HStack>
          <Text fontWeight="semibold">{actual.link.localName}</Text>
          <HStack gap={2}>
            <Button minH="44px" onClick={() => confirmar(actual.link!.localId)} loading={guardando}>
              <LuCheck /> Es el mismo
            </Button>
            <Picker
              value=""
              options={opciones}
              onChange={(v) => confirmar(Number(v))}
              placeholder="Buscar otro…"
              title="¿Con cuál del POS?"
              size="lg"
            />
            <Button variant="ghost" minH="44px" onClick={() => setSaltados((s) => [...s, actual.externalId])}>
              Más tarde
            </Button>
          </HStack>
        </VStack>
      ) : (
        <VStack align="stretch" gap={2}>
          <Text color="fg.muted">
            <LuSearch style={{ display: 'inline' }} /> No se parece a ninguno del POS.
          </Text>
          <Picker
            value=""
            options={opciones}
            onChange={(v) => confirmar(Number(v))}
            placeholder="Buscar en el POS…"
            title="¿Con cuál del POS?"
            size="lg"
          />
          {/* SIN ESTO LA SESIÓN SE ATORA. Con 65 platillos y 109 productos sin pareja es seguro
              toparse con uno que no corresponde a nada, y sin salida el operador solo puede
              confirmar algo que sabe que está mal o abandonar las 64 decisiones que faltan. */}
          <Button
            variant="ghost"
            minH="44px"
            alignSelf="flex-start"
            onClick={() => setSaltados((s) => [...s, actual.externalId])}
          >
            No corresponde a ninguno, más tarde
          </Button>
        </VStack>
      )}

      {ultimaConfirmada && (
        <Button
          variant="ghost"
          minH="44px"
          alignSelf="flex-start"
          onClick={() => deshacer(ultimaConfirmada.externalId)}
        >
          <LuUndo2 /> Deshacer «{ultimaConfirmada.name}»
        </Button>
      )}
    </VStack>
  );
}
