import { useState } from 'react';
import { Button, Input, Text, VStack } from '@chakra-ui/react';
import { posApi } from '../../api/pos';
import { ApiError, notePasswordChanged } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { useSessionStore } from '../../stores/session';

const MIN_LEN = 12;

// Formulario para cambiar la propia contraseña. Lo usan «Mi cuenta» y la pantalla del cambio
// obligatorio; al terminar apaga el indicador de la sesión, que el servidor ya apagó.
export function CambiarContrasena({ onChanged }: { onChanged?: () => void }) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [saving, setSaving] = useState(false);

  const valid = current !== '' && next.length >= MIN_LEN && !next.includes('@') && next === confirm && next !== current;

  const save = async () => {
    setSaving(true);
    // Quién la cambia se toma ANTES de esperar: si mientras tanto entra otra persona con su PIN, el
    // éxito no es suyo y no le apaga su propio cambio pendiente.
    const who = useSessionStore.getState().user?.id;
    try {
      await posApi.changeOwnPassword(current, next);
      toaster.create({ title: 'Contraseña actualizada', type: 'success' });
      setCurrent(''); setNext(''); setConfirm('');
      notePasswordChanged();
      const user = useSessionStore.getState().user;
      if (user && user.id === who && user.mustChangePassword) useSessionStore.setState({ user: { ...user, mustChangePassword: false } });
      onChanged?.();
    } catch (err) {
      toaster.create({ title: 'No se pudo cambiar', description: err instanceof ApiError ? err.message : String(err), type: 'error' });
    } finally {
      setSaving(false);
    }
  };

  return (
    <VStack align="stretch" gap={3}>
      <Input size="lg" type="password" autoComplete="current-password" placeholder="Contraseña actual" value={current} onChange={(e) => setCurrent(e.target.value)} />
      <Input size="lg" type="password" autoComplete="new-password" placeholder="Nueva contraseña (mín. 12)" value={next} onChange={(e) => setNext(e.target.value)} />
      <Input size="lg" type="password" autoComplete="new-password" placeholder="Repetir nueva contraseña" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
      <Text fontSize="sm" color="fg.muted">Mínimo 12 caracteres, sin @ y distinta de la actual.</Text>
      <Button onClick={save} disabled={!valid} loading={saving}>Cambiar contraseña</Button>
    </VStack>
  );
}
