import { useState, type ReactNode } from 'react';
import {
  Box, Heading, Button, HStack, VStack, Table, Input, Center, Spinner, Badge, Text, IconButton, Tabs,
} from '@chakra-ui/react';
import { LuPencil, LuKeyRound, LuHash, LuX, LuUserX, LuArrowLeft } from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { ScheduleEditor } from './ScheduleEditor';
import { RolesPage } from './RolesPage';
import { adminApi, type AdminUser, type Role } from '../../api/admin';
import { mensajeDeError } from '../../api/mensajes';
import { Picker } from '../../components/Picker';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { Switch } from '../../components/ui/switch';
import { Page } from '../../components/Page';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { fitsIn } from './roleEdit';

const MIN_LEN = 12; // espejo de domain.MinPasswordLen (el backend es la autoridad)

// RolePicker ofrece los roles activos de la empresa que caben en lo que puede la sesión (el servidor
// rechaza asignar uno que puede más) y, al editar, también el que la persona ya tiene aunque esté
// inactivo: sin él, el disparador saldría vacío y no se sabría qué rol tiene hoy.
function RolePicker({ roles, value, onChange, current, mine }: {
  roles: Role[]; value: number | null; onChange: (id: number) => void; current?: number | null; mine: string[] | undefined;
}) {
  const options = roles
    .filter((r) => r.id === current || (r.active && fitsIn(r.permissions ?? [], mine)))
    .map((r) => ({ value: String(r.id), label: r.name, hint: r.active ? undefined : 'inactivo' }));
  return (
    <Picker value={value ? String(value) : ''} options={options} onChange={(v) => onChange(Number(v))}
      placeholder="Elige un rol" title="Rol" focusSearch={false} />
  );
}

export function EmployeesPage() {
  const user = useSessionStore((s) => s.user);
  const canPeople = can('employees.manage', user);
  const canRoles = can('roles.manage', user);
  const [tab, setTab] = useState<'people' | 'roles'>(canPeople ? 'people' : 'roles');
  const [roleFilter, setRoleFilter] = useState<number | null>(null);
  // rolesDirty: el editor de Roles ya no es una hoja modal; cambiar a Personal lo desmonta y se
  // perdería lo marcado. Salir por el riel no se puede detener: la app no usa un data router.
  const [rolesDirty, setRolesDirty] = useState(false);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const pickTab = (v: 'people' | 'roles') => {
    if (v === 'people' && active === 'roles' && rolesDirty) { setConfirmLeave(true); return; }
    setTab(v);
  };
  const leaveRoles = () => { setConfirmLeave(false); setRolesDirty(false); setTab('people'); };
  const both = canPeople && canRoles;
  const active = canPeople && (tab === 'people' || !canRoles) ? 'people' : 'roles';

  const tabs = both && (
    <Tabs.Root value={active} onValueChange={(e) => pickTab(e.value as 'people' | 'roles')} variant="enclosed">
      <Tabs.List>
        <Tabs.Trigger value="people" minH="44px">Personal</Tabs.Trigger>
        <Tabs.Trigger value="roles" minH="44px">Roles</Tabs.Trigger>
      </Tabs.List>
    </Tabs.Root>
  );

  // Roles va en dos paneles (#243): las pestañas suben a la columna de la lista y el relleno se
  // reduce, para que el rol elegido tenga el alto entero de la tableta.
  if (active === 'roles') {
    return (
      <Page fill p={3}>
        <RolesPage tabs={tabs || undefined} onDirtyChange={setRolesDirty}
          onShowPeople={(id) => { setRoleFilter(id); setRolesDirty(false); setTab('people'); }} />
        <ConfirmSheet isOpen={confirmLeave} title="¿Descartar los cambios?" destructive
          description="Lo que marcaste o quitaste en este rol no se guarda."
          confirmLabel="Descartar" cancelLabel="Seguir editando"
          onConfirm={leaveRoles} onCancel={() => setConfirmLeave(false)} />
      </Page>
    );
  }

  return (
    <Page fill maxW="900px">
      <PeopleTab roleFilter={roleFilter} onClearFilter={() => setRoleFilter(null)}
        header={<><Heading size="lg">Empleados</Heading>{tabs}</>} />
    </Page>
  );
}

function PeopleTab({ roleFilter, onClearFilter, header }: { roleFilter: number | null; onClearFilter: () => void; header: ReactNode }) {
  const qc = useQueryClient();
  const mine = useSessionStore((s) => s.user?.permissions);
  const { data, isLoading } = useQuery({ queryKey: ['users'], queryFn: adminApi.users });
  // Las bajas van en su propia consulta: la lista de siempre no cambia, y el botón dice cuántas hay.
  const inactiveQ = useQuery({ queryKey: ['users', 'inactive'], queryFn: adminApi.inactiveUsers });
  const [showInactive, setShowInactive] = useState(false);
  const [reactivate, setReactivate] = useState<AdminUser | null>(null);
  const rolesQ = useQuery({ queryKey: ['roles'], queryFn: adminApi.roles });
  const roles = rolesQ.data?.items ?? [];
  const roleName = (u: AdminUser) => roles.find((r) => r.id === u.roleId)?.name ?? u.role;

  // Alta
  const [name, setName] = useState('');
  const [username, setUsername] = useState('');
  const [roleId, setRoleId] = useState<number | null>(null);
  const [password, setPassword] = useState('');
  const [pin, setPin] = useState('');
  const [email, setEmail] = useState('');

  const invalidate = () => qc.invalidateQueries({ queryKey: ['users'] });
  const onErr = (e: unknown) => toaster.create({ title: 'Error', description: mensajeDeError(e), type: 'error' });

  const create = useMutation({
    mutationFn: () => adminApi.createUser({
      name, roleId: roleId!, username: username || undefined, password,
      pin: pin || undefined, recoveryEmail: email || undefined,
    }),
    onSuccess: () => {
      invalidate();
      setName(''); setUsername(''); setRoleId(null); setPassword(''); setPin(''); setEmail('');
      toaster.create({ title: 'Empleado creado', description: 'Deberá cambiar su contraseña al entrar.', type: 'success' });
    },
    onError: onErr,
  });

  const [manage, setManage] = useState<AdminUser | null>(null);
  const [scheduleUser, setScheduleUser] = useState<AdminUser | null>(null);

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  const canCreate = name && username && roleId && password.length >= MIN_LEN && !password.includes('@');
  const byRole = (u: AdminUser) => roleFilter === null || u.roleId === roleFilter;
  const shown = (data?.items ?? []).filter(byRole);
  const inactive = (inactiveQ.data?.items ?? []).filter(byRole);
  const filterName = roles.find((r) => r.id === roleFilter)?.name;
  // Sin la cuenta mientras carga o si falló: «(0)» haría creer que no hay a quién reactivar.
  const inactiveLabel = inactiveQ.isSuccess ? `Dadas de baja (${inactive.length})` : 'Dadas de baja';
  // El botón va en el renglón del título, que ya mide 44 px por las pestañas: la lista activa no
  // pierde alto. Se eligió contra una sección plegada al final (el control quedaba fuera de la
  // pantalla) y un selector junto a las pestañas (dos niveles de pestañas en un renglón).
  const toggle = (
    <Button minH="44px" ml="auto" variant="outline" colorPalette="gray" onClick={() => setShowInactive(!showInactive)}>
      {showInactive ? <><LuArrowLeft /> Volver a la plantilla</> : <><LuUserX /> {inactiveLabel}</>}
    </Button>
  );

  const inactiveTable = (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm">
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Nombre</Table.ColumnHeader>
          <Table.ColumnHeader>Usuario</Table.ColumnHeader>
          <Table.ColumnHeader>Rol</Table.ColumnHeader>
          <Table.ColumnHeader />
        </Table.Row></Table.Header>
        <Table.Body>
          {inactive.map((u) => {
            const role = roles.find((r) => r.id === u.roleId);
            return (
              <Table.Row key={u.id}>
                <Table.Cell>{u.name}</Table.Cell>
                <Table.Cell>{u.username ?? '—'}</Table.Cell>
                <Table.Cell><HStack gap={2}><Badge>{roleName(u)}</Badge>{role && !role.active && <Badge colorPalette="orange">rol inactivo</Badge>}</HStack></Table.Cell>
                <Table.Cell textAlign="end">
                  <Button size="sm" minH="44px" variant="outline" colorPalette="gray" onClick={() => setReactivate(u)}>Reactivar</Button>
                </Table.Cell>
              </Table.Row>
            );
          })}
        </Table.Body>
      </Table.Root>
      {inactiveQ.isLoading && <Center p={6}><Spinner /></Center>}
      {inactiveQ.isError && (
        <VStack p={6} gap={3}>
          <Text color="fg.error">No se pudo cargar la lista</Text>
          <Button minH="44px" variant="outline" onClick={() => inactiveQ.refetch()}>Reintentar</Button>
        </VStack>
      )}
      {inactiveQ.isSuccess && inactive.length === 0 && (
        <Text p={6} textAlign="center" color="fg.muted">
          {roleFilter !== null ? 'Nadie dado de baja tiene este rol' : 'No hay personas dadas de baja'}
        </Text>
      )}
    </Box>
  );

  return (
    <Box flex="1" minH={0} overflowY="auto">
      <HStack mb={3} gap={4} flexWrap="wrap">
        {header}
        {toggle}
      </HStack>
      {showInactive ? (
        <>
          {/* El título y el filtro comparten renglón: la vista dice dónde se está sin gastar dos. */}
          <HStack mb={2} gap={3} flexWrap="wrap" minH="44px">
            <Heading as="h2" size="md">{inactiveLabel}</Heading>
            {roleFilter !== null && (
              <Button minH="44px" variant="subtle" colorPalette="gray" onClick={onClearFilter}>
                Rol: {filterName ?? 'ese rol'} <LuX />
              </Button>
            )}
          </HStack>
          {inactiveTable}
        </>
      ) : (<>
      <Box bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px" mb={4}>
        <VStack align="stretch" gap={3}>
          <HStack flexWrap="wrap" gap={3}>
            <Input minH="44px" placeholder="Nombre" value={name} onChange={(e) => setName(e.target.value)} maxW="220px" />
            <Input minH="44px" placeholder="Usuario (login)" value={username} onChange={(e) => setUsername(e.target.value)} maxW="180px" autoCapitalize="none" />
            <Box minW="200px" flex="1" maxW="260px"><RolePicker roles={roles} value={roleId} onChange={setRoleId} mine={mine} /></Box>
          </HStack>
          <HStack flexWrap="wrap" gap={3} align="end">
            <Box>
              <Input minH="44px" type="password" placeholder={`Contraseña (mín. ${MIN_LEN}, sin @)`} value={password} onChange={(e) => setPassword(e.target.value)} maxW="220px" />
              {password !== '' && (password.length < MIN_LEN || password.includes('@')) && <Text color="fg.error" fontSize="xs" mt={1}>Mín. {MIN_LEN} caracteres, sin @</Text>}
            </Box>
            <Input minH="44px" placeholder="PIN opcional" value={pin} onChange={(e) => setPin(e.target.value)} maxW="130px" inputMode="numeric" />
            <Input minH="44px" placeholder="Email de recuperación (opcional)" value={email} onChange={(e) => setEmail(e.target.value)} maxW="260px" type="email" />
            <Button minH="44px" disabled={!canCreate} loading={create.isPending} onClick={() => create.mutate()}>Crear</Button>
          </HStack>
        </VStack>
      </Box>

      {roleFilter !== null && (
        <Button minH="44px" mb={3} variant="subtle" colorPalette="gray" onClick={onClearFilter}>
          Rol: {filterName ?? 'ese rol'} <LuX />
        </Button>
      )}
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
            {shown.map((u) => (
              <Table.Row key={u.id}>
                <Table.Cell>{u.name}</Table.Cell>
                <Table.Cell>{u.username ?? '—'}</Table.Cell>
                <Table.Cell><Badge>{roleName(u)}</Badge></Table.Cell>
                <Table.Cell>{u.recoveryEmail ? u.recoveryEmail : <Text color="fg.subtle">—</Text>}</Table.Cell>
                <Table.Cell>{u.isActive ? <Badge colorPalette="green">activo</Badge> : <Badge>inactivo</Badge>}</Table.Cell>
                <Table.Cell textAlign="end">
                  <IconButton aria-label="Gestionar" size="sm" minW="44px" minH="44px" variant="ghost" onClick={() => setManage(u)}><LuPencil /></IconButton>
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
        {roleFilter !== null && shown.length === 0 && (
          <VStack p={6} gap={3}>
            <Text color="fg.muted">Nadie tiene este rol</Text>
            <Button minH="44px" variant="outline" onClick={onClearFilter}>Quitar el filtro</Button>
          </VStack>
        )}
      </Box>
      </>)}

      {reactivate && <ReactivateSheet user={reactivate} roles={roles} mine={mine}
        onClose={() => setReactivate(null)} onSaved={invalidate} onError={onErr} />}

      {manage && <ManageDialog user={manage} roles={roles} mine={mine} onClose={() => setManage(null)} onSaved={invalidate} onError={onErr}
        onSchedule={() => { setScheduleUser(manage); setManage(null); }} />}
      {scheduleUser && <ScheduleEditor userId={scheduleUser.id} name={scheduleUser.name} onClose={() => setScheduleUser(null)}
        people={(data?.items ?? []).map((u) => ({ id: u.id, name: u.name }))} />}
    </Box>
  );
}

// Dialog de gestión: editar datos, resetear contraseña, fijar PIN, activar/desactivar.
function ManageDialog({ user, roles, mine, onClose, onSaved, onError, onSchedule }: {
  user: AdminUser; roles: Role[]; mine: string[] | undefined; onClose: () => void; onSaved: () => void; onError: (e: unknown) => void; onSchedule: () => void;
}) {
  const [name, setName] = useState(user.name);
  const [roleId, setRoleId] = useState<number | null>(user.roleId ?? null);
  const [isActive, setIsActive] = useState(user.isActive);
  const [email, setEmail] = useState(user.recoveryEmail ?? '');
  const [newPass, setNewPass] = useState('');
  const [newPin, setNewPin] = useState('');
  // A quien puede más que la sesión no se le toca nada (el servidor responde 403). Sin su rol en la
  // lista no se sabe qué puede; el servidor sigue siendo la barrera.
  const userRole = roles.find((r) => r.id === user.roleId);
  const outranks = !!userRole && !fitsIn(userRole.permissions ?? [], mine);

  const save = useMutation({
    mutationFn: () => adminApi.updateUser(user.id, { name, roleId: roleId!, isActive, recoveryEmail: email || null }),
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
          {outranks ? (
            <Text color="fg.muted">Esta persona tiene permisos que tu usuario no tiene: no puedes cambiar sus datos, contraseña ni PIN.</Text>
          ) : (
          <VStack align="stretch" gap={5}>
            <VStack align="stretch" gap={2}>
              <Text fontWeight="700">Datos</Text>
              <Input minH="44px" placeholder="Nombre" value={name} onChange={(e) => setName(e.target.value)} />
              <RolePicker roles={roles} value={roleId} onChange={setRoleId} current={user.roleId} mine={mine} />
              <Input minH="44px" placeholder="Email de recuperación" value={email} onChange={(e) => setEmail(e.target.value)} type="email" />
              {/* El renglón entero es el control: tocar «Activo» no movía el interruptor. */}
              <Switch checked={isActive} onCheckedChange={(e) => setIsActive(e.checked)}
                w="100%" minH="44px" flexDirection="row-reverse" justifyContent="space-between" cursor="pointer">
                Activo
              </Switch>
              <Button minH="44px" loading={save.isPending} disabled={!roleId} onClick={() => save.mutate()}>Guardar datos</Button>
            </VStack>

            <VStack align="stretch" gap={2}>
              <Text fontWeight="700"><LuKeyRound style={{ display: 'inline' }} /> Restablecer contraseña</Text>
              <HStack>
                <Input minH="44px" type="password" placeholder={`Nueva contraseña (mín. ${MIN_LEN}, sin @)`} value={newPass} onChange={(e) => setNewPass(e.target.value)} />
                <Button minH="44px" variant="outline" colorPalette="orange" loading={resetPass.isPending}
                  disabled={newPass.length < MIN_LEN || newPass.includes('@')} onClick={() => resetPass.mutate()}>Restablecer</Button>
              </HStack>
            </VStack>

            <VStack align="stretch" gap={2}>
              <Text fontWeight="700"><LuHash style={{ display: 'inline' }} /> PIN</Text>
              <HStack>
                <Input minH="44px" inputMode="numeric" placeholder="Nuevo PIN (4-6 díg.)" value={newPin} onChange={(e) => setNewPin(e.target.value)} />
                <Button minH="44px" variant="outline" loading={setPinMut.isPending} disabled={newPin.length < 4} onClick={() => setPinMut.mutate()}>Fijar PIN</Button>
              </HStack>
            </VStack>
          </VStack>
          )}
        </DialogBody>
        <DialogFooter>
          {/* El horario lo lee y lo guarda quien configura la asistencia; sin el permiso la hoja abría
              vacía y editable con un 403 detrás, como si la persona no tuviera horario. */}
          {mine?.includes('attendance.configure') && (
            <Button variant="outline" minH="44px" mr="auto" onClick={onSchedule}>Horario y asistencia</Button>
          )}
          <Button minH="44px" variant="ghost" onClick={onClose}>Cerrar</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

// ReactivateSheet devuelve a una persona dada de baja con su usuario, contraseña y PIN de antes. Su
// rol se conserva si sigue activo y cabe en lo que puede la sesión; si no, se elige otro aquí mismo,
// porque el servidor no reactiva a nadie con un rol inactivo.
function ReactivateSheet({ user, roles, mine, onClose, onSaved, onError }: {
  user: AdminUser; roles: Role[]; mine: string[] | undefined; onClose: () => void; onSaved: () => void; onError: (e: unknown) => void;
}) {
  const prev = roles.find((r) => r.id === user.roleId);
  const keeps = !!prev && prev.active && fitsIn(prev.permissions ?? [], mine);
  const [roleId, setRoleId] = useState<number | null>(keeps ? prev.id : null);
  const save = useMutation({
    mutationFn: () => adminApi.updateUser(user.id, { name: user.name, roleId: roleId!, isActive: true, recoveryEmail: user.recoveryEmail ?? null }),
    onSuccess: () => { onSaved(); toaster.create({ title: `Se reactivó a ${user.name}`, type: 'success' }); onClose(); },
    onError,
  });
  return (
    <ConfirmSheet isOpen title={`¿Reactivar a ${user.name}?`}
      description="Vuelve a entrar con su usuario, contraseña y PIN de antes."
      confirmLabel="Reactivar" cancelLabel="Cancelar"
      loading={save.isPending} confirmDisabled={!roleId} onConfirm={() => save.mutate()} onCancel={onClose}>
      <VStack align="stretch" gap={2} mt={4}>
        {prev && !keeps && (
          <Text fontSize="sm">
            {prev.active ? `Su rol, ${prev.name}, tiene permisos que tu usuario no tiene: elige otro.` : `Su rol, ${prev.name}, está inactivo: elige otro.`}
          </Text>
        )}
        <RolePicker roles={roles} value={roleId} onChange={setRoleId} mine={mine} />
      </VStack>
    </ConfirmSheet>
  );
}
