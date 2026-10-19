import { describe, expect, it } from 'vitest';
import { mockBaseState } from './base-source';
import { Interaction } from './interaction';
import { tileToScreen } from './iso';
import type { Camera, InputEvent, WorldState } from './renderer';
import { footprints, grid } from './test-content';

function setup(selectedUid: string | null = null) {
  const events: InputEvent[] = [];
  let camera: Camera = { x: 400, y: 300, zoom: 1 };
  const world: WorldState = {
    gridSize: grid.size,
    items: mockBaseState().layout.items,
    footprints,
    selectedUid,
  };
  const ix = new Interaction({
    getCamera: () => camera,
    setCamera: (c) => {
      camera = c;
    },
    getWorld: () => world,
    emit: (e) => events.push(e),
  });
  const screenOf = (tx: number, ty: number) => {
    const p = tileToScreen(tx, ty);
    return { x: p.x * camera.zoom + camera.x, y: p.y * camera.zoom + camera.y };
  };
  return { ix, events, screenOf, cam: () => camera };
}

describe('Interaction', () => {
  it('tap on a building emits tileTap + select with its uid', () => {
    const { ix, events, screenOf } = setup();
    const p = screenOf(21, 21);
    ix.pointerDown(1, p.x, p.y);
    ix.pointerUp(1, p.x, p.y);
    expect(events).toEqual([
      { type: 'tileTap', x: 21, y: 21 },
      { type: 'select', uid: 'hq', x: 21, y: 21 },
    ]);
  });
  it('tap on empty ground deselects; tap outside the grid is ignored', () => {
    const { ix, events, screenOf } = setup('hq');
    const p = screenOf(5, 40);
    ix.pointerDown(1, p.x, p.y);
    ix.pointerUp(1, p.x, p.y);
    expect(events.at(-1)).toEqual({ type: 'select', uid: null, x: 5, y: 40 });
    events.length = 0;
    const out = screenOf(-3, -3);
    ix.pointerDown(2, out.x, out.y);
    ix.pointerUp(2, out.x, out.y);
    expect(events).toEqual([]);
  });
  it('hover emits once per cell and null when leaving', () => {
    const { ix, events, screenOf } = setup();
    const p = screenOf(7, 9);
    ix.pointerMove(1, p.x, p.y);
    ix.pointerMove(1, p.x + 1, p.y);
    expect(events).toEqual([{ type: 'hover', tile: { x: 7, y: 9 } }]);
    ix.pointerLeave();
    expect(events.at(-1)).toEqual({ type: 'hover', tile: null });
  });
  it('dragging the selected building emits dragStart/Move/End with cells', () => {
    const { ix, events, screenOf } = setup('hq');
    const a = screenOf(21, 21);
    const b = screenOf(30, 10);
    ix.pointerDown(1, a.x, a.y);
    ix.pointerMove(1, b.x, b.y);
    ix.pointerUp(1, b.x, b.y);
    expect(events).toEqual([
      { type: 'dragStart', uid: 'hq', x: 21, y: 21 },
      { type: 'dragMove', uid: 'hq', x: 30, y: 10 },
      { type: 'dragEnd', uid: 'hq', x: 30, y: 10 },
    ]);
  });
  it('dragging an unselected building pans the camera instead', () => {
    const { ix, events, screenOf, cam } = setup(null);
    const a = screenOf(21, 21);
    ix.pointerDown(1, a.x, a.y);
    ix.pointerMove(1, a.x + 40, a.y);
    ix.pointerUp(1, a.x + 40, a.y);
    expect(events.map((e) => e.type)).toEqual(['pan']);
    expect(cam().x).toBe(440);
  });
  it('wheel zooms the camera and reports the delta', () => {
    const { ix, events, cam } = setup();
    ix.wheel(-1, 400, 300);
    expect(cam().zoom).toBeGreaterThan(1);
    expect(events[0]).toMatchObject({ type: 'zoom' });
  });
});
