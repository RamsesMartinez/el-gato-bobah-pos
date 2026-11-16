import { useState } from 'react';
import { Box, Heading, Text, Input, Button, HStack } from '@chakra-ui/react';
import { posApi } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { useSessionStore } from '../../stores/session';
import { ApiError } from '../../api/client';
import { Page } from '../../components/Page';
import { CambiarContrasena } from './CambiarContrasena';

// Cuenta propia: cualquier empleado cambia su contraseña y/o PIN. El cambio obligatorio tras un
// alta o restablecimiento no pasa por aquí: lo pide RequireAuth antes de pintar la aplicación.
export function AccountPage() {
  const user = useSessionStore((s) => s.user);
  const [pin, setPin] = useState('');

  const changePin = async () => {
    try {
      await posApi.setOwnPin(pin);
      toaster.create({ title: 'PIN actualizado', type: 'success' });
      setPin('');
    } catch (err) {
      toaster.create({ title: 'No se pudo cambiar el PIN', description: err instanceof ApiError ? err.message : String(err), type: 'error' });
    }
  };

  return (
    <Page maxW="520px">
      <Heading size="lg" mb={1}>Mi cuenta</Heading>
      <Text color="fg.muted" mb={6}>
        {user?.name}{user?.companySlug ? ` · @${user.companySlug}` : ''}
      </Text>

      <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5} mb={5}>
        <Text fontWeight="700" mb={3}>Contraseña</Text>
        <CambiarContrasena />
      </Box>
      <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>PIN de cambio rápido</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>Opcional. Para cambiar de operador sin escribir la contraseña completa.</Text>
        <HStack>
          <Input size="lg" type="password" inputMode="numeric" placeholder="Nuevo PIN (4-6 díg.)" value={pin} onChange={(e) => setPin(e.target.value)} maxW="200px" />
          <Button variant="outline" onClick={changePin} disabled={pin.length < 4}>Guardar PIN</Button>
        </HStack>
      </Box>
    </Page>
  );
}
