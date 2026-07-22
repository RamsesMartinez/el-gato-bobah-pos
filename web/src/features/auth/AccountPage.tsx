import { useState } from 'react';
import { Box, Heading, Text, Input, Button, VStack, HStack } from '@chakra-ui/react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { posApi } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { useSessionStore } from '../../stores/session';
import { ApiError } from '../../api/client';
import { Page } from '../../components/Page';

const MIN_LEN = 12;

// Cuenta propia: cualquier empleado cambia su contraseña y/o PIN. ?forzar=1 (tras alta/reset por
// admin) muestra un aviso; el backend fue la autoridad que marcó must_change_password.
export function AccountPage() {
  const [params] = useSearchParams();
  const forced = params.get('forzar') === '1';
  const navigate = useNavigate();
  const user = useSessionStore((s) => s.user);

  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [pin, setPin] = useState('');

  const pwValid = current !== '' && next.length >= MIN_LEN && !next.includes('@') && next === confirm;

  const changePw = async () => {
    try {
      await posApi.changeOwnPassword(current, next);
      toaster.create({ title: 'Contraseña actualizada', type: 'success' });
      setCurrent(''); setNext(''); setConfirm('');
      if (forced) navigate('/pos');
    } catch (err) {
      toaster.create({ title: 'No se pudo cambiar', description: err instanceof ApiError ? err.message : String(err), type: 'error' });
    }
  };

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

      {forced && (
        <Box mb={5} p={4} borderWidth="1px" borderColor="colorPalette.emphasized" colorPalette="orange" borderRadius="lg" bg="colorPalette.subtle">
          <Text fontWeight="700">Debes cambiar tu contraseña</Text>
          <Text fontSize="sm" color="fg.muted">Tu contraseña fue asignada por un administrador. Elige una nueva para continuar.</Text>
        </Box>
      )}

      <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5} mb={5}>
        <Text fontWeight="700" mb={3}>Contraseña</Text>
        <VStack align="stretch" gap={3}>
          <Input type="password" placeholder="Contraseña actual" value={current} onChange={(e) => setCurrent(e.target.value)} />
          <Input type="password" placeholder="Nueva contraseña (mín. 12)" value={next} onChange={(e) => setNext(e.target.value)} />
          <Input type="password" placeholder="Repetir nueva contraseña" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          <Button onClick={changePw} disabled={!pwValid}>Cambiar contraseña</Button>
        </VStack>
      </Box>

      <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>PIN de cambio rápido</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>Opcional. Para cambiar de operador sin escribir la contraseña completa.</Text>
        <HStack>
          <Input type="password" inputMode="numeric" placeholder="Nuevo PIN (4-6 díg.)" value={pin} onChange={(e) => setPin(e.target.value)} maxW="200px" />
          <Button variant="outline" onClick={changePin} disabled={pin.length < 4}>Guardar PIN</Button>
        </HStack>
      </Box>
    </Page>
  );
}
