import { useState } from 'react';
import { Button, HStack, Input, Text, VStack } from '@chakra-ui/react';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
  DrawerCloseTrigger,
} from '../../components/ui/drawer';

interface Props {
  isOpen: boolean;
  plataforma: string;
  /** Lo que ya haya en el campo de la barra de plataformas, para no pedirlo dos veces. */
  valorInicial: string;
  /** Guarda el folio en la cuenta y manda el pedido. */
  onGuardarYMandar: (folio: string) => void;
  /** La salida explícita: manda el pedido sin folio. Queda listado como pendiente. */
  onMandarSinFolio: () => void;
  onCancelar: () => void;
}

// Se pide el folio ANTES de mandar el pedido, cuando el operador todavía tiene la tablet de la
// plataforma enfrente. Al cobrar esa pantalla ya se movió, y el dato es irrecuperable: el reporte
// de pagos de Rappi expone 3 meses y el de Uber 31 días.
//
// Es OBLIGATORIO CON UNA SALIDA, no opcional: un campo opcional no se llena y la feature se vuelve
// decorativa; uno obligatorio sin salida detiene la operación el día que el dato de verdad no está,
// en hora pico. La salida deja la falta REGISTRADA, que es lo que la lista de pendientes aprovecha.
export function FolioPlataformaSheet({
  isOpen, plataforma, valorInicial, onGuardarYMandar, onMandarSinFolio, onCancelar,
}: Props) {
  const [folio, setFolio] = useState(valorInicial);
  const limpio = folio.trim();

  // El campo se vacía al SALIR, no al entrar con un efecto: la hoja queda montada entre aperturas
  // (montarla al vuelo la haría nacer con `open` puesto, que es el caso que dejó a la hoja de cobro
  // sin abrir en Chakra 3.37), y un `useEffect` que escribe estado al abrir es un render en cascada
  // que el linter rechaza con razón. Los tres caminos de salida pasan por aquí.
  const salir = (seguir: () => void) => {
    setFolio('');
    seguir();
  };

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) salir(onCancelar); }}>
      <DrawerBackdrop />
      {/* dvh y no vh: en una tableta, al abrirse el teclado del sistema la ventana visual se
          encoge y `vh` sigue midiendo la pantalla completa, así que el footer queda debajo del
          teclado. Con el footer FIJO dentro de un contenedor en dvh, la salida explícita sigue
          visible mientras se teclea — y sin eso SC-003 pasa de un toque a dos: cerrar el teclado y
          después tocar el botón. */}
      <DrawerContent borderTopRadius="l3" maxH="90dvh" display="flex" flexDirection="column">
        {/* Cerrar NO manda el pedido: vuelve al carrito. Los dos botones del footer mandan, así
            que sin este control la única forma de arrepentirse era tocar fuera de la hoja, y eso no
            se ve. El operador que tocó Cobrar por error tiene que poder volver a corregir la cuenta. */}
        <DrawerCloseTrigger />
        <DrawerHeader pb={1} pr={12}>
          <Text fontSize="lg" fontWeight="700">¿Con qué folio llegó de {plataforma}?</Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={2}>
          <VStack align="stretch" gap={2}>
            <Input
              autoFocus
              size="lg" minH="52px"
              aria-label={`Folio de ${plataforma}`}
              placeholder={`Folio de ${plataforma}`}
              value={folio}
              onChange={(e) => setFolio(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' && limpio) salir(() => onGuardarYMandar(limpio)); }}
              autoComplete="off" autoCapitalize="off" autoCorrect="off" spellCheck={false}
            />
            {/* Instrucción de una línea, en el idioma de quien opera: dónde está el dato, no por
                qué el sistema lo pide. */}
            <Text fontSize="sm" color="fg.muted">
              Es el número que {plataforma} le puso al pedido, tal como aparece en su pantalla.
            </Text>
          </VStack>
        </DrawerBody>
        {/* Footer FIJO: es lo que mantiene la salida a la vista con el teclado abierto. */}
        <DrawerFooter borderTopWidth="1px" pt={3}>
          <HStack gap={2} w="100%">
            <Button
              flex="1" minH="52px" colorPalette="orange"
              disabled={!limpio}
              onClick={() => salir(() => onGuardarYMandar(limpio))}
            >
              Guardar y mandar
            </Button>
            {/* UNA sola salida, y dice exactamente lo que va a pasar. */}
            <Button flex="1" minH="52px" variant="outline" onClick={() => salir(onMandarSinFolio)}>
              Mandar sin folio
            </Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
