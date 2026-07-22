import { useState } from 'react';
import { Box, Center, VStack, Heading, Input, Button, Text } from '@chakra-ui/react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { posApi } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { useUiStore } from '../../stores/ui';
import { ApiError } from '../../api/client';
import { RADIUS } from '../../theme/ui';

const MIN_LEN = 12; // espejo de domain.MinPasswordLen (el backend es la autoridad)

// Confirma el reset con el token del email (?token=cid.token). El backend valida fuerza + HIBP.
export function ResetPasswordPage() {
  const [params] = useSearchParams();
  const token = params.get('token') ?? '';
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [loading, setLoading] = useState(false);
  const palette = useUiStore((s) => s.palette);
  const navigate = useNavigate();

  const mismatch = confirm !== '' && password !== confirm;
  const tooShort = password !== '' && password.length < MIN_LEN;
  const hasAt = password.includes('@'); // '@' reservado para usuario@empresa
  const valid = token !== '' && password.length >= MIN_LEN && !hasAt && password === confirm;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await posApi.resetPassword(token, password);
      toaster.create({ title: 'Contraseña actualizada', description: 'Ya puedes entrar con la nueva.', type: 'success' });
      navigate('/login');
    } catch (err) {
      toaster.create({
        title: 'No se pudo restablecer',
        description: err instanceof ApiError ? err.message : String(err),
        type: 'error',
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <Center h="100dvh" bg="bg.subtle" colorPalette={palette}>
      <Box as="form" onSubmit={submit} bg="bg.panel" p={8} borderRadius={RADIUS} boxShadow="lg" w="380px" maxW="90vw">
        <VStack gap={4} align="stretch">
          <Heading size="lg">Nueva contraseña</Heading>
          {token === '' ? (
            <Text color="fg.muted">Enlace inválido o incompleto. Solicita uno nuevo desde «¿Olvidaste tu contraseña?».</Text>
          ) : (
            <>
              <Input size="lg" type="password" placeholder="Nueva contraseña (mín. 12)" value={password} onChange={(e) => setPassword(e.target.value)} autoFocus />
              <Input size="lg" type="password" placeholder="Repetir contraseña" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
              {tooShort && <Text color="red.500" fontSize="sm">Usa al menos {MIN_LEN} caracteres.</Text>}
              {hasAt && <Text color="red.500" fontSize="sm">La contraseña no puede contener @.</Text>}
              {mismatch && <Text color="red.500" fontSize="sm">Las contraseñas no coinciden.</Text>}
              <Button size="lg" type="submit" loading={loading} disabled={!valid}>Guardar contraseña</Button>
            </>
          )}
        </VStack>
      </Box>
    </Center>
  );
}
