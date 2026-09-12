import { useEffect, useState } from 'react';
import {
  empresas as pedirEmpresas,
  entrar,
  olvidarAcceso,
  type Empresa,
  type Empresas,
  type Operador,
} from './api';
import { soloFecha } from './fecha';

// LA CONSOLA DE PLATAFORMA (spec 016).
//
// Dos pantallas: entrar, y ver qué empresas hay. Sin router y sin librería de estado — con dos
// estados posibles, un `useState` dice la verdad y cualquier cosa más grande es infraestructura
// para una pantalla que todavía no existe.
//
// Quien lee esto: esta pantalla NO es del negocio. No lleva nada de lo que un cliente ve, y no
// puede: la conexión que la atiende no tiene permiso para leer pedidos, ventas ni empleados.

export function Consola() {
  const [operador, setOperador] = useState<Operador | null>(null);
  if (!operador) return <Entrar alEntrar={setOperador} />;
  return (
    <Empresas_
      operador={operador}
      alSalir={() => {
        olvidarAcceso();
        setOperador(null);
      }}
    />
  );
}

function Entrar({ alEntrar }: { alEntrar: (o: Operador) => void }) {
  const [usuario, setUsuario] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [enviando, setEnviando] = useState(false);

  async function enviar(e: React.FormEvent) {
    e.preventDefault();
    setEnviando(true);
    setError('');
    try {
      alEntrar(await entrar(usuario, password));
    } catch (err) {
      // El servidor responde lo mismo para usuario inexistente, contraseña equivocada y operador
      // desactivado, a propósito. La pantalla no intenta adivinar cuál fue.
      setError(err instanceof Error ? err.message : 'No se pudo entrar.');
      setEnviando(false);
    }
  }

  return (
    <div className="consola">
      <form className="panel entrar" onSubmit={enviar}>
        <h1>Consola de plataforma</h1>
        {/* role="alert" para que un lector de pantalla ANUNCIE el error: sin él solo cambia el
            DOM y quien no ve la pantalla se queda esperando a que pase algo. */}
        {error && <p className="error" role="alert">{error}</p>}
        <label>
          <span>Usuario</span>
          <input
            value={usuario}
            onChange={(e) => setUsuario(e.target.value)}
            autoComplete="username"
            autoFocus
          />
        </label>
        <label>
          <span>Contraseña</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
        </label>
        <button type="submit" disabled={enviando || usuario === '' || password === ''}>
          {enviando ? 'Entrando…' : 'Entrar'}
        </button>
      </form>
    </div>
  );
}

function Empresas_({ operador, alSalir }: { operador: Operador; alSalir: () => void }) {
  const [datos, setDatos] = useState<Empresas | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    // `vigente` corta lo que llegue tarde: si la respuesta aterriza después de salir, escribirla
    // repintaría la lista de clientes encima de la pantalla de entrar.
    let vigente = true;
    pedirEmpresas()
      .then((d) => {
        if (vigente) setDatos(d);
      })
      .catch((err: unknown) => {
        if (!vigente) return;
        // Un 401 aquí significa que el acceso caducó o que alguien desactivó al operador mientras
        // la pantalla estaba abierta: en los dos casos lo correcto es volver a entrar.
        if (err instanceof Error && 'status' in err && (err as { status: number }).status === 401) {
          alSalir();
          return;
        }
        setError(err instanceof Error ? err.message : 'No se pudo cargar la lista.');
      });
    return () => {
      vigente = false;
    };
  }, [alSalir]);

  return (
    <div className="consola">
      <div className="encabezado">
        <div>
          <h1>Empresas</h1>
          <p className="tenue">
            {operador.name}
            {/* Dice QUÉ es el número: "68" a secas solo lo entiende quien haya abierto
                server/migrations/. Es el dato de la instalación, no de ningún cliente. */}
            {datos ? ` · versión de instalación ${datos.schema.version}` : ''}
          </p>
        </div>
        <button className="salir" onClick={alSalir}>
          Salir
        </button>
      </div>
      {error && <p className="error" role="alert">{error}</p>}
      <div className="panel">
        {datos === null && !error && <p className="vacio">Cargando…</p>}
        {datos !== null && datos.items.length === 0 && (
          <p className="vacio">Todavía no hay ninguna empresa en esta instalación.</p>
        )}
        {datos !== null && datos.items.length > 0 && <Tabla empresas={datos.items} />}
      </div>
    </div>
  );
}

function Tabla({ empresas }: { empresas: Empresa[] }) {
  return (
    <table>
      <thead>
        <tr>
          <th>Empresa</th>
          <th>Identificador</th>
          <th>Desde</th>
        </tr>
      </thead>
      <tbody>
        {empresas.map((e) => (
          <tr key={e.id} className={e.activa ? undefined : 'apagada'}>
            <td>
              {e.name}
              {!e.activa && ' (inactiva)'}
            </td>
            <td>{e.slug}</td>
            <td>{soloFecha(e.createdAt)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
