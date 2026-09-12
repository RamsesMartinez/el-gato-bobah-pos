import { useEffect, useRef } from 'react';
import { useLocation } from 'react-router';
import { esRecargaDeLaPagina, medirPantalla } from '../api/uso';
import { pantallaDe } from './rutas-medidas';

// CUENTA LAS ABIERTAS DE PANTALLA (spec 017), y nada más.
//
// No pinta nada, no envuelve nada y no puede fallar hacia afuera: vive al lado de las rutas y solo
// mira por dónde va el operador.
export function MedidorDeUso() {
  const { pathname } = useLocation();
  // La PRIMERA vista de una carga por recarga no cuenta: el operador apretó F5 por costumbre y eso
  // no es «volvió a entrar». Se le pregunta al navegador qué tipo de navegación fue, en vez de
  // adivinarlo con un cronómetro — una ventana de N segundos se equivoca por los dos lados: un F5
  // con caché frío tarda más que la ventana, y un cajero que rebota entre dos pantallas en segundos
  // cuenta una sola vez.
  const saltarLaPrimera = useRef(esRecargaDeLaPagina());

  useEffect(() => {
    if (saltarLaPrimera.current) {
      saltarLaPrimera.current = false;
      return;
    }
    const pantalla = pantallaDe(pathname);
    if (pantalla) medirPantalla(pantalla);
  }, [pathname]);

  return null;
}
