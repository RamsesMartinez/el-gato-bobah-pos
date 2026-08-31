import { useQuery } from '@tanstack/react-query';

// ?inline: Vite lo entrega ya como data URI en vez de como URL con hash. El ticket tiene que ser
// un documento autocontenido (ver el comentario de TicketBusinessInfo.logoDataUri).
import defaultLogo from '../../assets/logo.webp?inline';
import { posApi, type BusinessSettings } from '../../api/pos';
import type { TicketBusinessInfo } from '../../utils/printReceipt';

// toTicketBusinessInfo arma el encabezado a partir de los ajustes. Pura: la única decisión real es
// qué logo va, y por eso vive aquí y no dentro del hook.
export function toTicketBusinessInfo(
  settings: BusinessSettings,
  uploadedLogoDataUri?: string,
): TicketBusinessInfo {
  return {
    businessName: settings.businessName,
    address: settings.address,
    phone: settings.phone,
    headerNote: settings.headerNote,
    footerNote: settings.footerNote,
    // El `||` y no `??` a propósito: una conversión fallida deja string vacío, y un
    // <img src=""> en el ticket es peor que el logo por default.
    logoDataUri: uploadedLogoDataUri || defaultLogo,
  };
}

// blobToDataUri convierte el binario del logo en un data URI. Se hace una sola vez por versión del
// logo y se cachea en react-query: reconvertir 256 KB en cada ticket es trabajo tirado.
function blobToDataUri(blob: Blob): Promise<string> {
  return new Promise((resolve) => {
    const reader = new FileReader();
    reader.onerror = () => resolve(''); // el caller cae al logo por default
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
    reader.readAsDataURL(blob);
  });
}

// useTicketBusinessInfo entrega el encabezado listo para imprimir. Devuelve undefined mientras
// carga: es preferible que la vista previa espere a que el ticket salga con el nombre equivocado.
export function useTicketBusinessInfo(): {
  data: TicketBusinessInfo | undefined;
  isLoading: boolean;
  autoPrintOnClose: boolean;
  // Si el ticket lista los adicionales sin costo. Sale del ajuste del negocio, con true como
  // valor seguro mientras la consulta no responde: el ticket completo es el comportamiento viejo.
  printFreeModifiers: boolean;
  // Si al mandar el pedido sale la comanda de cocina. false como valor seguro mientras carga: en
  // duda, no se imprime papel de más.
  printKitchenTicket: boolean;
} {
  const settings = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });

  // La clave lleva logoUpdatedAt: subir un logo nuevo cambia la clave y el data URI se rearma solo,
  // sin reiniciar ni invalidar a mano (SC-004).
  const logo = useQuery({
    queryKey: ['ticket-logo', settings.data?.logoUpdatedAt],
    enabled: settings.data?.hasLogo === true,
    // El logo cambia cuando alguien lo sube, no solo por pasar el tiempo.
    staleTime: Infinity,
    queryFn: async () => {
      const res = await posApi.ticketLogo();
      return blobToDataUri(await res.blob());
    },
  });

  if (!settings.data) {
    return {
      data: undefined, isLoading: settings.isLoading,
      autoPrintOnClose: false, printFreeModifiers: true, printKitchenTicket: false,
    };
  }
  // No se espera al logo subido: si tarda o falla, el ticket sale con el default en vez de dejar
  // al operador esperando frente al cliente.
  return {
    data: toTicketBusinessInfo(settings.data, logo.data),
    isLoading: false,
    autoPrintOnClose: settings.data.autoPrintOnClose,
    printFreeModifiers: settings.data.printFreeModifiers,
    printKitchenTicket: settings.data.printKitchenTicket,
  };
}
