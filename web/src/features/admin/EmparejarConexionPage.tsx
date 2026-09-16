import { Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuArrowLeft } from 'react-icons/lu';
import { useNavigate, useParams } from 'react-router';
import { Page } from '../../components/Page';
import { EmparejarPage } from './EmparejarPage';

/**
 * Envoltura de ruta para el emparejamiento: saca el id de la conexión de la URL y le da salida.
 *
 * Va aparte de `EmparejarPage` para que ésta siga siendo probable sin router — es la pantalla con
 * más lógica de las dos y sus pruebas no tienen por qué montar rutas.
 */
export function EmparejarConexionPage() {
  const { id } = useParams();
  const navegar = useNavigate();
  const conexionId = Number(id);

  if (!Number.isFinite(conexionId) || conexionId <= 0) {
    return (
      <Page>
        <Text>Esa tienda no existe.</Text>
      </Page>
    );
  }

  return (
    <Page>
      <VStack align="stretch" gap={4}>
        <HStack>
          <Button variant="ghost" minH="44px" onClick={() => navegar('/plataformas')}>
            <LuArrowLeft /> Tiendas
          </Button>
        </HStack>
        <EmparejarPage conexionId={conexionId} onListo={() => navegar('/plataformas')} />
      </VStack>
    </Page>
  );
}
