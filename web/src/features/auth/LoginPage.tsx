import { useState } from 'react';
import { Box, Center, VStack, Heading, Input, Button, Text, Image, InputGroup, IconButton } from '@chakra-ui/react';
import { LuEye, LuEyeOff } from 'react-icons/lu';
import { useNavigate } from 'react-router';
import logo from '../../assets/logo.webp';
import { toaster } from '../../components/ui/toaster';
import { posApi } from '../../api/pos';
import { useSessionStore } from '../../stores/session';
import { useUiStore } from '../../stores/ui';
import { ApiError } from '../../api/client';
import { RADIUS } from '../../theme/ui';

export function LoginPage() {
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [showPass, setShowPass] = useState(false);
  const [loading, setLoading] = useState(false);
  const palette = useUiStore((s) => s.palette);
  const setSession = useSessionStore((s) => s.setSession);
  const navigate = useNavigate();

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      // Un solo campo usuario@empresa; el backend separa el @.
      const { accessToken, user } = await posApi.login(identifier, password);
      setSession(accessToken, user);
      // Tras alta/reset por admin: obligar a cambiar la contraseña antes de operar.
      navigate(user.mustChangePassword ? '/cuenta?forzar=1' : '/pos');
    } catch (err) {
      toaster.create({
        title: 'No se pudo entrar',
        description: err instanceof ApiError ? err.message : String(err),
        type: 'error',
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <Center h="100dvh" bg="bg.subtle" colorPalette={palette}>
      <Box as="form" onSubmit={submit} bg="bg.panel" p={8} borderRadius={RADIUS} boxShadow="lg" w="360px" maxW="90vw">
        <VStack gap={5} align="stretch">
          <Box textAlign="center">
            <Center mb={3}><Image src={logo} alt="El Gato Bobah" boxSize="88px" borderRadius="2xl" /></Center>
            <Heading size="lg">El Gato Bobah</Heading>
            <Text color="fg.muted">Punto de venta</Text>
          </Box>
          <Input size="lg" placeholder="usuario@empresa" value={identifier}
            onChange={(e) => setIdentifier(e.target.value)} autoCapitalize="none" autoFocus />
          <InputGroup endElement={
            <IconButton aria-label={showPass ? 'Ocultar contraseña' : 'Mostrar contraseña'} size="sm" variant="ghost"
              tabIndex={-1} onClick={() => setShowPass((v) => !v)}>
              {showPass ? <LuEyeOff /> : <LuEye />}
            </IconButton>
          }>
            <Input size="lg" type={showPass ? 'text' : 'password'} placeholder="Contraseña"
              value={password} onChange={(e) => setPassword(e.target.value)} />
          </InputGroup>
          <Button size="lg" type="submit" loading={loading}>Entrar</Button>
          <Button variant="ghost" size="sm" onClick={() => navigate('/recuperar')} type="button">
            ¿Olvidaste tu contraseña?
          </Button>
        </VStack>
      </Box>
    </Center>
  );
}
