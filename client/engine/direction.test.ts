import { describe, expect, it } from 'vitest';
import { directionIndex, facingOf, toClipDir } from './direction';

describe('facingOf', () => {
  // Экран: +x тайла идёт вниз-вправо, +y — вниз-влево. Нумерация: 0 вниз, по часовой стрелке на экране.
  const cases: [string, number, number, number][] = [
    ['вниз (x+, y+)', 1, 1, 0],
    ['вниз-влево (y+)', 0, 1, 1],
    ['влево (x-, y+)', -1, 1, 2],
    ['вверх-влево (x-)', -1, 0, 3],
    ['вверх (x-, y-)', -1, -1, 4],
    ['вверх-вправо (y-)', 0, -1, 5],
    ['вправо (x+, y-)', 1, -1, 6],
    ['вниз-вправо (x+)', 1, 0, 7],
  ];
  it.each(cases)('%s', (_n, dx, dy, want) => {
    expect(facingOf(dx, dy)).toBe(want);
  });

  it('длина вектора не важна, повтор берётся из кэша', () => {
    expect(facingOf(5, 5)).toBe(0);
    expect(facingOf(5, 5)).toBe(0);
    expect(facingOf(0.3, 0)).toBe(7);
    expect(facingOf(-0.25, -0.25)).toBe(4);
  });

  it('нулевой вектор — null', () => {
    expect(facingOf(0, 0)).toBeNull();
  });

  it('сектор шириной 45°: небольшое отклонение не меняет направление', () => {
    expect(facingOf(10, 11)).toBe(0);
    expect(facingOf(11, 10)).toBe(0);
  });
});

describe('toClipDir / directionIndex', () => {
  it('8 направлений — без изменений, 1 — всегда 0', () => {
    for (let f = 0; f < 8; f++) {
      expect(toClipDir(f, 8)).toBe(f);
      expect(toClipDir(f, 1)).toBe(0);
    }
  });
  it('4 направления: 0=S, 1=W, 2=N, 3=E (по часовой)', () => {
    expect(toClipDir(0, 4)).toBe(0);
    expect(toClipDir(2, 4)).toBe(1);
    expect(toClipDir(4, 4)).toBe(2);
    expect(toClipDir(6, 4)).toBe(3);
    expect(toClipDir(7, 4)).toBe(0);
  });
  it('нулевой вектор оставляет запасное направление', () => {
    expect(directionIndex(0, 0, 8, 5)).toBe(5);
    expect(directionIndex(1, 0, 8)).toBe(7);
    expect(directionIndex(1, 0, 1)).toBe(0);
  });
});
