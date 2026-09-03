import { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../components/ui/drawer';
import { Box, Button, HStack, VStack, Text, Input, SimpleGrid, Flex } from '@chakra-ui/react';
import { LuCheck, LuMinus, LuPlus, LuSplit, LuX } from 'react-icons/lu';
import { toaster } from '../components/ui/toaster';
import { posApi } from '../api/pos';
import { ApiError } from '../api/client';
import type { CobroHecho, OrderView, PedidoParaCobrar } from '../types/pos';
import { money } from '../utils/format';
import { TAP_LG, TAP_XL } from '../theme/ui';
import { useUiStore } from '../stores/ui';
import { uuid } from '../utils/uuid';
import { esEfectivo, metodosDeLaLista } from '../domain/metodosDePago';
import {
  billetesUtiles, montoDeLaParte, partesPosibles, partesQueQuedan, presetsDePropina,
  validarCobro, round2,
} from '../domain/cobro';
import type { MotivoInvalido } from '../domain/cobro';

interface Props {
  order: PedidoParaCobrar | null;
  onClose: () => void;
  // Se llama tras CADA cobro que entra, con lo que quedó del pedido. Quien la recibe decide qué
  // hacer: la barra solo refresca; el POS, cuando el pedido queda saldado, lo relee para imprimir
  // el ticket con el PAGADO que dice el servidor.
  onCobrado: (res: CobroHecho, orderId: number) => void;
}

// Traduce el error del servidor a lo que necesita leer quien está cobrando.
//
// Antes salía `String(e)` — el objeto de error crudo — justo en los momentos en que el operador
// tiene el dinero del cliente en la mano y necesita saber qué hacer a continuación.
function loQueLee(e: unknown): { titulo: string; detalle?: string; recargar: boolean } {
  const msg = e instanceof ApiError ? e.message : String(e);
  const codigo = e instanceof ApiError ? e.code : undefined;
  if (/caja/i.test(msg) || codigo === 'NO_OPEN_REGISTER') {
    return { titulo: 'No hay caja abierta', detalle: 'Ábrela y vuelve a cobrar.', recargar: false };
  }
  if (/ya está cobrado|ya está pagado|no puedes cobrar más/i.test(msg)) {
    return { titulo: 'Otra caja acaba de cobrar este pedido', recargar: true };
  }
  if (/ya se registró/i.test(msg)) {
    return {
      titulo: 'Ese cobro ya entró con otro método o monto',
      detalle: 'Revisa el pedido antes de volver a cobrar.',
      recargar: true,
    };
  }
  if (/ya no está activo/i.test(msg)) {
    return {
      titulo: 'Ese método de pago ya no está activo',
      detalle: 'Elige otro, o vuelve a activarlo en Ajustes.',
      recargar: false,
    };
  }
  if (/plataforma/i.test(msg)) {
    return { titulo: 'Con ese método no se puede cobrar este pedido', recargar: false };
  }
  return { titulo: 'No se pudo cobrar', detalle: msg, recargar: true };
}

// Cobra un pedido que se mandó a cocina sin cobrar, entero o por pedazos.
//
// UN PEDAZO A LA VEZ, y de ahí sale toda la forma de la pantalla. Capturar tres pagos y mandarlos de
// un golpe registra dinero que todavía no se recibió: si la terminal declina la tarjeta del segundo
// comensal DESPUÉS de que el servidor acusó, el sistema ya lo dio por cobrado y no hay forma de
// deshacer un pago — no existe endpoint que lo quite y el reembolso es de la cuenta entera, con los
// tres comensales parados enfrente. Cobrando de a uno, el registro coincide con el instante en que
// el dinero está en la mano, y lo que falta lo dice el servidor entre uno y otro.
//
// Lo que se elige aquí es CUÁNTO se cobra ahora; el resto de la hoja es el mismo cobro simple de
// siempre. No hay un "modo dividido" con su propio estado que reconstruir tras una recarga: el
// estado entero es el faltante, y ese vive en el servidor.
export function CobrarSheet({ order, onClose, onCobrado }: Props) {
  const qc = useQueryClient();
  const palette = useUiStore((s) => s.palette);
  const [metodo, setMetodo] = useState<number | null>(null);
  const [recibido, setRecibido] = useState('');
  // null = "todo lo que falta". No es un string vacío ni un efecto que lo rellene: el faltante baja
  // del servidor y cambia con cada pedazo cobrado, así que sembrarlo en el estado obligaría a
  // resincronizarlo, y esa resincronización es de donde salen las dos cifras que divergen.
  const [montoElegido, setMontoElegido] = useState<string | null>(null);
  // En cuántas partes se está repartiendo lo que falta. null = no se está repartiendo, que es el
  // caso de casi todos los pedidos: se cobra todo a una persona y la hoja no gasta un solo píxel
  // en el repartidor. Antes eran cuatro botones fijos —Todo, entre 2, 3 y 4— siempre en pantalla,
  // en una hoja donde el alto es lo que escasea.
  const [partes, setPartes] = useState<number | null>(null);
  const [propina, setPropina] = useState('');
  // Lo que ESTA hoja lleva cobrado, para que el operador vea qué pedazos ya entraron sin tener que
  // acordarse. No se pide al servidor: es la sesión de esta pantalla.
  const [yaCobrado, setYaCobrado] = useState<Array<{ monto: number; metodo: string }>>([]);
  // El último rebote del servidor, EN LA HOJA y no solo en un toast: el toast se va solo, y esto se
  // lee justo cuando el operador tiene el dinero del cliente en la mano y necesita decidir qué hacer.
  const [rebote, setRebote] = useState<{ titulo: string; detalle?: string } | null>(null);
  // La llave de ESTE pedazo, estable mientras el pedazo siga sin cobrarse.
  //
  // Generarla en cada envío rompía justo el caso para el que existe: el cobro entra, la respuesta se
  // pierde en la red, el operador vuelve a tocar, y con llave nueva el servidor lo registra otra vez.
  // Rota SOLO al cobrar con éxito, que es cuando empieza el pedazo siguiente. Si el operador edita
  // el monto o el método antes de reintentar, la llave sigue siendo la misma a propósito: el
  // servidor la sella contra la carga y responde que ese cobro ya entró con otro método, en vez de
  // registrar un segundo pago sobre uno que quizá sí aterrizó.
  const [llave, setLlave] = useState(uuid);

  // El pedido VIVO, no la foto que traía el prop.
  //
  // La hoja recibía el objeto que la lista tenía al abrirla y nunca se actualizaba, así que un
  // pedido que otra caja cobró entretanto seguía diciendo "Falta $500" indefinidamente. Ahora la
  // cifra baja del servidor, y cobrar emite su evento SSE, que invalida esta query.
  const { data: vivo } = useQuery({
    queryKey: ['orders', order?.id],
    queryFn: () => posApi.order(order!.id),
    enabled: order !== null,
  });

  const { data: metodos } = useQuery({
    queryKey: ['payment-methods'],
    queryFn: posApi.paymentMethods,
    enabled: order !== null,
    // Una tableta encendida lleva horas con el catálogo en caché y puede estar ofreciendo un método
    // que el negocio ya desactivó. Al abrir la hoja se vuelve a preguntar.
    refetchOnMount: 'always',
  });

  const falta = vivo ? Number(vivo.outstanding) : Number(order?.outstanding ?? 0);
  const totalDelPedido = vivo ? Number(vivo.total) : Number(order?.total ?? 0);

  // Cobrar completo es el caso de casi todos los pedidos y no puede costar un tap: sin reparto ni
  // monto tecleado, el monto ES lo que falta.
  //
  // El monto tecleado gana sobre el reparto: es más específico. Tocar el repartidor lo borra, para
  // que no queden dos intenciones en pantalla y una ganando en silencio.
  const monto = montoElegido ?? String(montoDeLaParte(falta, partes ?? 1) ?? falta);

  const elegibles = useMemo(
    // Espejo de domain.MetodoCorrespondeALaPlataforma: ofrecer un método que va a rebotar manda al
    // operador a adivinar cuál sirve, con el cliente enfrente.
    () => metodosDeLaLista(metodos?.items ?? [], order?.deliveryPlatformId ?? null),
    [metodos, order?.deliveryPlatformId],
  );
  const elegido = elegibles.find((m) => m.id === metodo);
  const efectivo = esEfectivo(elegido);

  const v = validarCobro({
    monto, metodoId: metodo, propina, recibido, esEfectivo: efectivo,
    falta, totalDelPedido,
  });

  const cobrar = useMutation({
    mutationFn: async () => posApi.chargeOrder(order!.id, {
      methodId: metodo!, amount: v.monto,
      ...(v.propina > 0 ? { tip: v.propina } : {}),
      clientUuid: llave,
    }),
    onSuccess: (res) => {
      setRebote(null);
      if (res.yaEstaba) {
        // El cobro ya estaba registrado: esta llamada no movió dinero, y decirlo evita que el
        // operador crea que cobró dos veces. El pedazo sí está cobrado —entró en el intento cuya
        // respuesta se perdió— así que cuenta en la lista igual que los demás.
        toaster.create({ title: 'Ese cobro ya estaba registrado', type: 'info' });
      }
      setYaCobrado((xs) => [...xs, { monto: v.monto, metodo: elegido?.name ?? '' }]);
      const resta = Number(res.outstanding);
      // La respuesta del cobro entra al MISMO caché del que la hoja lee, no a un estado paralelo.
      //
      // Es la cifra que acaba de calcular el servidor, así que sirve de inmediato mientras el
      // refetch viaja; guardándola aparte habría dos lugares con lo que falta, y ésa es justamente
      // la deuda que dejó a la barra diciendo $2,141 y a su lista $1,928.
      qc.setQueryData(['orders', order!.id], (prev: OrderView | undefined) =>
        (prev ? { ...prev, outstanding: res.outstanding, paid: res.paid } : prev));
      qc.invalidateQueries({ queryKey: ['orders'] });
      onCobrado(res, order!.id);
      if (res.paid || resta <= 0) {
        toaster.create({ title: 'Cobrado', type: 'success' });
        onClose();
        return;
      }
      // Queda saldo: la hoja NO se cierra. Se prepara para el siguiente comensal con lo que falta,
      // y con llave nueva: el pedazo que viene es otro cobro.
      setLlave(uuid());
      setMontoElegido(null);
      // Entró una parte: queda una persona menos. Al llegar a la última se sale del reparto y la
      // hoja vuelve a ofrecer todo lo que falta — que es exactamente lo que esa persona debe, con
      // el residuo de los redondeos ya incluido.
      setPartes((p) => {
        if (p === null) return null;
        const quedan = partesQueQuedan(p);
        return quedan > 1 ? quedan : null;
      });
      // El método NO se hereda del pedazo anterior: cada persona paga con lo suyo, y arrastrarlo
      // registraría con tarjeta dinero que entró en efectivo.
      setMetodo(null);
      setRecibido('');
      setPropina('');
    },
    onError: (e) => {
      const { titulo, detalle, recargar } = loQueLee(e);
      setRebote({ titulo, detalle });
      toaster.create({ title: titulo, description: detalle, type: 'error' });
      // Tras un error la cifra de la pantalla puede estar vieja: se vuelve a preguntar en vez de
      // congelar la última buena.
      if (recargar) qc.invalidateQueries({ queryKey: ['orders'] });
    },
  });

  if (!order) return null;

  const moneda = order.currency;
  const aCubrirEnEfectivo = round2(v.monto + v.propina);
  const billetes = billetesUtiles(aCubrirEnEfectivo);
  // El cambio que sobra se puede dejar como propina de un toque, en vez de que el operador teclee la
  // resta. Sin este gesto, "quédese con el cambio" obligaba a capturar el total recibido como monto
  // y eso rebota con ErrCobroExcede: el operador quedaba atorado con el cliente enfrente.
  const cambioComoPropina = efectivo && v.cambio > 0 && v.propina === 0
    && round2(v.propina + v.cambio) <= totalDelPedido ? v.cambio : 0;

  // Un pedido sin saldo no se cobra, y decirlo importa: la hoja se puede abrir sobre un pedido que
  // otra caja acaba de saldar, y "escribe cuánto vas a cobrar" ahí manda al operador a buscar un
  // problema que no existe.
  const saldado = falta <= 0;

  // `Record<MotivoInvalido, string>` EXHAUSTIVO, y ese tipo es el punto: agregar un motivo de
  // rechazo en `domain/cobro` sin escribir aquí qué lee el operador NO COMPILA. Sin él, un motivo
  // nuevo apagaría el botón sin decir por qué, que es la peor forma de rechazar algo — el operador
  // ve un botón muerto y no tiene ninguna acción que tomar.
  const textos: Record<MotivoInvalido, string> = {
    'sin-monto': 'Escribe cuánto vas a cobrar.',
    'monto-invalido': 'Escribe el monto solo con números y punto, sin comas.',
    'sin-metodo': 'Falta con qué paga.',
    excede: `Es más de lo que falta (${money(String(falta), moneda)}).`,
    'propina-excede': 'La propina no puede ser mayor que la cuenta.',
    'falta-efectivo': `Faltan ${money(String(v.faltaEfectivo), moneda)}.`,
  };
  const aviso = saldado ? 'Este pedido ya está cobrado.' : textos[v.motivo ?? 'sin-monto'];

  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }} size="md">
      <DrawerBackdrop />
      {/* La paleta del negocio, como el resto del POS. Los estados de selección estaban quemados en
          verde: el mismo sistema se veía de un color en una pantalla y de otro en la de al lado, y
          un negocio que cambiaba su color lo veía cambiar en todas menos en la que cobra.
          El verde SÍ se queda en las dos acciones que meten dinero —COBRAR y "el cambio es
          propina"—, que es semántico y es el mismo criterio del botón COBRAR del panel. */}
      <DrawerContent colorPalette={palette} borderTopRadius="2xl"
        maxW={{ base: '100%', lg: '960px' }} mx="auto">
        <DrawerHeader borderBottomWidth="1px" py={3}>
          <HStack justify="space-between" align="start">
            <Box minW={0}>
              <Text fontWeight="800" fontSize="lg" lineClamp={1}>
                {order.folioName || `#${order.number}`}
              </Text>
              <Text fontSize="sm" color="fg.muted">#{order.number}</Text>
            </Box>
            {/* Las DOS cifras. Pintando solo el faltante donde el operador espera el total, un
                pedido de $500 con $300 abonados se veía idéntico a uno de $200. */}
            {/* Dividir vive en el ENCABEZADO, que ya existe, y no en una fila propia: casi siempre
                se cobra a una sola persona, y una fila que no se usa es alto que se le quita a lo
                que sí. Aparece solo cuando hay algo que repartir. */}
            {falta > 0 && partes === null && partesPosibles(falta) > 1 && (
              <Button size="sm" minH="44px" variant="outline" colorPalette="gray" flexShrink={0}
                onClick={() => { setPartes(2); setMontoElegido(null); }}>
                <LuSplit /> Dividir
              </Button>
            )}
            <Box textAlign="right" flexShrink={0}>
              <Text fontSize="xs" color="fg.muted">Total {money(String(totalDelPedido), moneda)}</Text>
              <Text fontWeight="800" fontSize="2xl" lineHeight="1.1">
                Falta {money(String(falta), moneda)}
              </Text>
            </Box>
          </HStack>
        </DrawerHeader>

        <DrawerBody py={3}>
          <VStack align="stretch" gap={3}>
            {yaCobrado.length > 0 && (
              <HStack gap={2} flexWrap="wrap">
                {yaCobrado.map((c, i) => (
                  <HStack key={i} gap={1} px={2} py={1} borderRadius="md" bg="green.subtle" color="green.fg">
                    <LuCheck size={14} />
                    <Text fontSize="sm" fontWeight="600">
                      {money(String(c.monto), moneda)} {c.metodo}
                    </Text>
                  </HStack>
                ))}
              </HStack>
            )}

            {/* El repartidor, SOLO cuando se está repartiendo.
                Cerrado no ocupa nada: el control que lo abre vive en el encabezado, que ya existía.
                Abierto es una fila, y el número de partes es libre —no dos, tres o cuatro— porque
                una mesa de seis es tan común como una de tres. */}
            {partes !== null && (
              <Box borderWidth="1px" borderColor="border" borderRadius="lg" px={3} py={2}>
                <Flex align="center" justify="space-between" gap={2} flexWrap="wrap">
                  <HStack gap={2}>
                    <Button aria-label="Una parte menos" minH={TAP_LG} minW={TAP_LG} variant="outline"
                      colorPalette="gray" disabled={partes <= 2}
                      onClick={() => { setPartes((p) => Math.max(2, (p ?? 2) - 1)); setMontoElegido(null); }}>
                      <LuMinus />
                    </Button>
                    <VStack gap={0} minW="6.5rem">
                      <Text fontSize="2xs" color="fg.muted">Entre</Text>
                      <Text fontWeight="800" fontSize="lg">{partes} personas</Text>
                    </VStack>
                    <Button aria-label="Una parte más" minH={TAP_LG} minW={TAP_LG} variant="outline"
                      colorPalette="gray" disabled={partes >= partesPosibles(falta)}
                      onClick={() => { setPartes((p) => Math.min(partesPosibles(falta), (p ?? 2) + 1)); setMontoElegido(null); }}>
                      <LuPlus />
                    </Button>
                  </HStack>
                  <HStack gap={2}>
                    {/* Teclear un monto sigue disponible para el caso que no es parejo: "yo pago
                        los tacos y ella el refresco". */}
                    <Input w="7rem" minH={TAP_LG} inputMode="decimal" placeholder="Otro monto"
                      aria-label="Otro monto"
                      value={monto} onChange={(e) => setMontoElegido(e.target.value)} />
                    <Button aria-label="Dejar de dividir" minH={TAP_LG} minW={TAP_LG}
                      variant="ghost" colorPalette="gray"
                      onClick={() => { setPartes(null); setMontoElegido(null); }}>
                      <LuX />
                    </Button>
                  </HStack>
                </Flex>
              </Box>
            )}

            <Box>
              <Text fontSize="sm" fontWeight="600" mb={2}>¿Con qué paga?</Text>
              {/* Botones y no un desplegable: son pocos y se tocan con el dedo.
                  Y NINGUNO viene preseleccionado, a propósito: aquí el pedido ya existe y un dedo
                  que va directo a Cobrar registraría con tarjeta dinero que entró en efectivo,
                  descuadrando el corte en los dos métodos a la vez. El tap es la confirmación. */}
              {elegibles.length === 0 ? (
                <Text fontSize="sm" color="fg.muted">
                  Este pedido no tiene métodos de pago configurados. Agrégalos en Ajustes para poder
                  cobrarlo.
                </Text>
              ) : (
                <SimpleGrid columns={{ base: 2, sm: 3 }} gap={2}>
                  {elegibles.map((m) => (
                    <Button key={m.id} minH={TAP_LG} variant={metodo === m.id ? 'solid' : 'outline'}
                      colorPalette={metodo === m.id ? undefined : 'gray'}
                      onClick={() => { setMetodo(m.id); setRecibido(''); }}>
                      {m.name}
                    </Button>
                  ))}
                </SimpleGrid>
              )}
            </Box>

            {/* Propina. El porcentaje es de lo que se cobra AHORA: sobre el total del pedido, un
                "15%" salía siendo 37.5% de la cifra que la pantalla tiene enfrente. */}
            {metodo !== null && v.monto > 0 && (
              <Box>
                <Text fontSize="sm" fontWeight="600" mb={2}>Propina</Text>
                <HStack gap={2} flexWrap="wrap">
                  <Button minH={TAP_LG} variant={propina === '' ? 'solid' : 'outline'}
                    colorPalette={propina === '' ? undefined : 'gray'} onClick={() => setPropina('')}>
                    Sin
                  </Button>
                  {presetsDePropina(v.monto).map((p) => {
                    const on = propina === String(p.monto);
                    return (
                      <Button key={p.etiqueta} minH={TAP_LG}
                        variant={on ? 'solid' : 'outline'} colorPalette={on ? undefined : 'gray'}
                        onClick={() => setPropina(String(p.monto))}>
                        <VStack gap={0}>
                          <Text fontSize="2xs" opacity={0.8}>{p.etiqueta}</Text>
                          <Text fontWeight="700">{money(String(p.monto), moneda)}</Text>
                        </VStack>
                      </Button>
                    );
                  })}
                  <Input w="7rem" minH={TAP_LG} inputMode="decimal" placeholder="Otra"
                    aria-label="Otra propina"
                    value={propina} onChange={(e) => setPropina(e.target.value)} />
                </HStack>
              </Box>
            )}

            {/* Con qué billete paga, solo para efectivo: es lo único que produce cambio. */}
            {efectivo && (
              <Box>
                <Text fontSize="sm" fontWeight="600" mb={2}>¿Con cuánto paga?</Text>
                <HStack gap={2} flexWrap="wrap">
                  <Button minH={TAP_LG} variant={recibido === '' ? 'solid' : 'outline'}
                    colorPalette={recibido === '' ? undefined : 'gray'}
                    onClick={() => setRecibido('')}>
                    Exacto
                  </Button>
                  {billetes.map((b) => (
                    <Button key={b} minH={TAP_LG} variant={recibido === String(b) ? 'solid' : 'outline'}
                      colorPalette={recibido === String(b) ? undefined : 'gray'}
                      onClick={() => setRecibido(String(b))}>
                      {money(String(b), moneda)}
                    </Button>
                  ))}
                  <Input w="7rem" minH={TAP_LG} inputMode="decimal" placeholder="Otro"
                    aria-label="Con cuánto paga"
                    value={recibido} onChange={(e) => setRecibido(e.target.value)} />
                </HStack>
                {v.cambio > 0 && (
                  <Flex mt={2} align="center" justify="space-between" gap={2}>
                    <Text fontWeight="700" fontSize="lg" color="orange.600">
                      Cambio {money(String(v.cambio), moneda)}
                    </Text>
                    {cambioComoPropina > 0 && (
                      <Button minH="44px" size="sm" variant="outline" colorPalette="green"
                        onClick={() => setPropina(String(cambioComoPropina))}>
                        El cambio es propina
                      </Button>
                    )}
                  </Flex>
                )}
              </Box>
            )}
          </VStack>
        </DrawerBody>

        <DrawerFooter borderTopWidth="1px" flexDirection="column" gap={2} alignItems="stretch">
          {rebote && (
            <Box borderWidth="1px" borderColor="red.emphasized" bg="red.subtle" borderRadius="md" px={3} py={2}>
              <Text fontWeight="700" color="red.fg">{rebote.titulo}</Text>
              {rebote.detalle && <Text fontSize="sm" color="fg.muted">{rebote.detalle}</Text>}
            </Box>
          )}
          {!v.ok && aviso && (
            <Text fontSize="sm" color="fg.muted" textAlign="center">{aviso}</Text>
          )}
          {/* Se cobra el MONTO capturado, no lo que entregó el cliente: el excedente es cambio, no
              ingreso. Registrarlo como ingreso inflaría la venta y descuadraría el corte. */}
          {saldado ? (
            <Button w="100%" size="lg" minH={TAP_XL} variant="outline" colorPalette="gray" onClick={onClose}>
              Cerrar
            </Button>
          ) : (
            <Button w="100%" size="lg" minH={TAP_XL} colorPalette="green"
              disabled={!v.ok} loading={cobrar.isPending}
              onClick={() => cobrar.mutate()}>
              Cobrar {money(String(round2(v.monto + v.propina)), moneda)}
            </Button>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
