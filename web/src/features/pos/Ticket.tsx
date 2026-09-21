import {
  Box, Flex, HStack, VStack, Text, Button, IconButton, Separator, Center, Input,
} from '@chakra-ui/react';
import { useState } from 'react';
import {
  LuTrash2, LuStickyNote, LuPanelRightClose, LuStore, LuBike, LuTag, LuEllipsisVertical, LuUser,
} from 'react-icons/lu';
import { MenuRoot, MenuTrigger, MenuContent, MenuItem, MenuSeparator } from '../../components/ui/menu';
import { useTicketStore, useActiveTicket, lineTotal, ticketTotal } from '../../stores/ticket';
import { envioDeLaCuenta } from '../../domain/envio';
import { descuentoDeLaCuenta, totalConDescuento } from '../../domain/descuento';
import type { TicketLine } from '../../types/pos';
import type { SwipeHandlers } from '../../hooks/useSwipeDownToClose';
import { money } from '../../utils/format';

// Los dos destinos de una cuenta. Se ofrecen aquí y no dentro de la hoja de cobro porque son
// decisiones sobre el PEDIDO, no sobre el dinero: mandar a cocina no cobra nada, y tenerlo junto al
// método de pago hacía que la pantalla pidiera propina para algo que podía no cobrarse.
interface Props {
  onCheckout: () => void;
  // Costo de envío de ESTE pedido, y el default del negocio. Viven en el panel y no en una pantalla
  // de cobro porque son atributos del pedido que se está armando: el operador los decide mientras
  // toma la orden, no cuando cuenta el dinero. Vacío = el default del negocio.
  envioPorDefecto: number;
  // Renglones que el servidor va a rechazar porque el producto se inactivó mientras estaba en el
  // carrito. Se avisa AQUÍ, mientras se puede quitar, y no al cobrar con el cliente enfrente.
  noDisponibles: TicketLine[];
  // Manda a cocina sin cobrar. Queda por cobrar y el tablero lo marca.
  onEnviar: () => void;
  enviando?: boolean;
  onEditLine: (line: TicketLine) => void;
  onHide?: () => void; // ocultar el panel lateral (solo modo ancho)
  // solo cuando Ticket vive dentro del bottom sheet (modo angosto): arrastrar el header
  // hacia abajo también lo cierra, además del botón "Ocultar pedido".
  swipeHandlers?: SwipeHandlers;
}

// para_llevar salió del selector: no cambiaba nada y ningún reporte agrupaba por él. Y los dos que
// quedan ya no son dos botones sino uno que alterna: para dos opciones excluyentes, mostrar la
// activa y cambiarla de un toque cuesta la mitad del ancho, que es el recurso escaso del encabezado.

export function Ticket({
  onCheckout, onEnviar, enviando, onEditLine, onHide, swipeHandlers,
  envioPorDefecto, noDisponibles,
}: Props) {
  const {
    lines, customerName, folioName, serviceType, platformId, envio, descuento, descuentoModo,
  } = useActiveTicket();
  const setServiceType = useTicketStore((s) => s.setServiceType);
  const setCustomerName = useTicketStore((s) => s.setCustomerName);
  const setEnvio = useTicketStore((s) => s.setEnvio);
  const setDescuento = useTicketStore((s) => s.setDescuento);
  const setDescuentoModo = useTicketStore((s) => s.setDescuentoModo);
  const inc = useTicketStore((s) => s.incrementLine);
  const dec = useTicketStore((s) => s.decrementLine);
  const remove = useTicketStore((s) => s.removeLine);
  const clear = useTicketStore((s) => s.clearActive);
  const total = ticketTotal(lines);
  // La MISMA función que usan la píldora y la barra angosta. Cada superficie la llama con la misma
  // cuenta; lo que no puede haber es dos implementaciones, que es como el panel acabó apagando sus
  // botones por un envío que la píldora cobraba al default.
  const envioDelPedido = envioDeLaCuenta({ serviceType, platformId }, envio, envioPorDefecto);
  const llevaEnvio = envioDelPedido.aplica;
  const envioMalEscrito = envioDelPedido.malEscrito;
  const descuentoDelPedido = descuentoDeLaCuenta(descuento, descuentoModo, total);
  const descuentoMalCapturado = descuentoDelPedido.malEscrito || descuentoDelPedido.excede;
  // Qué se está capturando ahora mismo, elegido desde el menú. Uno a la vez: dos campos abiertos le
  // quitarían a la lista el alto que este reacomodo vino a devolverle.
  const [capturando, setCapturando] = useState<null | 'cliente' | 'descuento'>(null);
  // Un descuento imposible ABRE su campo solo, aunque nadie lo haya pedido desde el menú.
  //
  // Puede llegar así desde una cuenta guardada: el texto tecleado vive en `egb:ticket:v2`, así que
  // una cuenta abierta con "1,000" en el campo sobrevive a un F5. Sin esto, los botones de abajo
  // aparecen apagados y el aviso que explica por qué está escondido en un menú — el operador se
  // queda sin poder cobrar y sin nada que leer.
  const campoAbierto = capturando !== null || descuentoMalCapturado;
  const queSeCaptura = descuentoMalCapturado ? 'descuento' : capturando;
  const totalDelPedido = totalConDescuento(total, descuentoDelPedido.monto, envioDelPedido.monto);

  return (
    <Flex direction="column" h="100%" bg="bg.panel">
      {/* ARRIBA LO QUE DESCRIBE AL PEDIDO, ABAJO LO QUE MUEVE DINERO.
          Es la regla del reacomodo (spec 023) y lo que hay que poder repetir dentro de seis meses:
          ningún control secundario vive ya junto a COBRAR.

          La fila de tipo + cliente que estaba abajo costaba 52 px en TODO pedido de mostrador —el
          de todos los días— para servir a dos datos que este negocio casi no usa, y que siguen
          existiendo para el negocio que sí. */}
      <HStack p={3} pb={2} gap={2}
        style={swipeHandlers ? { touchAction: 'none' } : undefined} {...swipeHandlers}>
        {onHide && (
          <IconButton size="lg" minW="48px" minH="48px" variant="ghost" colorPalette="gray"
            aria-label="Ocultar pedido" onClick={onHide}>
            <LuPanelRightClose />
          </IconButton>
        )}
        {/* Sin la palabra "Pedido": con 304 px útiles y nombres de hasta 20 caracteres
            ("Colorpoint Shorthair" — el esquema de folio POR OMISIÓN es `razas`, no animales), esa
            palabra se comía el nombre. Lo que cede es el ancho del nombre, que trunca, y NUNCA la
            altura de un control: un botón de 36 px se deja de acertar; un nombre cortado se sigue
            leyendo, y completo está en la barra de cuentas, en el ticket y en la comanda. */}
        <Text fontWeight="700" fontSize="lg" truncate flex="1" minW={0}>
          {folioName}
          {customerName && <Text as="span" color="fg.muted" fontWeight="500"> · {customerName}</Text>}
        </Text>
        {/* Un pedido de plataforma ES a domicilio: el check de la tabla lo exige, así que ofrecer el
            cambio sería ofrecer algo que el servidor rechaza. */}
        {platformId === null && (
          <Button size="sm" minH="44px" px={2.5} flexShrink={0} variant="outline" colorPalette="gray"
            onClick={() => setServiceType(serviceType === 'mostrador' ? 'domicilio' : 'mostrador')}>
            {serviceType === 'mostrador' ? <LuStore /> : <LuBike />}
            {serviceType === 'mostrador' ? 'Mostrador' : 'Domicilio'}
          </Button>
        )}
        <MenuRoot>
          <MenuTrigger asChild>
            <IconButton aria-label="Más opciones del pedido" size="sm" minW="44px" minH="44px"
              variant="ghost" colorPalette="gray" flexShrink={0}>
              <LuEllipsisVertical />
            </IconButton>
          </MenuTrigger>
          {/* Renglones de 48 px: aquí sí hay espacio, y un renglón de menú se acierta peor que un
              botón suelto porque están apilados uno sobre otro. */}
          <MenuContent>
            <MenuItem value="cliente" minH="48px" onClick={() => setCapturando('cliente')}>
              <LuUser /> Nombre del cliente
            </MenuItem>
            <MenuItem value="descuento" minH="48px" onClick={() => setCapturando('descuento')}>
              <LuTag /> Descuento
            </MenuItem>
            {lines.length > 0 && (
              <>
                <MenuSeparator />
                {/* Estar dentro de un menú no lo vuelve inofensivo: sigue preguntando. */}
                <MenuItem value="vaciar" minH="48px" color="red.fg"
                  onClick={() => { if (confirm('¿Vaciar pedido?')) clear(); }}>
                  <LuTrash2 /> Vaciar el pedido
                </MenuItem>
              </>
            )}
          </MenuContent>
        </MenuRoot>
      </HStack>

      {/* El campo que abre el menú vive AQUÍ, fuera de la caja de totales, y es temporal.
          Fuera de ella porque esa caja tiene el alto acotado que impide que COBRAR se vaya de la
          pantalla: el aviso de "ya no están en el menú" ya mandó el botón a un scroll interno por
          vivir adentro. Aquí lo único que se encoge es la lista, que ya tiene su scroll, y el pie
          queda anclado — los botones no se mueven bajo el dedo. */}
      {campoAbierto && (
        <HStack px={3} pb={2} gap={2} flexShrink={0}>
          {queSeCaptura === 'cliente' ? (
            <Input flex="1" minW={0} minH="44px" autoFocus aria-label="Nombre del cliente"
              placeholder="Nombre del cliente" value={customerName}
              onChange={(e) => setCustomerName(e.target.value)} />
          ) : (
            <>
              <HStack gap={1} flexShrink={0}>
                {/* Dos botones y no un Picker: para dos opciones excluyentes una hoja inferior son
                    dos toques donde basta uno. Y nunca un <select> nativo. */}
                <Button size="sm" minH="44px" minW="44px" px={3} aria-label="Descuento en pesos"
                  aria-pressed={descuentoModo === 'monto'}
                  variant={descuentoModo === 'monto' ? 'solid' : 'outline'}
                  colorPalette={descuentoModo === 'monto' ? undefined : 'gray'}
                  onClick={() => setDescuentoModo('monto')}>$</Button>
                <Button size="sm" minH="44px" minW="44px" px={3} aria-label="Descuento en porcentaje"
                  aria-pressed={descuentoModo === 'pct'}
                  variant={descuentoModo === 'pct' ? 'solid' : 'outline'}
                  colorPalette={descuentoModo === 'pct' ? undefined : 'gray'}
                  onClick={() => setDescuentoModo('pct')}>%</Button>
              </HStack>
              <Input flex="1" minW={0} minH="44px" inputMode="decimal" autoFocus aria-label="Descuento"
                placeholder={descuentoModo === 'pct' ? '0 %' : money(0)}
                value={descuento} onChange={(e) => setDescuento(e.target.value)} />
              {/* Los dos avisos viajan CON el campo. Sin ellos, un descuento mal escrito apaga los
                  botones de abajo y el operador no tiene cómo saber por qué. */}
              {descuentoDelPedido.malEscrito && (
                <Text fontSize="xs" color="red.fg" flexShrink={0}>Solo números</Text>
              )}
              {descuentoDelPedido.excede && (
                <Text fontSize="xs" color="red.fg" flexShrink={0}>
                  Máx {descuentoModo === 'pct' ? '100 %' : money(total)}
                </Text>
              )}
            </>
          )}
          {/* «Listo» no cierra con un descuento imposible. Cerrarlo dejaba el aviso fuera de la
              pantalla y los botones de abajo apagados sin nada que explicara por qué — y la única
              salida era adivinar que había que volver a abrir el menú. */}
          <Button size="sm" minH="44px" px={3} variant="ghost" colorPalette="gray" flexShrink={0}
            disabled={descuentoMalCapturado}
            onClick={() => setCapturando(null)}>Listo</Button>
        </HStack>
      )}
      <Separator />

      <VStack align="stretch" gap={0} flex="1" overflowY="auto" px={2} py={2}>
        {lines.length === 0 && (
          <Center h="100%" px={6}>
            <Text color="fg.subtle" textAlign="center">Toca un producto para agregarlo</Text>
          </Center>
        )}
        {lines.map((l) => (
          <Box key={l.lineId} py={1.5} px={2} borderBottomWidth="1px" borderColor="border.muted">
            <Flex justify="space-between" gap={2}>
              <Box flex="1" onClick={() => onEditLine(l)} cursor="pointer">
                <Text fontWeight="600" fontSize="sm">{l.name}</Text>
                {l.modifiers.length > 0 && (
                  <Text fontSize="xs" color="fg.muted" lineClamp={2}>
                    {l.modifiers
                      .map((m) => (m.qty > 1 ? `${m.name} ×${m.qty}` : m.name))
                      .join(' · ')}
                  </Text>
                )}
                {l.notes && (
                  <HStack gap={1} color="orange.500">
                    <LuStickyNote size={12} />
                    <Text fontSize="xs">{l.notes}</Text>
                  </HStack>
                )}
              </Box>
              <Text fontWeight="600" fontSize="sm" whiteSpace="nowrap">{money(lineTotal(l))}</Text>
            </Flex>
            {/* Los controles del renglón medían ~24 px, por debajo del piso de 44 que la
                constitución fija: por ahí el dedo toca dos veces y la segunda cae en otra cosa —
                y aquí "otra cosa" era la papelera, que estaba pegada al menos.

                La papelera se va al EXTREMO OPUESTO. Separar la acción destructiva de la frecuente
                es requisito funcional, no gusto: quitar un renglón por accidente con el cliente
                enfrente obliga a volver a buscarlo en el menú.

                Cuesta ~14 px de alto por renglón (los 20 que suben los controles, menos los 6 que
                se recuperan apretando el relleno). En el panel de 600 px eso es un renglón menos a
                la vista, y se paga: un control que no se puede tocar no sirve de nada. */}
            <HStack mt={0.5} gap={1} justify="space-between">
              <HStack gap={1}>
                <Button size="sm" minW="44px" minH="44px" onClick={() => dec(l.lineId)}>−</Button>
                <Text minW="32px" textAlign="center" fontSize="sm" fontWeight="600">{l.qty}</Text>
                <Button size="sm" minW="44px" minH="44px" onClick={() => inc(l.lineId)}>+</Button>
              </HStack>
              <IconButton aria-label="Quitar" size="sm" minW="44px" minH="44px" variant="ghost"
                colorPalette="red" onClick={() => remove(l.lineId)}><LuTrash2 /></IconButton>
            </HStack>
          </Box>
        ))}
      </VStack>

      <Separator />
      {/* Lo que el servidor ya no acepta. Se dice mientras la cuenta se está armando y se puede
          quitar de un toque; enterarse al cobrar deja al operador resolviéndolo con el cliente
          enfrente.

          VIVE FUERA del bloque de totales, y no es cosmético: ese bloque tiene el alto acotado que
          le impide empujar a COBRAR fuera de la pantalla, y este aviso mide ~120 px. Adentro, un
          domicilio con un producto dado de baja y el descuento abierto llenaba el techo y mandaba
          el propio botón COBRAR a un scroll interno. */}
      {noDisponibles.length > 0 && (
        <Box colorPalette="orange" borderWidth="1px" borderColor="colorPalette.emphasized"
          bg="colorPalette.subtle" borderRadius="lg" p={2} mx={3} mb={2} flexShrink={0}>
          <Text fontWeight="700" fontSize="sm" color="colorPalette.fg">
            Ya no están en el menú
          </Text>
          <Text fontSize="xs" color="fg.muted" mb={2}>
            {noDisponibles.map((l) => l.name).join(', ')}
          </Text>
          <Button size="sm" minH="44px" variant="outline" colorPalette="orange"
            onClick={() => noDisponibles.forEach((l) => remove(l.lineId))}>
            Quitar del pedido
          </Button>
        </Box>
      )}

      {/* maxH en dvh y no en px: lo que se reparte es el alto de la tableta. Sin un alto, un
          `overflowY` no hace scroll —la caja crece— y con el teclado numérico abierto el botón
          COBRAR se va abajo de la pantalla sin forma de alcanzarlo. */}
      <Box p={3} maxH="60dvh" overflowY="auto" flexShrink={0}>
        {/* Sin un solo control tocable: la fila dice cuánto es y de dónde sale, nada más. Lo que
            se captura vive arriba, en el menú. */}
        <Flex justify="space-between" align="center" mb={2}>
          <Text fontSize="lg" fontWeight="600">Total</Text>
          <VStack gap={0} align="end">
            {/* El renglón chico existe para que el operador pueda decirle al cliente de dónde sale
                el total: un total rebajado sin decir por qué se discute en el mostrador. Y es lo
                que hace que esconder el CAMPO del descuento en un menú no esconda el descuento. */}
            {descuentoDelPedido.monto > 0 && (
              <Text fontSize="xs" color="fg.muted">
                {money(total)} − {money(descuentoDelPedido.monto)} de descuento
              </Text>
            )}
            <Text fontSize="2xl" fontWeight="800">{money(totalDelPedido)}</Text>
          </VStack>
        </Flex>

        {/* El envío solo cuando el pedido lo cobra el negocio. Con plataforma no aparece: lo cobra
            ella, y la regla la contesta `cobraEnvio` en vez de deducirla aquí — deducirla fue como
            la pantalla llegó a ofrecer un envío que el servidor no cobra. */}
        {llevaEnvio && (
          <HStack gap={2} mb={2}>
            <Text fontSize="sm" color="fg.muted" flexShrink={0}>Envío</Text>
            <Input flex="1" minH="44px" inputMode="decimal" aria-label="Costo de envío"
              placeholder={money(envioPorDefecto)}
              value={envio} onChange={(e) => setEnvio(e.target.value)} />
            {envioMalEscrito && (
              <Text fontSize="xs" color="red.fg">Solo números</Text>
            )}
          </HStack>
        )}

        <HStack gap={2}>
          {/* Enviar es secundario y COBRAR domina: cobrar es lo que pasa en casi toda venta, y dos
              botones con el mismo peso invitan al toque equivocado — que aquí significa creer que
              se cobró algo que no se cobró. */}
          {/* Contorno azul tenue y no gris: en gris se leía como un texto apagado y no como un
              control, y quien no lo reconoce como botón termina cobrando para mandar a cocina. El
              azul lo separa además del verde de COBRAR, que es el que mueve dinero. */}
          <Button flex="1" size="lg" h="56px" variant="outline" colorPalette="blue"
            disabled={lines.length === 0 || envioMalEscrito || descuentoMalCapturado}
            loading={enviando} onClick={onEnviar}>
            Enviar a cocina
          </Button>
          {/* COBRAR ya NO manda el pedido a cocina: abre la hoja y el pedido nace al tocar el botón
              final. Este comentario decía lo contrario y era cierto hasta ese cambio; cerrar la
              hoja sin cobrar ahora no deja nada preparándose. */}
          {/* `loading` también aquí: es el botón que más se toca y era el único de los dos sin
              estado ocupado. En red lenta el operador toca dos veces y salen dos POST /orders
              concurrentes con el mismo clientUuid; la idempotencia de Create es check-then-insert
              sin lock, así que la segunda choca contra el índice único y sale un 500 sobre un
              pedido que SÍ se creó — con la cuenta ya cerrada, recapturar manda dos veces a cocina. */}
          <Button flex="1.3" size="lg" h="56px" colorPalette="green" fontWeight="800"
            aria-describedby="cobrar-manda-a-cocina" loading={enviando}
            disabled={lines.length === 0 || envioMalEscrito || descuentoMalCapturado} onClick={onCheckout}>
            COBRAR
          </Button>
        </HStack>
        <Text id="cobrar-manda-a-cocina" fontSize="2xs" color="fg.muted" textAlign="right" mt={1}>
          Cobrar también manda el pedido a cocina
        </Text>
      </Box>
    </Flex>
  );
}
