// Aislamiento por empresa DENTRO del dispositivo. Nada de lo que quedó de una sesión anterior debe
// sobrevivir a entrar con otra empresa: ninguna pantalla puede pintar un producto que no es del
// tenant de la sesión.
//
// Sale de un caso real: la caché del menú vive bajo la llave ['menu'] —sin empresa y con gcTime
// infinito— y el carrito se persiste en localStorage. Al cambiar de empresa en la misma tablet, el
// POS seguía pintando el catálogo anterior y el ticket viejo seguía ahí; al cobrar, el backend
// rechazaba con "el producto ya no está en este menú" porque esos ids son de la otra empresa.

const LLAVE = 'sesion.ultimaEmpresa';

// empresaRecordada devuelve la última empresa que usó este dispositivo, o null si no hay marca.
// Vive en localStorage y no en memoria porque el carrito TAMBIÉN se persiste: si la marca muriera
// al recargar, un carrito de la empresa anterior sobreviviría a la recarga sin nada que lo detecte.
export function empresaRecordada(): number | null {
  try {
    const v = localStorage.getItem(LLAVE);
    return v === null ? null : Number(v) || null;
  } catch {
    return null; // almacenamiento bloqueado: se trata como "no sé", y recordarEmpresa fuerza la limpieza
  }
}

export function recordarEmpresa(companyID: number): void {
  try {
    localStorage.setItem(LLAVE, String(companyID));
  } catch { /* almacenamiento bloqueado: sin marca, la próxima entrada limpia de más. Es el lado seguro. */ }
}

// hayQueLimpiar dice si al entrar con `entrante` hay que tirar lo que quedó del tenant anterior.
//
// Se decide al ENTRAR y no al salir a propósito: cerrar sesión también pasa cuando el refresh falla
// por un hipo de red, y limpiar ahí le borraría el ticket a medias a un cajero que va a volver a
// entrar a la MISMA empresa. Sin marca previa (dispositivo nuevo o almacenamiento bloqueado) se
// limpia: el costo es una recarga del menú, y el de no limpiar es cobrar con el catálogo ajeno.
export function hayQueLimpiar(recordada: number | null, entrante: number): boolean {
  return recordada !== entrante;
}
