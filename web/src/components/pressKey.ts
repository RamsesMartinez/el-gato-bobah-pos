// pressKey aplica una tecla a lo tecleado. Sin punto donde no van decimales (piezas, empaques), y
// un solo punto donde sí.
export function pressKey(value: string, key: string, allowDecimal: boolean): string {
  if (key === 'back') return value.slice(0, -1);
  if (key === '.') {
    if (!allowDecimal || value.includes('.')) return value;
    return value === '' ? '0.' : `${value}.`;
  }
  if (!/^\d$/.test(key)) return value;
  if (value === '0') return key; // «07» no es un número que alguien quiera teclear
  return value + key;
}
