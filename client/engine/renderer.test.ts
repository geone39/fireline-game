import { describe, expect, it, vi } from 'vitest';
import { indexSprites, parseArtManifest } from './atlas';
import { mockBaseState } from './base-source';
import { sampleBattle, sampleEvents } from './battle-fixture';
import type { BattleRenderer, InputEvent, InputHandler, Renderer, WorldState } from './renderer';
import { artManifest, footprints, grid } from './test-content';

export function mockRenderer(): Renderer & { emit(e: InputEvent): void } {
  const handlers = new Set<InputHandler>();
  return {
    loadAtlas: vi.fn().mockResolvedValue(undefined),
    render: vi.fn(),
    on: vi.fn((handler: InputHandler) => {
      handlers.add(handler);
      return () => handlers.delete(handler);
    }),
    setCamera: vi.fn(),
    play: vi.fn(),
    emit: (e) => handlers.forEach((h) => h(e)),
  };
}

export function mockBattleRenderer(): BattleRenderer & { emit(e: InputEvent): void } {
  return {
    ...mockRenderer(),
    setBattle: vi.fn(),
    playEvents: vi.fn(),
    skipEvents: vi.fn(),
    setSkipAnimations: vi.fn(),
  };
}

describe('BattleRenderer contract (0.3)', () => {
  it('a 0.2 consumer of Renderer accepts a BattleRenderer unchanged', () => {
    const r: Renderer = mockBattleRenderer();
    r.render({ gridSize: grid.size });
    expect(r.render).toHaveBeenCalledTimes(1);
  });
  it('a consumer drives a turn: setBattle, input -> command, playEvents, done -> setBattle', () => {
    const r = mockBattleRenderer();
    let state = sampleBattle(grid.size, footprints);
    const commands: string[] = [];
    r.setBattle(state);
    r.on((e) => {
      if (e.type === 'battleTarget') {
        commands.push(`attack:${e.id}`);
        r.playEvents(sampleEvents());
      } else if (e.type === 'battleEventsDone') {
        state = { ...state, selectedId: null };
        r.setBattle(state);
      }
    });
    r.emit({ type: 'battleTarget', id: 3, x: 16, y: 24 });
    r.emit({ type: 'battleEventsDone' });
    expect(commands).toEqual(['attack:3']);
    expect(r.playEvents).toHaveBeenCalledWith(sampleEvents());
    expect(r.setBattle).toHaveBeenLastCalledWith(expect.objectContaining({ selectedId: null }));
  });
});

describe('Renderer contract', () => {
  it('can be mocked and driven by a consumer (select -> render with selectedUid)', async () => {
    const r = mockRenderer();
    let world: WorldState = {
      gridSize: grid.size,
      items: mockBaseState().layout.items,
      footprints,
      water: grid.maps.coastal,
      selectedUid: null,
    };
    await r.loadAtlas(artManifest);
    r.render(world);
    r.on((e) => {
      if (e.type === 'select') {
        world = { ...world, selectedUid: e.uid };
        r.render(world);
      }
    });
    r.emit({ type: 'select', uid: 'hq', x: 20, y: 20 });
    expect(r.render).toHaveBeenCalledTimes(2);
    expect(r.render).toHaveBeenLastCalledWith(expect.objectContaining({ selectedUid: 'hq' }));
    r.setCamera({ x: 0, y: 0, zoom: 1 });
    r.play({ name: 'boom', x: 1, y: 2 });
    expect(r.play).toHaveBeenCalledTimes(1);
  });
  it('v0.1 consumers keep working: minimal WorldState is valid', () => {
    const r = mockRenderer();
    r.render({ gridSize: grid.size, highlight: { x: 1, y: 1 } });
    expect(r.render).toHaveBeenCalledWith({ gridSize: 44, highlight: { x: 1, y: 1 } });
  });
  it('unsubscribe stops delivery', () => {
    const r = mockRenderer();
    const h = vi.fn();
    const off = r.on(h);
    off();
    r.emit({ type: 'pan', dx: 1, dy: 1 });
    expect(h).not.toHaveBeenCalled();
  });
});

describe('test base vs content and art', () => {
  const sprites = indexSprites(parseArtManifest(artManifest));
  it('every type in the mock base has a footprint and a placeholder sprite', () => {
    for (const it of mockBaseState().layout.items) {
      expect(footprints[it.type], it.type).toBeDefined();
      expect(sprites.has(it.type), it.type).toBe(true);
    }
  });
  it('mock base fits inside the grid and has no overlaps', () => {
    const taken = new Set<string>();
    for (const it of mockBaseState().layout.items) {
      const [w, h] = footprints[it.type] ?? [1, 1];
      for (let dx = 0; dx < w; dx++) {
        for (let dy = 0; dy < h; dy++) {
          const k = `${it.x + dx},${it.y + dy}`;
          expect(taken.has(k), `${it.uid} overlaps at ${k}`).toBe(false);
          taken.add(k);
          expect(it.x + dx).toBeLessThan(grid.size);
          expect(it.y + dy).toBeLessThan(grid.size);
        }
      }
    }
  });
});
