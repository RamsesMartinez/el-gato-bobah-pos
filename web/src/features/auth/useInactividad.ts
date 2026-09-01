import { useEffect, useRef, useState } from 'react';
import type { Almacen } from './inactividad';
import { proximoVencimiento, marcarBloqueada, estabaBloqueada, limpiarBloqueo } from './inactividad';

// Eventos que cuentan como "alguien está usando la tableta".
//
// Se escuchan en captura y de forma pasiva: en captura para que un componente que detenga la
// propagación no deje el reloj corriendo, y pasiva para no estorbar el desplazamiento táctil, que
// es la interacción más frecuente del POS.
const ACTIVIDAD = ['pointerdown', 'keydown', 'wheel', 'touchstart'] as const;

// Bloquea la pantalla tras un rato sin actividad.
//
// El temporizador vive en el CLIENTE y no habla con el servidor: mandarle cada toque de cada
// tableta sería tráfico constante para proteger de un escenario —alguien con la tableta en la
// mano— que el bloqueo de pantalla ya cubre. La barrera real es la caducidad de la sesión, y esa
// sí la aplica el servidor.
export function useInactividad(segundos: number, activo: boolean): { bloqueado: boolean; desbloquear: () => void } {
  // Arranca leyendo la marca, no en false: si arrancara en false, bastaría F5 —o el
  // pull-to-refresh de la PWA— para saltarse el bloqueo entero, porque el canje del refresh
  // devuelve la sesión completa del operador anterior sin pedir PIN.
  const [bloqueado, setBloqueado] = useState(() => estabaBloqueada(almacen()));
  // Arranca en 0 y NO en Date.now(): leer el reloj durante el render es impuro y da resultados
  // distintos en cada re-render. Se siembra al montar el efecto, antes de que el intervalo corra.
  const ultimaActividad = useRef(0);

  useEffect(() => {
    if (!activo || bloqueado) return;

    const marcar = () => { ultimaActividad.current = Date.now(); };
    marcar();
    for (const ev of ACTIVIDAD) {
      window.addEventListener(ev, marcar, { capture: true, passive: true });
    }

    // Se revisa cada segundo en vez de programar un temporizador exacto: si la tableta se suspende,
    // el temporizador exacto dispara tarde y la pantalla se queda abierta justo cuando nadie está.
    // Comparar contra el reloj en cada tick lo detecta al volver.
    const tick = window.setInterval(() => {
      const vence = proximoVencimiento(ultimaActividad.current, segundos);
      if (vence !== null && Date.now() >= vence) {
        marcarBloqueada(almacen());
        setBloqueado(true);
      }
    }, 1000);

    return () => {
      for (const ev of ACTIVIDAD) window.removeEventListener(ev, marcar, { capture: true });
      window.clearInterval(tick);
    };
  }, [segundos, activo, bloqueado]);

  return {
    bloqueado,
    desbloquear: () => {
      ultimaActividad.current = Date.now();
      limpiarBloqueo(almacen());
      setBloqueado(false);
    },
  };
}

// almacen: sessionStorage, o uno inerte si el navegador no lo da.
//
// sessionStorage y no localStorage: la marca tiene que morir con la pestaña. Con localStorage, una
// tableta bloqueada quedaría bloqueada incluso después de cerrar y reabrir el navegador, y el
// operador tendría que borrar datos del sitio para volver a trabajar.
function almacen(): Almacen {
  try {
    return window.sessionStorage;
  } catch {
    // Sin almacén, estabaBloqueada() devuelve true al leer y el bloqueo sigue valiendo: el modo de
    // fallo de una protección tiene que ser proteger.
    return { getItem: () => { throw new Error('sin almacén'); }, setItem: () => {}, removeItem: () => {} };
  }
}
