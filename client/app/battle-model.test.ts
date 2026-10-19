import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { END_REASON_KEYS, rejectKey, stars } from './battle-model';
import { APP_STRINGS } from './i18n';

describe('battle-model', () => {
  it('battle-snapshot.json совпадает с sim/testdata G24', () => {
    const g = JSON.parse(readFileSync(new URL('../../sim/testdata/replays/G24.json', import.meta.url), 'utf8')) as { snapshot: unknown };
    const copy = JSON.parse(readFileSync(new URL('./battle-snapshot.json', import.meta.url), 'utf8')) as unknown;
    expect(copy).toEqual(g.snapshot);
  });

  it('rejectKey: коды ядра 1..17 имеют строки ru/en, остальное — unknown', () => {
    for (let c = 1; c <= 17; c++) {
      expect(APP_STRINGS.ru[rejectKey(c)]).toBeTruthy();
      expect(APP_STRINGS.en[rejectKey(c)]).toBeTruthy();
    }
    expect(rejectKey(0)).toBe('battle.reject.unknown');
    expect(rejectKey(99)).toBe('battle.reject.unknown');
  });

  it('причины окончания и звёзды', () => {
    for (const k of END_REASON_KEYS.slice(1)) expect(APP_STRINGS.ru[`battle.end.${k}`]).toBeTruthy();
    expect(stars(2)).toBe('★★☆');
    expect(stars(0)).toBe('☆☆☆');
  });
});
