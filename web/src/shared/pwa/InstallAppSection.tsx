import { useEffect, useState } from 'react';
import { Box, Text, Button } from '@chakra-ui/react';
import { isInstalled, onInstallAvailable, promptInstall } from './installPrompt';

// Sección de Configuración > Negocio para instalar la PWA como app de pantalla completa
// en la tablet, sin depender de que el operador vea el toast automático a tiempo.
//
// El POS se sirve por HTTPS con dominio, así que Chrome ofrece instalar. Por IP local (http://) no
// lo ofrece, y se habilitaba con chrome://flags/#unsafely-treat-insecure-origin-as-secure; eso no va
// en pantalla: es un detalle del navegador que quien opera no necesita (constitución).
export function InstallAppSection() {
  const [installed, setInstalled] = useState(isInstalled);
  const [available, setAvailable] = useState(false);

  useEffect(() => {
    const unsubscribe = onInstallAvailable(setAvailable);
    const onInstalled = () => setInstalled(true);
    window.addEventListener('appinstalled', onInstalled);
    return () => {
      unsubscribe();
      window.removeEventListener('appinstalled', onInstalled);
    };
  }, []);

  return (
    <Box mt={6} borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
      <Text fontWeight="700" mb={1}>Instalar en esta tablet</Text>
      <Text fontSize="sm" color="fg.muted" mb={3}>
        La agrega a la pantalla de inicio como app de pantalla completa, sin barra del
        navegador.
      </Text>
      {installed ? (
        <Text fontSize="sm" fontWeight="600">✓ Ya está instalada en este dispositivo.</Text>
      ) : available ? (
        <Button size="lg" onClick={() => void promptInstall()}>Instalar app</Button>
      ) : (
        <Text fontSize="sm" color="fg.muted">
          Si no aparece el botón: menú ⋮ de Chrome → «Instalar app».
        </Text>
      )}
    </Box>
  );
}
