import { useState } from 'react';
import { Box, Center, VStack, Heading, Input, Button, Text } from '@chakra-ui/react';
import { useNavigate } from 'react-router-dom';
import { posApi } from '../../api/pos';
import { useUiStore } from '../../stores/ui';
import { RADIUS } from '../../theme/ui';

// Recuperación de contraseña (público). Anti-enumeración: el backend siempre responde 204, así
// que mostramos el MISMO mensaje neutro exista o no la cuenta / tenga o no email registrado.
export function ForgotPasswordPage() {
  const [identifier, setIdentifier] = useState('');
  const [sent, setSent] = useState(false);
  const [loading, setLoading] = useState(false);
  const palette = useUiStore((s) => s.palette);
  const navigate = useNavigate();

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await posApi.forgotPassword(identifier);
    } catch {
      /* neutral: no revelamos fallos */
    } finally {
      setLoading(false);
      setSent(true);
    }
  };

  return (
    <Center h="100dvh" bg="bg.subtle" colorPalette={palette}>
      <Box as="form" onSubmit={submit} bg="bg.panel" p={8} borderRadius={RADIUS} boxShadow="lg" w="380px" maxW="90vw">
        <VStack gap={5} align="stretch">
          <Heading size="lg">Recuperar contraseña</Heading>
          {sent ? (
            <>
              <Text color="fg.muted">
                Si la cuenta existe y tiene un correo de recuperación registrado, te enviamos un
                enlace para restablecer tu contraseña. Revisa tu bandeja de entrada.
              </Text>
              <Button size="lg" onClick={() => navigate('/login')}>Volver a entrar</Button>
            </>
          ) : (
            <>
              <Text color="fg.muted">Escribe tu usuario@empresa; te enviaremos un enlace a tu correo.</Text>
              <Input size="lg" placeholder="usuario@empresa" value={identifier}
                onChange={(e) => setIdentifier(e.target.value)} autoCapitalize="none" autoFocus />
              <Button size="lg" type="submit" loading={loading} disabled={!identifier.includes('@')}>Enviar enlace</Button>
              <Button variant="ghost" size="sm" type="button" onClick={() => navigate('/login')}>Cancelar</Button>
            </>
          )}
        </VStack>
      </Box>
    </Center>
  );
}
