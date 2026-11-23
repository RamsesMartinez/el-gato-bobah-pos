import { readFileSync } from 'node:fs';
import { expect, it } from 'vitest';
import { SIMBOLOS, familia } from './sprite';

it('el sprite en línea es el mismo que el del POS', () => {
  const pos = readFileSync('public/ilustraciones/food-families.v1.svg', 'utf8');
  expect(SIMBOLOS).toBe((pos.match(/<symbol[\s\S]*?<\/symbol>/g) ?? []).join(''));
});

it('una familia vacía o desconocida dibuja la genérica', () => {
  expect(familia(null)).toBe('dish');
  expect(familia('pizza')).toBe('dish');
  expect(familia('coffee')).toBe('coffee');
});
