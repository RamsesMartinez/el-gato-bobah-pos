import { Button, HStack, Text } from '@chakra-ui/react';
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

  // `fill`: la lista de la tienda hace scroll en su propia caja y para eso la página tiene que
  // tener alto (patrón de Page.tsx). Sin él la lista crece y saca de la tableta todo lo de abajo.
  return (
    <Page fill py={3}>
      <HStack>
        <Button variant="ghost" minH="44px" onClick={() => navegar('/plataformas')}>
          <LuArrowLeft /> Tiendas
        </Button>
      </HStack>
      <EmparejarPage conexionId={conexionId} onListo={() => navegar('/plataformas')} />
    </Page>
  );
}
