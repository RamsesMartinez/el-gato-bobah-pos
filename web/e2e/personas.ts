import { randomBytes, randomInt } from 'node:crypto';
import { API } from './ambiente';

// Personas que una prueba necesita y que no puede tomar de la base: un respaldo restaurado trae a
// la gente del negocio, con otros nombres y otros ids que el ambiente de pruebas. La prueba las crea
// con nombre marcado como de prueba, un PIN y una contraseña que nacen aquí y no se escriben en
// ningún archivo, y las da de baja al terminar.

export type PersonaDePrueba = { id: number; name: string; username: string; role: string; pin: string; password: string };

// pinNuevo: un PIN que el servidor no rechaza por débil, sin dígitos repetidos ni consecutivos.
export function pinNuevo(): string {
  for (;;) {
    const d = [0, 1, 2, 3].map(() => randomInt(10));
    if (new Set(d).size === 4 && d.every((x, i) => i === 0 || Math.abs(x - d[i - 1]) !== 1)) return d.join('');
  }
}

// altaDePrueba da de alta a una persona activa. Nace con cambio de contraseña obligatorio: si la
// prueba necesita entrar con ella, lo hace quien la llama.
export async function altaDePrueba(admin: string, nombre: string, role: string): Promise<PersonaDePrueba> {
  const username = `e2e-${nombre.toLowerCase()}-${randomBytes(3).toString('hex')}`;
  const password = `Ini-${randomBytes(9).toString('hex')}!`;
  const pin = pinNuevo();
  const name = `${nombre} (prueba e2e)`;
  const r = await fetch(`${API}/users`, {
    method: 'POST', headers: { Authorization: `Bearer ${admin}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, username, role, pin, password }),
  });
  if (!r.ok) throw new Error(`POST /users: ${r.status} ${await r.text()}`);
  const { id } = (await r.json()) as { id: number };
  return { id, name, username, role, pin, password };
}

// bajaDePrueba la deja inactiva: no se borra, porque lo que hizo (una propina, un turno) apunta a ella.
export async function bajaDePrueba(admin: string, p: PersonaDePrueba) {
  await fetch(`${API}/users/${p.id}`, {
    method: 'PATCH', headers: { Authorization: `Bearer ${admin}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: p.name, role: p.role, isActive: false }),
  });
}
