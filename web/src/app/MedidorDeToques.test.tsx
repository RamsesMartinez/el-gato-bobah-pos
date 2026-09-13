import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { MedidorDeToques } from './MedidorDeToques';
import { celdaDelToque } from './celda';

vi.mock('../api/uso', () => ({ medirToque: vi.fn() }));
import { medirToque } from '../api/uso';

// EL ESCUCHADOR DE TOQUES (US3 de la 019).
//
// Lo que estos tests protegen no es el cálculo —ése vive en `celda.ts` y se prueba en tabla— sino
// las cuatro formas de contar de más, que son las que ensucian la rejilla sin que nada falle:
// contar un arrastre, mezclar dos dedos, contar lo que pasó encima de una capa, y contar una
// pantalla que el servidor va a tirar.

function toque(tipo: 'pointerdown' | 'pointerup', destino: Element, x: number, y: number, pointerId = 1) {
  // jsdom no trae `PointerEvent`, así que se arma el evento a mano con lo que el escuchador lee.
  const ev = new Event(tipo, { bubbles: true });
  Object.assign(ev, { clientX: x, clientY: y, pointerId });
  destino.dispatchEvent(ev);
}

// El contenedor `data-medible` es lo que marca qué es «la pantalla»: en la aplicación lo pone
// AppShell, y aquí se monta igual porque sin él no se cuenta nada — que es justo la garantía.
function montar(ruta = '/pos') {
  const r = render(
    <MemoryRouter initialEntries={[ruta]}>
      <MedidorDeToques />
      <div data-medible data-testid="pantalla" />
    </MemoryRouter>,
  );
  return r;
}

function enLaPantalla(el: Element): Element {
  document.querySelector('[data-medible]')!.appendChild(el);
  return el;
}

describe('MedidorDeToques', () => {
  beforeEach(() => {
    vi.mocked(medirToque).mockClear();
    document.body.innerHTML = '';
  });
  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('un toque cuenta, con la celda que le toca', () => {
    montar();
    const boton = enLaPantalla(document.createElement('button'));

    toque('pointerdown', boton, 300, 200);
    toque('pointerup', boton, 302, 201);

    const esperada = celdaDelToque(302, 201, window.innerWidth, window.innerHeight)!;
    expect(medirToque).toHaveBeenCalledTimes(1);
    expect(medirToque).toHaveBeenCalledWith('pos', esperada.celda, esperada.orientacion);
  });

  // Sin esto, cada desplazamiento de la lista de productos deja un rastro y la rejilla mide scroll
  // en vez de intención: la columna por donde se arrastra saldría como la más usada del POS.
  it('un arrastre de más de 10 px NO cuenta', () => {
    montar();
    const lista = enLaPantalla(document.createElement('div'));

    toque('pointerdown', lista, 300, 400);
    toque('pointerup', lista, 300, 200);

    expect(medirToque).not.toHaveBeenCalled();
  });

  // EN EL MOSTRADOR HAY DOS MANOS: una sostiene comida y la otra toca. Con un solo estado
  // compartido, el `down` de un dedo se compara con el `up` del otro y los dos salen como
  // arrastres — se pierden los dos toques y nada falla.
  it('dos contactos simultáneos no se mezclan', () => {
    montar();
    const a = enLaPantalla(document.createElement('button'));
    const b = enLaPantalla(document.createElement('button'));

    toque('pointerdown', a, 100, 100, 1);
    toque('pointerdown', b, 700, 400, 2);
    toque('pointerup', a, 103, 102, 1);
    toque('pointerup', b, 702, 403, 2);

    expect(medirToque).toHaveBeenCalledTimes(2);
  });

  // EL PEOR CONTAMINANTE. Hojas, diálogos y el bloqueo por PIN no cambian de ruta: son capas encima
  // de la misma pantalla, así que sus toques se atribuirían a la de abajo. El teclado del PIN
  // siempre cae en el mismo sitio y dejaría una zona caliente en el centro que dentro de seis meses
  // alguien va a leer como «un control muy usado».
  it('un toque dentro de un [role="dialog"] no cuenta', () => {
    montar();
    const hoja = enLaPantalla(document.createElement('div'));
    hoja.setAttribute('role', 'dialog');
    const tecla = document.createElement('button');
    hoja.appendChild(tecla);

    toque('pointerdown', tecla, 500, 300);
    toque('pointerup', tecla, 500, 300);

    expect(medirToque).not.toHaveBeenCalled();
  });

  // LA LISTA BLANCA TAMBIÉN CORRE EN LA TABLETA. El escuchador vive en la raíz y ve toda la
  // aplicación: sin filtrar aquí, encolaría toques de pantallas no instrumentadas durante todo el
  // turno para que el servidor los tire — wifi del mostrador gastado en nada.
  it('una pantalla que no está instrumentada no manda nada', () => {
    montar('/caja');
    const boton = enLaPantalla(document.createElement('button'));

    toque('pointerdown', boton, 300, 200);
    toque('pointerup', boton, 300, 200);

    expect(medirToque).not.toHaveBeenCalled();
  });

  // Un `up` sin su `down` —el dedo entró desde fuera de la ventana, o el navegador canceló el
  // contacto— no puede contar como toque en el lugar donde se levantó.
  // EL AVISO FLOTANTE, que es el portal que ya existía y caía sobre la zona del botón de cobrar.
  // Se anuncia con `role="status"`, no con `role="dialog"`, así que una lista de lo prohibido no lo
  // atrapaba: con el filtro invertido, lo que no cuelga del contenedor no cuenta y punto.
  it('lo que se pinta fuera del contenedor medible no cuenta', () => {
    montar();
    const aviso = document.createElement('div');
    aviso.setAttribute('role', 'status');
    const deshacer = document.createElement('button');
    aviso.appendChild(deshacer);
    document.body.appendChild(aviso);

    toque('pointerdown', deshacer, 950, 560);
    toque('pointerup', deshacer, 950, 560);

    expect(medirToque).not.toHaveBeenCalled();
  });

  it('un up sin su down no cuenta', () => {
    montar();
    const boton = enLaPantalla(document.createElement('button'));

    toque('pointerup', boton, 300, 200, 9);

    expect(medirToque).not.toHaveBeenCalled();
  });

  it('al desmontar deja de escuchar', () => {
    const { unmount } = montar();
    const boton = enLaPantalla(document.createElement('button'));
    unmount();

    toque('pointerdown', boton, 300, 200);
    toque('pointerup', boton, 300, 200);

    expect(medirToque).not.toHaveBeenCalled();
  });

  // EL DEDO NO SE ENTERA (SC-004). El escuchador va en la fase de captura, que es justo donde un
  // manejador mal escrito puede tragarse el evento antes de que llegue al botón: cancelarlo o
  // detener su propagación dejaría al operador tocando «Cobrar» sin que pase nada, y el defecto
  // aparecería solo con la medición encendida.
  describe('no estorba', () => {
    it('un toque sobre un botón sigue disparando su onClick', () => {
      montar();
      const boton = document.createElement('button');
      const cobrar = vi.fn();
      boton.addEventListener('click', cobrar);
      document.body.appendChild(boton);

      toque('pointerdown', boton, 300, 200);
      toque('pointerup', boton, 300, 200);
      boton.dispatchEvent(new Event('click', { bubbles: true }));

      expect(cobrar).toHaveBeenCalledTimes(1);
    });

    it('no cancela ni detiene el evento, ni siquiera el que sí cuenta', () => {
      montar();
      const boton = enLaPantalla(document.createElement('button'));
      const propio = vi.fn();
      boton.addEventListener('pointerup', propio);

      const bajada = new Event('pointerdown', { bubbles: true, cancelable: true });
      Object.assign(bajada, { clientX: 300, clientY: 200, pointerId: 1 });
      boton.dispatchEvent(bajada);
      const subida = new Event('pointerup', { bubbles: true, cancelable: true });
      Object.assign(subida, { clientX: 300, clientY: 200, pointerId: 1 });
      boton.dispatchEvent(subida);

      expect(medirToque).toHaveBeenCalledTimes(1);
      expect(bajada.defaultPrevented, 'cancelar el pointerdown rompe el foco y el scroll').toBe(false);
      expect(subida.defaultPrevented).toBe(false);
      expect(propio, 'el manejador del control tiene que seguir recibiendo el evento').toHaveBeenCalledTimes(1);
    });

    // El desplazamiento de una lista es exactamente el gesto que el umbral descarta, y descartarlo
    // como medición no puede ser estorbarlo como gesto.
    it('el desplazamiento de una lista no se toca', () => {
      montar();
      const lista = enLaPantalla(document.createElement('div'));
      const seDesplazo = vi.fn();
      lista.addEventListener('pointermove', seDesplazo);

      toque('pointerdown', lista, 300, 400);
      const mover = new Event('pointermove', { bubbles: true, cancelable: true });
      Object.assign(mover, { clientX: 300, clientY: 300, pointerId: 1 });
      lista.dispatchEvent(mover);
      toque('pointerup', lista, 300, 200);

      expect(seDesplazo).toHaveBeenCalledTimes(1);
      expect(mover.defaultPrevented).toBe(false);
      expect(medirToque).not.toHaveBeenCalled();
    });
  });
});
