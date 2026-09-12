import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { useSessionStore } from '../stores/session';
import { _soloParaPruebas, medirAccion, medirPantalla, medirToque, vaciarCola } from './uso';

// NUNCA UNA CAPTURA, NUNCA EL CONTENIDO DE LA PANTALLA (FR-009 de la 019).
//
// Es la mitad de la feature, según el spec, y la que no se puede corregir después: una captura del
// POS lleva nombres de clientes, el contenido de un pedido y sus importes, y eso no puede salir del
// local. La tentación es concreta y razonable —pintar las manchas encima de la pantalla real se
// entiende mejor—, así que el guardia tiene que atrapar la FORMA antes de que exista, no el valor
// después de que salió.
//
// Dos mitades, porque ninguna sola alcanza:
//  1. Estática: nadie escribió la forma prohibida en los archivos que arman el envío.
//  2. De ejecución: lo que de verdad viaja en el cuerpo son las llaves declaradas y ninguna más.

// Se listan los que arman el envío, y los medidores se descubren solos: un `MedidorDeAlgo.tsx`
// nuevo queda cubierto sin que nadie se acuerde de agregarlo aquí. Un guardia que hay que
// actualizar a mano es un guardia que se queda atrás en el primer archivo nuevo.
function archivosQueArmanElEnvio(): string[] {
  const medidores = readdirSync('src/app')
    .filter((n) => /^Medidor[^.]*\.tsx$/.test(n))
    .map((n) => `src/app/${n}`);
  return ['src/api/uso.ts', 'src/app/celda.ts', 'src/app/rutas-medidas.ts', ...medidores];
}

// Cada forma con su porqué, para que quien la agregue sepa qué se le está negando y con qué
// alternativa: casi siempre la respuesta es «cuéntalo con una acción con nombre» (spec 017).
const FORMAS_PROHIBIDAS: { patron: RegExp; porque: string }[] = [
  { patron: /toDataURL|toBlob\s*\(/, porque: 'convertir la pantalla en imagen' },
  { patron: /getImageData|drawImage|html2canvas|createElement\(\s*['"]canvas/, porque: 'dibujar la pantalla' },
  { patron: /getDisplayMedia|captureStream/, porque: 'grabar la pantalla' },
  { patron: /\.(innerHTML|outerHTML|innerText|textContent)\b/, porque: 'leer el texto de la pantalla' },
  { patron: /document\.title/, porque: 'el título lleva el nombre del cliente en varias pantallas' },
  { patron: /\.(value|placeholder)\s*[,)\]}]/, porque: 'leer lo que el operador escribió' },
  { patron: /location\.(href|search)|window\.location\b/, porque: 'la dirección lleva folios e identificadores' },
];

describe('la medición nunca lleva la pantalla', () => {
  it('ningún archivo que arma el envío usa una forma que capture contenido', () => {
    const culpables: string[] = [];
    for (const archivo of archivosQueArmanElEnvio()) {
      const src = readFileSync(archivo, 'utf8');
      src.split('\n').forEach((linea, i) => {
        // Los comentarios explican justamente lo que NO se hace; nombrarlo no es hacerlo.
        const codigo = linea.replace(/\/\/.*$/, '');
        for (const { patron, porque } of FORMAS_PROHIBIDAS) {
          if (patron.test(codigo)) culpables.push(`${archivo}:${i + 1} (${porque}): ${linea.trim()}`);
        }
      });
    }
    expect(culpables, 'una captura del POS lleva nombres de clientes e importes, y no puede salir del local').toEqual([]);
  });

  it('la lista de archivos vigilados no se quedó vacía', () => {
    // Sin esto, renombrar `uso.ts` deja el guardia pasando en verde sobre cero archivos, que es la
    // forma más silenciosa de perder una prueba.
    expect(archivosQueArmanElEnvio().length).toBeGreaterThanOrEqual(3);
  });
});

describe('lo que de verdad viaja en el cuerpo', () => {
  let cuerpos: unknown[];

  beforeEach(() => {
    _soloParaPruebas.reiniciar();
    cuerpos = [];
    useSessionStore.setState({ token: 'token-de-prueba' });
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init: RequestInit) => {
        cuerpos.push(JSON.parse(String(init.body)));
        return Promise.resolve(new Response(null, { status: 204 }));
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    _soloParaPruebas.reiniciar();
  });

  it('el cuerpo solo tiene las llaves declaradas, y cada evento y cada toque solo las suyas', () => {
    medirPantalla('pos');
    medirAccion('pos', 'cobrar');
    medirToque('pos', 37, 'horizontal');
    vaciarCola();

    expect(cuerpos).toHaveLength(1);
    const cuerpo = cuerpos[0] as Record<string, unknown>;
    // Se comprueba contra la lista de lo PERMITIDO y no contra la de lo prohibido: así una llave
    // nueva —la que alguien agregue el día que quiera «solo un poquito de contexto»— rompe el test
    // sin que nadie haya tenido que anticiparla.
    expect(Object.keys(cuerpo).sort()).toEqual(['eventos', 'toques']);

    for (const e of cuerpo.eventos as Record<string, unknown>[]) {
      for (const llave of Object.keys(e)) {
        expect(['pantalla', 'accion'], `el evento viajó con la llave "${llave}"`).toContain(llave);
      }
      expect(typeof e.pantalla).toBe('string');
    }

    // EL TOQUE NO LLEVA UN PUNTO. Es la promesa entera de la 019: lo que viaja es el número de
    // celda, ya redondeado en la tableta. Un `x` o un `y` aquí —aunque el servidor los ignorara—
    // existirían en el cuerpo del request y en el log de cualquier proxy del camino.
    const toques = cuerpo.toques as Record<string, unknown>[];
    expect(toques).toHaveLength(1);
    for (const llave of Object.keys(toques[0])) {
      expect(['pantalla', 'celda', 'orientacion'], `el toque viajó con la llave "${llave}"`).toContain(llave);
    }
    expect(Number.isInteger(toques[0].celda)).toBe(true);
  });
});
