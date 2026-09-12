import { useEffect, useState } from 'react';
import { mapaDeUso, type Empresa, type MapaDeUso as Mapa, type PantallaDeUso } from './api';
import { etiquetaDeAccion, etiquetaDePantalla } from './etiquetas-de-uso';

// EL MAPA DE CALOR DE USO (spec 017), pintado con CSS propio.
//
// UN SOLO EJE: las pantallas ordenadas de más a menos usada, y sus acciones desplegables debajo. No
// es una rejilla de días tipo calendario de contribuciones — la pregunta que esta pantalla responde
// es cuál se usa mucho y cuál no usa nadie, y eso se contesta con un eje.
//
// Cero librerías de gráficas: una escala de color son cuatro líneas de CSS, y una dependencia trae
// su calendario de versiones y su CVE que bloquea el merge. Y el número va ESCRITO dentro de la
// celda: una escala de color sola es ilegible para quien no distingue esos tonos, y obliga a
// comparar a ojo cuánto es «más oscuro».

const PERIODOS = [
  { dias: 1, nombre: 'Hoy' },
  { dias: 7, nombre: '7 días' },
  { dias: 30, nombre: '30 días' },
  { dias: 90, nombre: '90 días' },
];

function haceDias(n: number): string {
  const d = new Date();
  d.setDate(d.getDate() - (n - 1));
  return d.toISOString().slice(0, 10);
}

function hoy(): string {
  return new Date().toISOString().slice(0, 10);
}

export function MapaDeUso({ empresas }: { empresas: Empresa[] }) {
  const [dias, setDias] = useState(7);
  const [empresa, setEmpresa] = useState<number | undefined>(undefined);
  const [abierta, setAbierta] = useState<string | null>(null);

  // El resultado viaja CON la clave de la consulta que lo produjo, en un solo estado.
  //
  // Así «cargando» se deduce comparando claves en vez de poner el mapa en null al entrar al efecto
  // —que además de ser un setState síncrono dentro de un efecto, deja ver por un instante los datos
  // del periodo anterior como si fueran los del nuevo—.
  const [resultado, setResultado] = useState<{ clave: string; mapa: Mapa | null; error: string }>({
    clave: '',
    mapa: null,
    error: '',
  });
  const clave = `${dias}|${empresa ?? 'todas'}`;
  const cargando = resultado.clave !== clave;
  const mapa = cargando ? null : resultado.mapa;
  const error = cargando ? '' : resultado.error;

  useEffect(() => {
    let vigente = true;
    mapaDeUso(haceDias(dias), hoy(), empresa)
      .then((m) => {
        if (vigente) setResultado({ clave: `${dias}|${empresa ?? 'todas'}`, mapa: m, error: '' });
      })
      .catch((err: unknown) => {
        if (!vigente) return;
        setResultado({
          clave: `${dias}|${empresa ?? 'todas'}`,
          mapa: null,
          error: err instanceof Error ? err.message : 'No se pudo cargar el uso.',
        });
      });
    return () => {
      vigente = false;
    };
  }, [dias, empresa]);

  // `?? []` y no `mapa.pantallas` a secas: la consola no tiene frontera de error, así que una
  // respuesta con otra forma —un proxy que devuelve HTML, una versión vieja de la API— tumbaría la
  // pantalla entera, lista de empresas incluida, en vez de mostrar un mapa vacío.
  const pantallas = mapa?.pantallas ?? [];
  const conUso = pantallas.filter((p) => total(p) > 0);
  const maximo = conUso.length > 0 ? total(conUso[0]) : 0;

  return (
    <section className="panel">
      <div className="filtros">
        <div className="grupo" role="group" aria-label="Periodo">
          {PERIODOS.map((p) => (
            <button
              key={p.dias}
              className={p.dias === dias ? 'chip activo' : 'chip'}
              onClick={() => setDias(p.dias)}
            >
              {p.nombre}
            </button>
          ))}
        </div>
        <div className="grupo" role="group" aria-label="Empresa">
          <button
            className={empresa === undefined ? 'chip activo' : 'chip'}
            onClick={() => setEmpresa(undefined)}
          >
            Todas
          </button>
          {empresas.map((e) => (
            <button
              key={e.id}
              className={empresa === e.id ? 'chip activo' : 'chip'}
              onClick={() => setEmpresa(e.id)}
            >
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
      {!mapa && !error && <p className="vacio">Cargando…</p>}

      {mapa && conUso.length === 0 && (
        // FR-015: con cero datos se dice, no se pinta un mapa vacío que se lea como una falla.
        <p className="vacio">Todavía no hay uso registrado en este periodo.</p>
      )}

      {mapa && conUso.length > 0 && (
        <>
          <table className="mapa">
            <thead>
              <tr>
                <th>Pantalla</th>
                <th>Veces</th>
                <th>Por rol</th>
              </tr>
            </thead>
            <tbody>
              {pantallas.map((p) => (
                <Renglon
                  key={p.pantalla}
                  pantalla={p}
                  maximo={maximo}
                  abierta={abierta === p.pantalla}
                  alAbrir={() => setAbierta(abierta === p.pantalla ? null : p.pantalla)}
                />
              ))}
            </tbody>
          </table>
          <p className="tenue">
            «Sin corte» es uso que existe pero no se puede atribuir a un rol sin señalar a una
            persona.
          </p>
        </>
      )}
    </section>
  );
}

function Renglon({
  pantalla,
  maximo,
  abierta,
  alAbrir,
}: {
  pantalla: PantallaDeUso;
  maximo: number;
  abierta: boolean;
  alAbrir: () => void;
}) {
  const veces = total(pantalla);
  return (
    <>
      <tr className={veces === 0 ? 'apagada' : undefined}>
        <td>
          {pantalla.acciones.length > 0 ? (
            <button className="desplegar" onClick={alAbrir} aria-expanded={abierta}>
              <span aria-hidden="true">{abierta ? '▾ ' : '▸ '}</span>
              {etiquetaDePantalla(pantalla.pantalla)}
            </button>
          ) : (
            etiquetaDePantalla(pantalla.pantalla)
          )}
        </td>
        <td>
          <Celda veces={veces} maximo={maximo} />
        </td>
        <td className="tenue">
          {pantalla.porRol.length === 0
            ? '—'
            : pantalla.porRol.map((r) => `${r.rol ?? 'sin corte'} ${r.veces}`).join(' · ')}
        </td>
      </tr>
      {abierta &&
        pantalla.acciones.map((a) => (
          <tr key={a.accion} className="accion">
            <td>↳ {etiquetaDeAccion(a.accion)}</td>
            <td>
              <Celda veces={a.veces} maximo={maximo} />
            </td>
            <td />
          </tr>
        ))}
    </>
  );
}

// Celda: la intensidad dice «mucho o poco» de un vistazo y el NÚMERO dice cuánto.
//
// Los dos, no uno: el color solo es ilegible para quien no distingue esos tonos y obliga a comparar
// a ojo; el número solo obliga a leer la tabla entera para saber cuál destaca.
function Celda({ veces, maximo }: { veces: number; maximo: number }) {
  const intensidad = maximo > 0 ? veces / maximo : 0;
  return (
    <span className="celda" style={{ backgroundColor: `rgba(226, 59, 46, ${0.08 + intensidad * 0.7})` }}>
      {veces}
    </span>
  );
}

function total(p: PantallaDeUso): number {
  return p.aperturas + p.acciones.reduce((s, a) => s + a.veces, 0);
}
