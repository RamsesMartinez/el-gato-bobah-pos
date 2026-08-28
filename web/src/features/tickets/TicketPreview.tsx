import { useRef } from 'react';
import { Box, Button, Center, HStack, Spinner } from '@chakra-ui/react';
import { LuPrinter, LuX } from 'react-icons/lu';

import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogFooter } from '../../components/ui/dialog';
import { buildReceiptHtml, printFrame } from '../../utils/printReceipt';
import { useContainerWidth } from '../../hooks/useContainerWidth';
import { useTicketBusinessInfo } from './ticketBusinessInfo';
import type { OrderView } from '../../types/pos';

// Ancho real del papel: 80mm a 96dpi. La vista previa se muestra a ese tamaño para que lo que el
// operador aprueba sea literalmente lo que sale por la impresora.
const TICKET_PX = 302;

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
  // debajo de los 80mm del documento le mete scroll horizontal, y en una tablet arrastrar esa barra
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
              accidente cuando busca Imprimir. */}
          <HStack w="100%" gap={2}>
            <Button variant="outline" size="lg" flex="1" onClick={onClose}>
              <LuX /> Cerrar
            </Button>
            <Button size="lg" flex="2" disabled={!html} onClick={() => printFrame(frame.current)}>
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
  isOpen,
  onClose,
}: {
  order: OrderView | null;
  reprint?: boolean;
  sample?: boolean;
  isOpen: boolean;
  onClose: () => void;
}) {
  const { data: business } = useTicketBusinessInfo();
  // html vacío = el diálogo abre con un spinner en vez de no abrir. Un toque que no hace nada es
  // exactamente el fallo silencioso que esta feature vino a quitar.
  const html = order && business ? buildReceiptHtml(order, business, { reprint, sample }) : '';
  return <TicketPreviewDialog html={html} isOpen={isOpen} onClose={onClose} />;
}
