import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Heading, HStack, Text, VStack } from '@chakra-ui/react';
import { attendanceAdminApi, type AttendanceSettings } from '../../api/attendance';
import { Switch } from '../../components/ui/switch';
import { Picker } from '../../components/Picker';

const GRACE_OPTIONS = [0, 5, 10, 15, 20, 30, 45, 60].map((m) => ({ value: String(m), label: m === 0 ? 'Sin tolerancia' : `${m} min` }));

// AttendanceSettingsSection es la sección «Asistencia» de Negocio: activarla, y si se miden
// retardos y minutos extra con cuánta tolerancia. Por omisión todo apagado.
export function AttendanceSettingsSection() {
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ['asistencia', 'settings'], queryFn: attendanceAdminApi.settings });
  const save = useMutation({
    mutationFn: attendanceAdminApi.setSettings,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['asistencia'] });
    },
  });
  if (!data) return null;
  const applyChange = (p: Partial<AttendanceSettings>) => save.mutate({ ...data, ...p });

  return (
    <Box mt={6} borderWidth="1px" borderRadius="lg" p={4}>
      <Heading size="md" mb={2}>Asistencia</Heading>
      <VStack align="stretch" gap={1}>
        <HStack justify="space-between">
          <Text>Registrar entradas y salidas del personal</Text>
          <Switch size="lg" py="10px" checked={data.enabled} disabled={save.isPending}
            onCheckedChange={(e) => applyChange({ enabled: e.checked })} aria-label="Registrar asistencia" />
        </HStack>
        <HStack justify="space-between">
          <Text color={data.enabled ? undefined : 'fg.muted'}>Medir retardos y minutos extra contra el horario</Text>
          <Switch size="lg" py="10px" checked={data.measureSchedule} disabled={!data.enabled || save.isPending}
            onCheckedChange={(e) => applyChange({ measureSchedule: e.checked })} aria-label="Medir retardos" />
        </HStack>
        {data.enabled && data.measureSchedule && (
          <HStack gap={4} pt={2}>
            <Box flex="1">
              <Text fontSize="sm" color="fg.muted" mb={1}>Tolerancia de entrada</Text>
              <Picker title="Tolerancia de entrada" value={String(data.graceInMin)} options={GRACE_OPTIONS}
                onChange={(v) => applyChange({ graceInMin: Number(v) })} />
            </Box>
            <Box flex="1">
              <Text fontSize="sm" color="fg.muted" mb={1}>Tolerancia de salida</Text>
              <Picker title="Tolerancia de salida" value={String(data.graceOutMin)} options={GRACE_OPTIONS}
                onChange={(v) => applyChange({ graceOutMin: Number(v) })} />
            </Box>
          </HStack>
        )}
      </VStack>
    </Box>
  );
}
