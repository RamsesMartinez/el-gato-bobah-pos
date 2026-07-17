import { useState } from 'react';
import { Box, Center, VStack, Heading, Input, Button, Text, Image } from '@chakra-ui/react';
import { useNavigate } from 'react-router-dom';
import logo from '../../assets/logo.webp';
import { toaster } from '../../components/ui/toaster';
import { posApi } from '../../api/pos';
import { useSessionStore } from '../../stores/session';
import { useUiStore } from '../../stores/ui';
import { ApiError } from '../../api/client';
import { RADIUS } from '../../theme/ui';

export function LoginPage() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const palette = useUiStore((s) => s.palette);
  const setSession = useSessionStore((s) => s.setSession);
  const navigate = useNavigate();

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      const { accessToken, user } = await posApi.login(username, password);
      setSession(accessToken, user);
      navigate('/pos');
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
    <Center h="100vh" bg="bg.subtle" colorPalette={palette}>
      <Box as="form" onSubmit={submit} bg="bg.panel" p={8} borderRadius={RADIUS} boxShadow="lg" w="360px" maxW="90vw">
        <VStack gap={5} align="stretch">
          <Box textAlign="center">
            <Center mb={3}><Image src={logo} alt="El Gato Bobah" boxSize="88px" borderRadius="2xl" /></Center>
            <Heading size="lg">El Gato Bobah</Heading>
            <Text color="fg.muted">Punto de venta</Text>
          </Box>
          <Input size="lg" placeholder="Usuario" value={username} onChange={(e) => setUsername(e.target.value)} autoFocus />
          <Input size="lg" type="password" placeholder="Contraseña" value={password} onChange={(e) => setPassword(e.target.value)} />
          <Button size="lg" type="submit" loading={loading}>Entrar</Button>
        </VStack>
      </Box>
    </Center>
  );
}
