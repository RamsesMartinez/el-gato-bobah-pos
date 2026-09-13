import { useEffect } from 'react';
import { useLocation } from 'react-router';
import { medirToque } from '../api/uso';
import { pantallaDe, seMidenSusToques } from './rutas-medidas';
import { celdaDelToque } from './celda';

// DÓNDE CAE EL DEDO (spec 019). No pinta nada: solo escucha.
//
// Vive al lado de las rutas, como el medidor de la 017, y no envuelve la aplicación: un error aquí
// no puede dejar sin pintar el mostrador.

// Cuánto se puede mover el dedo entre que baja y sube sin dejar de ser un toque.
//
// Sin este umbral, cada desplazamiento de la lista de productos deja un rastro y el mapa mide
// scroll en vez de intención: la columna por donde se arrastra saldría como la más usada del POS.
// Diez píxeles es lo que se mueve un dedo quieto en un vidrio.
const UMBRAL_DE_ARRASTRE_PX = 10;

// Cuántos contactos simultáneos se siguen a la vez. Diez son todos los dedos de dos manos; el tope
// existe para que un `pointerup` que nunca llega —el navegador cancela el contacto sin avisar— no
// deje creciendo el mapa durante todo un turno.
const MAX_CONTACTOS = 10;

export function MedidorDeToques() {
  const pantalla = pantallaDe(useLocation().pathname);
  const midiendo = seMidenSusToques(pantalla);

  useEffect(() => {
    // En once de las doce pantallas no se escucha nada: el filtro de la lista blanca se aplica
    // aquí y no dentro del manejador, así que no hay ni un listener colgado de más.
    if (!midiendo || !pantalla) return;

    // EL PUNTO DE BAJADA SE GUARDA POR `pointerId`, en un mapa y no en una variable.
    //
    // En el mostrador hay dos manos —una sostiene comida, la otra toca— y dos contactos se
    // solapan. Con un solo estado compartido, el `down` de un dedo se compara con el `up` del otro
    // y salen arrastres falsos: se pierden los dos toques y nada falla.
    const bajadas = new Map<number, { x: number; y: number }>();

    const alBajar = (e: PointerEvent) => {
      // LAS CAPAS DE ENCIMA NO CUENTAN, y se decide al revés de lo obvio: **solo cuenta lo que
      // cuelga del contenedor de la aplicación** (`data-medible`, en AppShell). Hojas, diálogos,
      // avisos flotantes y el bloqueo por PIN no cambian de ruta, así que sus toques se
      // atribuirían a la pantalla de abajo.
      //
      // La primera versión excluía `[role="dialog"]`, que es una lista de lo prohibido: un portal
      // nuevo nacía CONTADO. Y ya había uno — el aviso flotante de Chakra se anuncia con
      // `role="status"` y se pinta abajo a la derecha, encima de la zona del botón de cobrar, con
      // un «Deshacer» que la gente toca. La rejilla habría inflado justo la celda que decide un
      // rediseño.
      //
      // El `[role="dialog"]` se queda como segunda red, para un diálogo que se pinte SIN portal
      // dentro del contenedor.
      //
      // CONSECUENCIA QUE HAY QUE TENER PRESENTE AL LEER LA REJILLA, medida en el ambiente de
      // pruebas: en este catálogo casi todo producto abre su hoja de modificadores, así que de una
      // captura completa se cuenta el toque que ABRE la hoja y ninguno de los de adentro. La
      // rejilla dice dónde se toca para EMPEZAR algo, no cuántas veces se tocó en total — y eso se
      // dice también al lado de la rejilla, en la consola.
      const destino = e.target;
      if (!(destino instanceof Element)) return;
      if (!destino.closest('[data-medible]')) return;
      if (destino.closest('[role="dialog"]')) return;

      if (bajadas.size >= MAX_CONTACTOS) bajadas.clear();
      bajadas.set(e.pointerId, { x: e.clientX, y: e.clientY });
    };

    const alSubir = (e: PointerEvent) => {
      const origen = bajadas.get(e.pointerId);
      bajadas.delete(e.pointerId);
      // Un `up` sin su `down` —el dedo entró desde fuera de la ventana, o la bajada cayó dentro de
      // una capa— no es un toque de esta pantalla.
      if (!origen) return;
      if (Math.abs(e.clientX - origen.x) > UMBRAL_DE_ARRASTRE_PX) return;
      if (Math.abs(e.clientY - origen.y) > UMBRAL_DE_ARRASTRE_PX) return;

      // Se mide contra el ÁREA VISIBLE (`clientX`/`clientY`, `innerWidth`/`innerHeight`), no contra
      // el documento: lo que este mapa responde es qué parte del vidrio usa la mano. En una
      // pantalla que se desplaza, la misma zona es contenido distinto en momentos distintos — y qué
      // control se tocó ya lo responde la 017 con las acciones con nombre.
      const zona = celdaDelToque(e.clientX, e.clientY, window.innerWidth, window.innerHeight);
      if (!zona) return;
      medirToque(pantalla, zona.celda, zona.orientacion);
    };

    // `pointercancel` no es opcional: el navegador se queda con el contacto al empezar un gesto de
    // desplazamiento, y sin este renglón esa bajada se queda en el mapa hasta que otro dedo con el
    // mismo id la pise.
    const alCancelar = (e: PointerEvent) => void bajadas.delete(e.pointerId);

    // En la fase de captura para que una hoja que llama a `stopPropagation` no apague la medición
    // sin que nadie se entere.
    document.addEventListener('pointerdown', alBajar, true);
    document.addEventListener('pointerup', alSubir, true);
    document.addEventListener('pointercancel', alCancelar, true);
    return () => {
      document.removeEventListener('pointerdown', alBajar, true);
      document.removeEventListener('pointerup', alSubir, true);
      document.removeEventListener('pointercancel', alCancelar, true);
    };
  }, [midiendo, pantalla]);

  return null;
}
