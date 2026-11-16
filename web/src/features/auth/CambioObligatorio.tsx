import { useState } from 'react';
import { Box, Button, Heading, Text } from '@chakra-ui/react';
import { posApi } from '../../api/pos';
import { useSessionStore } from '../../stores/session';
import { CambiarContrasena } from './CambiarContrasena';

// Lo único que ve quien entró con la contraseña que le puso un admin (#311). Ocupa el lugar de toda
// la aplicación —no es una ruta—, así que recargar o escribir otra dirección no la salta. El
// servidor rechaza lo demás de todas formas; esto es para que la persona sepa qué hacer.
export function CambioObligatorio() {
  const name = useSessionStore((s) => s.user?.name);
  const clear = useSessionStore((s) => s.clear);
  const [saliendo, setSaliendo] = useState(false);

  // Salir revoca la cookie de refresh antes de limpiar: sin eso, recargar volvería a esta sesión.
  const salir = async () => {
    setSaliendo(true);
    try {
      await posApi.logout();
    } catch {
      /* red caída: se cierra localmente de todos modos */
    }
    clear();
  };

  return (
    // Arriba y con scroll, no centrada: el teclado de la tableta encoge la pantalla y una caja
    // centrada deja el botón debajo del teclado.
    <Box h="100dvh" overflowY="auto" bg="bg.subtle" px={4} py={{ base: 6, md: 12 }} display="flex" justifyContent="center" alignItems="flex-start">
      <Box bg="bg.panel" borderWidth="1px" borderColor="border" borderRadius="lg" p={6} w="420px" maxW="100%">
        <Heading size="lg" mb={1}>Debes cambiar tu contraseña</Heading>
        <Text color="fg.muted" mb={5}>
          {name ? `${name}, tu` : 'Tu'} contraseña la asignó un administrador. Elige una nueva para continuar.
        </Text>
        <CambiarContrasena />
        <Button mt={4} w="full" variant="ghost" onClick={salir} loading={saliendo}>Cerrar sesión</Button>
      </Box>
    </Box>
  );
}
