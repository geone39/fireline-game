import { describe, expect, it } from 'vitest';
import { BattleInput } from './battle-input';
import { sampleBattle } from './battle-fixture';
import { tileToScreen } from './iso';
import type { Camera, InputEvent } from './renderer';
import { footprints, grid } from './test-content';

function setup() {
  let cam: Camera = { x: 300, y: 50, zoom: 1 };
  const out: InputEvent[] = [];
  const state = sampleBattle(grid.size, footprints);
  const input = new BattleInput({
    getCamera: () => cam,
    setCamera: (c) => {
      cam = c;
    },
    getState: () => state,
    emit: (e) => out.push(e),
  });
  const px = (tx: number, ty: number) => {
    const p = tileToScreen(tx, ty);
    return { x: p.x * cam.zoom + cam.x, y: p.y * cam.zoom + cam.y };
  };
  const tap = (tx: number, ty: number) => {
    const p = px(tx, ty);
    input.pointerDown(1, p.x, p.y);
    input.pointerUp(1, p.x, p.y);
  };
  return { out, tap, input, px, cam: () => cam };
}

describe('BattleInput', () => {
  it('tap on own unit -> battleSelect', () => {
    const t = setup();
    t.tap(11, 24);
    expect(t.out).toEqual([{ type: 'battleSelect', id: 11 }]);
  });
  it('tap on defender -> battleTarget; on ground -> battleCell', () => {
    const t = setup();
    t.tap(26, 21);
    t.tap(12, 12);
    expect(t.out).toEqual([
      { type: 'battleTarget', id: 2, x: 26, y: 21 },
      { type: 'battleCell', x: 12, y: 12 },
    ]);
  });
  it('drag pans the camera and produces no commands', () => {
    const t = setup();
    const p = t.px(12, 12);
    t.input.pointerDown(1, p.x, p.y);
    t.input.pointerMove(1, p.x + 40, p.y + 20);
    t.input.pointerUp(1, p.x + 40, p.y + 20);
    expect(t.out.some((e) => e.type.startsWith('battle'))).toBe(false);
    expect(t.out.some((e) => e.type === 'pan')).toBe(true);
  });
});
