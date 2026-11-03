import { Box, Tabs } from '@chakra-ui/react';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { ClockView } from '../../shared/attendance/ClockView';
import { ReportTab } from './ReportTab';

// AttendancePage es /asistencia. Checar es para todos; quien puede ver el reporte gana la pestaña.
// Pantalla operativa: va a todo el ancho y no usa `Page`, como el POS y Pedidos.
export function AttendancePage() {
  const user = useSessionStore((s) => s.user);
  if (!can('attendance.report', user)) {
    return <Box h="100%" minH={0}><ClockView /></Box>;
  }
  return (
    <Tabs.Root defaultValue="checar" h="100%" display="flex" flexDirection="column" minH={0}>
      <Tabs.List px={5} pt={2}>
        <Tabs.Trigger value="checar" minH="44px">Checar</Tabs.Trigger>
        <Tabs.Trigger value="reporte" minH="44px">Reporte</Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="checar" flex="1" minH={0} p={0}><ClockView /></Tabs.Content>
      <Tabs.Content value="reporte" flex="1" minH={0} px={5} pb={4} display="flex" flexDirection="column"><ReportTab /></Tabs.Content>
    </Tabs.Root>
  );
}
