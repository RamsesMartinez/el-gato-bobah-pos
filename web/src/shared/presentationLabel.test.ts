import { describe, it, expect } from 'vitest';
import { presentationLabel } from './presentationLabel';

describe('presentationLabel', () => {
  it('con marca, la marca junto al nombre', () => {
    expect(presentationLabel({ name: 'Caja', brand: 'Marca A' })).toBe('Caja · Marca A');
  });
  it('sin marca (o de una API anterior), solo el nombre', () => {
    expect(presentationLabel({ name: 'Caja', brand: null })).toBe('Caja');
    expect(presentationLabel({ name: 'Caja' })).toBe('Caja');
    expect(presentationLabel({ name: 'Caja', brand: '  ' })).toBe('Caja');
  });
});
