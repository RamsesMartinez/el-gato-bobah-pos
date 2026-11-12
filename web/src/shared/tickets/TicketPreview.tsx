import { useRef } from 'react';
import { Box, Button, Center, HStack, Spinner } from '@chakra-ui/react';
import { LuPrinter, LuX } from 'react-icons/lu';

import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogFooter } from '../../components/ui/dialog';
import { buildReceiptHtml, printFrame, RECEIPT_DOCUMENT_PX } from '../../utils/printReceipt';
import { useContainerWidth } from '../../hooks/useContainerWidth';
import { useTicketBusinessInfo } from './ticketBusinessInfo';
import type { ReceiptOrder, PaymentView } from '../../types/pos';

// El ancho del DOCUMENTO del ticket, no el del papel: el cuerpo mide 80mm más su relleno. Con los
// 80mm solos (302px) el marco recortaba la columna de importes y la cifra del TOTAL (#233). El papel
// recibe ese mismo documento encogido a su ancho, así que escalarlo aquí es verlo como sale.
//
// Más el ancho de una barra de desplazamiento clásica: el ticket siempre es más alto que el marco y
// en un navegador que no la superpone (Chrome de escritorio en Windows: 17px; Linux: 15px) la barra
// sale del ancho del documento y vuelve a recortar los importes. Donde la barra se superpone
// (Android) solo queda un margen blanco a la derecha.
const SCROLLBAR_PX = 17;
const TICKET_PX = RECEIPT_DOCUMENT_PX + SCROLLBAR_PX;

// TicketPreviewDialog es la parte presentacional: recibe el documento ya armado. Se exporta para
// poder probar el cableado sin montar react-query.
export function TicketPreviewDialog({
  html,
  isOpen,
  onClose,
}: {
  html: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const frame = useRef<HTMLIFrameElement>(null);
  // Se mide el hueco disponible para ESCALAR el ticket, no para encogerlo. Comprimir el iframe por
  // debajo del ancho del documento le mete scroll horizontal, y en una tablet arrastrar esa barra
  // cuenta como toque fuera del diálogo: se cierra solo.
  const { ref: slot, width } = useContainerWidth<HTMLDivElement>();
  const scale = width > 0 ? Math.min(1, width / TICKET_PX) : 1;
  const frameHeight = Math.round(window.innerHeight * 0.48);

  return (
    <DialogRoot
      open={isOpen}
      onOpenChange={(e) => { if (!e.open) onClose(); }}
      placement="center"
      size="sm"
      // El ticket se cierra con su botón, no tocando fuera: en una tablet un roce al desplazarse
      // lo cerraba a media revisión.
      closeOnInteractOutside={false}
    >
      <DialogBackdrop />
      <DialogContent mx={3} borderRadius="2xl">
        <DialogBody px={3} pt={4} pb={2}>
          {html ? (
            <Box ref={slot} w="100%" overflow="hidden">
              <Box
                bg="white" borderWidth="1px" borderRadius="md" overflow="hidden" mx="auto"
                w={`${TICKET_PX}px`} h={`${frameHeight}px`}
                // El transform es SOLO de pantalla: la impresión usa el layout propio del documento
                // del iframe y su @page, así que escalar la vista no encoge el papel.
                transform={`scale(${scale})`} transformOrigin="top center"
                mb={`${Math.round(-frameHeight * (1 - scale))}px`}
              >
                <iframe
                  ref={frame}
                  srcDoc={html}
                  title="Vista previa del ticket"
                  // Sin allow-scripts: aunque el escape del contenido ya está probado, el sandbox es
                  // un segundo candado que no depende de que ese escape sea perfecto. allow-modals
                  // es lo que deja pasar el print(); allow-same-origin, lo que deja al padre
                  // alcanzar contentWindow para dispararlo.
                  sandbox="allow-same-origin allow-modals"
                  style={{ width: `${TICKET_PX}px`, height: `${frameHeight}px`, border: 0, display: 'block', background: '#fff' }}
                />
              </Box>
            </Box>
          ) : (
            <Center py={12}><Spinner /></Center>
          )}
        </DialogBody>
        <DialogFooter px={3} pb={4} pt={2}>
          {/* Dos acciones, ambas de dedo: el target son tablets de 7" y no hay scroll para llegar
              al botón de imprimir. Cerrar va a la izquierda para que el pulgar no lo alcance por
              accidente cuando busca Imprimir.

              El `minH` es explícito y no se hereda de `size="lg"`. Hoy ese tamaño ya da 44 px
              —medido a 1024×600 contra el ambiente desplegado—, así que no arregla nada: fija el
              piso para que un cambio de la receta de Chakra no lo baje sin que nadie se entere. La
              intención llevaba en este comentario desde que se escribió y no estaba medida. */}
          <HStack w="100%" gap={2}>
            <Button variant="outline" size="lg" minH="44px" flex="1" onClick={onClose}>
              <LuX /> Cerrar
            </Button>
            <Button size="lg" minH="44px" flex="2" disabled={!html} onClick={() => printFrame(frame.current)}>
              <LuPrinter /> Imprimir
            </Button>
          </HStack>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

// TicketPreview resuelve el encabezado del negocio y arma el documento. El mismo componente sirve
// para el ticket recién cerrado y para una reimpresión; lo único que cambia es la marca del papel.
export function TicketPreview({
  order,
  reprint = false,
  sample = false,
  preCuenta = false,
  payment,
  isOpen,
  onClose,
}: {
  order: (ReceiptOrder & { outstanding?: string }) | null;
  // payment: el ticket de UN pago de una cuenta dividida, solo con lo que cubrió.
  payment?: PaymentView;
  reprint?: boolean;
  sample?: boolean;
  // preCuenta: el papel de una cuenta que todavía NO es una venta. Lleva su marca, no lleva número
  // de pedido —no existe— y no lleva el estado del cobro, que es lo que lo confundiría con el
  // ticket de un pedido real sin cobrar.
  preCuenta?: boolean;
  isOpen: boolean;
  onClose: () => void;
}) {
  const { data: business, printFreeModifiers } = useTicketBusinessInfo();
  // html vacío = el diálogo abre con un spinner en vez de no abrir. Un toque que no hace nada es
  // exactamente el fallo silencioso que esta feature vino a quitar.
  const html = order && business ? buildReceiptHtml(order, business, { reprint, sample, preCuenta, printFreeModifiers, payment }) : '';
  return <TicketPreviewDialog html={html} isOpen={isOpen} onClose={onClose} />;
}
