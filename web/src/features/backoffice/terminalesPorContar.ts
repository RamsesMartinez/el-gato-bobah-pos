export interface TerminalPorContar { terminalId: number; name: string }

// faltanTerminales: las terminales sin cifra capturada. Un campo vacío no es cero (`Number('')` sí).
export function faltanTerminales(terminales: TerminalPorContar[], valores: Record<number, string>): string[] {
  return terminales.filter((t) => {
    const v = (valores[t.terminalId] ?? '').trim();
    return v === '' || !Number.isFinite(Number(v)) || Number(v) < 0;
  }).map((t) => t.name);
}
