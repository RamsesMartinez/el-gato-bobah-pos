import { Component, type ErrorInfo, type ReactNode } from 'react';
import { Box, Button, Text, VStack } from '@chakra-ui/react';

// La barrera que impide que un defecto de una pantalla deje la tableta en blanco.
//
// SIN ELLA, un throw durante el render vacía el `#root` y el operador se queda sin nada: ni mensaje,
// ni botón, ni el aviso de "Nueva versión disponible" —lo pinta el toaster, que vive dentro del
// árbol que acaba de morir—. La tableta sigue con el service worker viejo sirviendo la versión rota
// y la única salida es cerrar la app y volver a abrirla, que es justo lo que nadie adivina. Pasó el
// 8 de septiembre de 2026: `platformOrderRef` faltaba en una cuenta guardada y el POS entero dejó de
// renderizar al entrar.
//
// Es una clase y no un hook a propósito: React solo ofrece `getDerivedStateFromError` /
// `componentDidCatch` en componentes de clase. No hay equivalente con hooks.
//
// Lo que NO hace: reportar a un servicio ni reintentar el render. El techo está aquí: recarga y ya.
// ponytail: si algún día hace falta saber cuántas veces pasa, el lugar de enganchar el reporte es
// `componentDidCatch`, que ya recibe el stack de componentes.
interface Props { children: ReactNode }
interface State { seCayo: boolean }

export class PantallaQueNoSeCae extends Component<Props, State> {
  state: State = { seCayo: false };

  static getDerivedStateFromError(): State {
    return { seCayo: true };
  }

  // El detalle va a la consola y NO a la pantalla: en la tableta va lo que el operador puede
  // accionar, y el stack de un componente no es accionable desde el mostrador. En la consola sí
  // sirve, que es donde alguien lo va a leer con el aparato en la mano.
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('la pantalla se cayó:', error, info.componentStack);
  }

  render() {
    if (!this.state.seCayo) return this.props.children;
    return (
      <Box p={6} minH="100dvh" display="flex" alignItems="center" justifyContent="center">
        <VStack gap={4} maxW="440px" textAlign="center">
          <Text fontSize="xl" fontWeight="700">La pantalla no se pudo mostrar</Text>
          <Text color="fg.muted">
            Reinicia la aplicación para seguir vendiendo. Lo que estaba capturado en la cuenta se
            conserva.
          </Text>
          {/* Recarga dura, sin `location.reload()`: una recarga normal puede volver a servirse del
              service worker viejo, que es el que tiene la versión rota. El parámetro fuerza una
              navegación nueva. */}
          <Button size="lg" minH="56px" w="100%" colorPalette="orange" data-alto-minimo="56"
            onClick={() => { window.location.href = `${window.location.pathname}?r=${Date.now()}`; }}>
            Reiniciar la aplicación
          </Button>
        </VStack>
      </Box>
    );
  }
}
