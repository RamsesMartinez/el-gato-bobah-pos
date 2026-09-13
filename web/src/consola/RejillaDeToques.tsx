import { useEffect, useState } from 'react';
import {
  rejillaDeToques,
  type Empresa,
  type OrientacionDeToque,
  type RejillaDeToques as Rejilla,
} from './api';
import { etiquetaDeZona, zonasDelPos, FECHA_DEL_LAYOUT } from './zonas-del-pos';

// DÓNDE CAE EL DEDO (spec 019), pintado con CSS propio.
//
// La rejilla se pinta SOLA: la proporción de la pantalla y nada más. Pintar las manchas encima de
// una captura se entendería mejor, y por eso está prohibido — una captura del POS de un cliente
// lleva nombres, el contenido de sus pedidos e importes, y eso no puede salir del local (FR-009).
// Lo que reemplaza a la captura es la leyenda de texto, fechada, al lado.
//
// Cero librerías de gráficas, como en el mapa de la 017: una escala de color son cuatro líneas de
// CSS, y una dependencia trae su calendario de versiones y su CVE que bloquea el merge.

const PERIODOS = [
  { dias: 7, nombre: '7 días' },
  { dias: 30, nombre: '30 días' },
  // 92 y no 90: es la retención completa de la rejilla. Pedir más devuelve 400, y un botón que
  // devuelve un error no va en la pantalla.
  { dias: 92, nombre: '92 días' },
];

const ORIENTACIONES: { valor: OrientacionDeToque; nombre: string }[] = [
  { valor: 'horizontal', nombre: 'Horizontal' },
  { valor: 'vertical', nombre: 'Vertical' },
];

function haceDias(n: number): string {
  const d = new Date();
  d.setDate(d.getDate() - (n - 1));
  return d.toISOString().slice(0, 10);
}

function hoy(): string {
  return new Date().toISOString().slice(0, 10);
}

export function RejillaDeToques({ empresas }: { empresas: Empresa[] }) {
  const [dias, setDias] = useState(7);
  const [orientacion, setOrientacion] = useState<OrientacionDeToque>('horizontal');
  const [empresa, setEmpresa] = useState<number | undefined>(undefined);

  // El resultado viaja CON la clave de la consulta que lo produjo, igual que en el mapa de la 017:
  // así «cargando» se deduce comparando claves, en vez de dejar ver por un instante los datos de la
  // orientación anterior como si fueran los de la nueva — que aquí sería peor, porque la rejilla se
  // vería normal describiendo otra forma de pantalla.
  const [resultado, setResultado] = useState<{ clave: string; rejilla: Rejilla | null; error: string }>({
    clave: '',
    rejilla: null,
    error: '',
  });
  const clave = `${dias}|${orientacion}|${empresa ?? 'todas'}`;
  const cargando = resultado.clave !== clave;
  const rejilla = cargando ? null : resultado.rejilla;
  const error = cargando ? '' : resultado.error;

  useEffect(() => {
    let vigente = true;
    const suClave = `${dias}|${orientacion}|${empresa ?? 'todas'}`;
    rejillaDeToques('pos', orientacion, haceDias(dias), hoy(), empresa)
      .then((r) => {
        if (vigente) setResultado({ clave: suClave, rejilla: r, error: '' });
      })
      .catch((err: unknown) => {
        if (!vigente) return;
        setResultado({
          clave: suClave,
          rejilla: null,
          error: err instanceof Error ? err.message : 'No se pudieron cargar los toques.',
        });
      });
    return () => {
      vigente = false;
    };
  }, [dias, orientacion, empresa]);

  // `?? []` y no `rejilla.celdas` a secas: la consola no tiene frontera de error, así que una
  // respuesta con otra forma —un proxy que devuelve HTML, una versión vieja de la API— tumbaría la
  // pantalla entera en vez de mostrar una rejilla vacía.
  const celdas = rejilla?.celdas ?? [];
  const columnas = rejilla?.rejilla.columnas ?? 12;
  const maximo = celdas.reduce((m, c) => (c.veces > m ? c.veces : m), 0);

  return (
    <section className="panel">
      <h2>Dónde cae el dedo</h2>
      <div className="filtros">
        <div className="grupo" role="group" aria-label="Periodo">
          {PERIODOS.map((p) => (
            <button key={p.dias} className={p.dias === dias ? 'chip activo' : 'chip'} onClick={() => setDias(p.dias)}>
              {p.nombre}
            </button>
          ))}
        </div>
        <div className="grupo" role="group" aria-label="Orientación">
          {ORIENTACIONES.map((o) => (
            <button
              key={o.valor}
              className={o.valor === orientacion ? 'chip activo' : 'chip'}
              onClick={() => setOrientacion(o.valor)}
            >
              {o.nombre}
            </button>
          ))}
        </div>
        <div className="grupo" role="group" aria-label="Empresa">
          <button className={empresa === undefined ? 'chip activo' : 'chip'} onClick={() => setEmpresa(undefined)}>
            Todas
          </button>
          {empresas.map((e) => (
            <button key={e.id} className={empresa === e.id ? 'chip activo' : 'chip'} onClick={() => setEmpresa(e.id)}>
              {e.name}
            </button>
          ))}
        </div>
      </div>

      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      {!rejilla && !error && <p className="vacio">Cargando…</p>}

      {rejilla && maximo === 0 && (
        <p className="vacio">Todavía no hay toques registrados en este periodo.</p>
      )}

      {rejilla && maximo > 0 && (
        <div className="toques">
          <div
            className="rejilla"
            role="grid"
            aria-label={`Toques por zona de la pantalla del punto de venta, ${rejilla.orientacion}`}
            style={{ gridTemplateColumns: `repeat(${columnas}, 1fr)` }}
          >
            {celdas.map((c) => (
              <Zona key={c.celda} celda={c.celda} veces={c.veces} maximo={maximo} columnas={columnas} />
            ))}
          </div>

          {/* LA LEYENDA reemplaza a la captura que no se puede pintar. Va fechada: el día que el POS
              se rediseñe, describe un layout que ya no existe. */}
          <div className="leyenda">
            <h3>Qué hay en cada zona</h3>
            <ul>
              {zonasDelPos(rejilla.orientacion).map((z) => (
                <li key={`${z.filas[0]}-${z.columnas[0]}`}>{etiquetaDeZona(z)}</li>
              ))}
            </ul>
            <p className="tenue">Según la pantalla del {FECHA_DEL_LAYOUT}.</p>
            {/* Lo que el número NO incluye, y sin decirlo se lee al revés: medido en el ambiente de
                pruebas, tocar un producto abre su hoja de modificadores, así que de una captura
                completa la rejilla ve el toque que ABRE y ninguno de los de adentro. */}
            <p className="tenue">
              Cuenta el toque que abre una acción. Lo que se toca dentro de una hoja o un diálogo no
              entra aquí.
            </p>

            <h3>Por rol</h3>
            <p className="tenue">
              {rejilla.porRol.length === 0
                ? '—'
                : rejilla.porRol.map((r) => `${r.rol ?? 'sin corte'} ${r.veces}`).join(' · ')}
            </p>
            {/* La misma nota que el mapa de la 017, porque la duda es la misma: sin ella «sin
                corte» se lee como un error de captura. */}
            <p className="tenue">
              «Sin corte» son toques que existen pero no se pueden atribuir a un rol sin señalar a
              una persona.
            </p>
          </div>
        </div>
      )}
    </section>
  );
}

function Zona({
  celda,
  veces,
  maximo,
  columnas,
}: {
  celda: number;
  veces: number;
  maximo: number;
  columnas: number;
}) {
  const fila = Math.floor(celda / columnas);
  const columna = celda % columnas;
  // Raíz cuadrada y no proporción directa: con una zona muy caliente —la del botón de cobrar—,
  // todo lo demás quedaría casi blanco y la rejilla diría «solo se usa una zona», que es falso.
  const intensidad = maximo > 0 ? Math.sqrt(veces / maximo) : 0;
  return (
    <div
      role="gridcell"
      className={veces === 0 ? 'zona apagada' : 'zona'}
      // El número va ESCRITO además del tono, por lo mismo que en el mapa de la 017: una escala de
      // color sola es ilegible para quien no distingue esos tonos.
      aria-label={`Fila ${fila}, columna ${columna}: ${veces} toques`}
      style={{ background: `rgba(226, 59, 46, ${0.06 + intensidad * 0.8})` }}
    >
      {veces > 0 ? veces : ''}
    </div>
  );
}
