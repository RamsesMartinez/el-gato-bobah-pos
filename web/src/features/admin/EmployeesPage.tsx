import { useState } from 'react';
import {
  Box, Heading, Button, HStack, VStack, Table, Input, Center, Spinner, Badge, Text, IconButton,
} from '@chakra-ui/react';
import { LuPencil, LuKeyRound, LuHash } from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi, type AdminUser } from '../../api/admin';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { Switch } from '../../components/ui/switch';
import { Page } from '../../components/Page';

const ROLES = ['admin', 'gerente', 'cajero', 'mesero'];
const MIN_LEN = 12; // espejo de domain.MinPasswordLen (el backend es la autoridad)

function RolePicker({ value, onChange }: { value: string; onChange: (r: string) => void }) {
  return (
    <HStack gap={1} flexWrap="wrap">
      {ROLES.map((r) => (
        <Button key={r} size="sm" minH="40px" textTransform="capitalize"
          variant={value === r ? 'solid' : 'outline'} colorPalette={value === r ? undefined : 'gray'}
          onClick={() => onChange(r)}>{r}</Button>
      ))}
    </HStack>
  );
}

export function EmployeesPage() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['users'], queryFn: adminApi.users });

  // Alta
  const [name, setName] = useState('');
  const [username, setUsername] = useState('');
  const [role, setRole] = useState('cajero');
  const [password, setPassword] = useState('');
  const [pin, setPin] = useState('');
  const [email, setEmail] = useState('');

  const invalidate = () => qc.invalidateQueries({ queryKey: ['users'] });
  const onErr = (e: unknown) => toaster.create({ title: 'Error', description: String(e), type: 'error' });

  const create = useMutation({
    mutationFn: () => adminApi.createUser({
      name, role, username: username || undefined, password,
      pin: pin || undefined, recoveryEmail: email || undefined,
    }),
    onSuccess: () => {
      invalidate();
      setName(''); setUsername(''); setPassword(''); setPin(''); setEmail('');
      toaster.create({ title: 'Empleado creado', description: 'Deberá cambiar su contraseña al entrar.', type: 'success' });
    },
    onError: onErr,
  });

  const [manage, setManage] = useState<AdminUser | null>(null);

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  const canCreate = name && username && password.length >= MIN_LEN && !password.includes('@');

  return (
    <Page maxW="900px">
      <Heading size="lg" mb={4}>Empleados</Heading>

      <Box bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px" mb={4}>
        <VStack align="stretch" gap={3}>
          <HStack flexWrap="wrap" gap={3}>
            <Input placeholder="Nombre" value={name} onChange={(e) => setName(e.target.value)} maxW="220px" />
            <Input placeholder="Usuario (login)" value={username} onChange={(e) => setUsername(e.target.value)} maxW="180px" autoCapitalize="none" />
            <RolePicker value={role} onChange={setRole} />
          </HStack>
          <HStack flexWrap="wrap" gap={3} align="end">
            <Box>
              <Input type="password" placeholder={`Contraseña (mín. ${MIN_LEN}, sin @)`} value={password} onChange={(e) => setPassword(e.target.value)} maxW="220px" />
              {password !== '' && (password.length < MIN_LEN || password.includes('@')) && <Text color="red.500" fontSize="xs" mt={1}>Mín. {MIN_LEN} caracteres, sin @</Text>}
            </Box>
            <Input placeholder="PIN opcional" value={pin} onChange={(e) => setPin(e.target.value)} maxW="130px" inputMode="numeric" />
            <Input placeholder="Email de recuperación (opcional)" value={email} onChange={(e) => setEmail(e.target.value)} maxW="260px" type="email" />
            <Button disabled={!canCreate} loading={create.isPending} onClick={() => create.mutate()}>Crear</Button>
          </HStack>
        </VStack>
      </Box>

      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Nombre</Table.ColumnHeader>
            <Table.ColumnHeader>Usuario</Table.ColumnHeader>
            <Table.ColumnHeader>Rol</Table.ColumnHeader>
            <Table.ColumnHeader>Recuperación</Table.ColumnHeader>
            <Table.ColumnHeader>Estado</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Gestionar</Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {(data?.items ?? []).map((u) => (
              <Table.Row key={u.id}>
                <Table.Cell>{u.name}</Table.Cell>
                <Table.Cell>{u.username ?? '—'}</Table.Cell>
                <Table.Cell><Badge textTransform="capitalize">{u.role}</Badge></Table.Cell>
                <Table.Cell>{u.recoveryEmail ? u.recoveryEmail : <Text color="fg.subtle">—</Text>}</Table.Cell>
                <Table.Cell>{u.isActive ? <Badge colorPalette="green">activo</Badge> : <Badge>inactivo</Badge>}</Table.Cell>
                <Table.Cell textAlign="end">
                  <IconButton aria-label="Gestionar" size="sm" variant="ghost" onClick={() => setManage(u)}><LuPencil /></IconButton>
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>

      {manage && <ManageDialog user={manage} onClose={() => setManage(null)} onSaved={invalidate} onError={onErr} />}
    </Page>
  );
}

// Dialog de gestión: editar datos, resetear contraseña, fijar PIN, activar/desactivar.
function ManageDialog({ user, onClose, onSaved, onError }: {
  user: AdminUser; onClose: () => void; onSaved: () => void; onError: (e: unknown) => void;
}) {
  const [name, setName] = useState(user.name);
  const [role, setRole] = useState(user.role);
  const [isActive, setIsActive] = useState(user.isActive);
  const [email, setEmail] = useState(user.recoveryEmail ?? '');
  const [newPass, setNewPass] = useState('');
  const [newPin, setNewPin] = useState('');

  const save = useMutation({
    mutationFn: () => adminApi.updateUser(user.id, { name, role, isActive, recoveryEmail: email || null }),
    onSuccess: () => { onSaved(); toaster.create({ title: 'Guardado', type: 'success' }); onClose(); },
    onError,
  });
  const resetPass = useMutation({
    mutationFn: () => adminApi.resetUserPassword(user.id, newPass),
    onSuccess: () => { onSaved(); setNewPass(''); toaster.create({ title: 'Contraseña restablecida', description: 'El empleado deberá cambiarla al entrar.', type: 'success' }); },
    onError,
  });
  const setPinMut = useMutation({
    mutationFn: () => adminApi.setUserPin(user.id, newPin),
    onSuccess: () => { setNewPin(''); toaster.create({ title: 'PIN actualizado', type: 'success' }); },
    onError,
  });

  return (
    <DialogRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="center" size="md">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>{user.name} · @{user.username}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          <VStack align="stretch" gap={5}>
            <VStack align="stretch" gap={2}>
              <Text fontWeight="700">Datos</Text>
              <Input placeholder="Nombre" value={name} onChange={(e) => setName(e.target.value)} />
              <RolePicker value={role} onChange={setRole} />
              <Input placeholder="Email de recuperación" value={email} onChange={(e) => setEmail(e.target.value)} type="email" />
              <HStack justify="space-between">
                <Text>Activo</Text>
                <Switch checked={isActive} onCheckedChange={(e) => setIsActive(e.checked)} />
              </HStack>
              <Button loading={save.isPending} onClick={() => save.mutate()}>Guardar datos</Button>
            </VStack>

            <VStack align="stretch" gap={2}>
              <Text fontWeight="700"><LuKeyRound style={{ display: 'inline' }} /> Restablecer contraseña</Text>
              <HStack>
                <Input type="password" placeholder={`Nueva contraseña (mín. ${MIN_LEN}, sin @)`} value={newPass} onChange={(e) => setNewPass(e.target.value)} />
                <Button variant="outline" colorPalette="orange" loading={resetPass.isPending}
                  disabled={newPass.length < MIN_LEN || newPass.includes('@')} onClick={() => resetPass.mutate()}>Restablecer</Button>
              </HStack>
            </VStack>

            <VStack align="stretch" gap={2}>
              <Text fontWeight="700"><LuHash style={{ display: 'inline' }} /> PIN</Text>
              <HStack>
                <Input inputMode="numeric" placeholder="Nuevo PIN (4-6 díg.)" value={newPin} onChange={(e) => setNewPin(e.target.value)} />
                <Button variant="outline" loading={setPinMut.isPending} disabled={newPin.length < 4} onClick={() => setPinMut.mutate()}>Fijar PIN</Button>
              </HStack>
            </VStack>
          </VStack>
        </DialogBody>
        <DialogFooter><Button variant="ghost" onClick={onClose}>Cerrar</Button></DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
