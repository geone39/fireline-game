import { describe, expect, it, vi } from 'vitest';
import { GRID_SIZE, HELLO_TILE } from './config';
import type { InputHandler, Renderer } from './renderer';

function mockRenderer(): Renderer {
  return {
    loadAtlas: vi.fn().mockResolvedValue(undefined),
    render: vi.fn(),
    on: vi.fn((handler: InputHandler) => {
      void handler;
      return () => {};
    }),
    setCamera: vi.fn(),
    play: vi.fn(),
  };
}

describe('Renderer contract', () => {
  it('can be mocked and driven by a consumer', async () => {
    const r = mockRenderer();
    await r.loadAtlas({});
    r.render({ gridSize: GRID_SIZE, highlight: { ...HELLO_TILE } });
    r.setCamera({ x: 0, y: 0, zoom: 1 });
    r.play({ name: 'boom', x: 1, y: 2 });
    const off = r.on(() => {});
    expect(typeof off).toBe('function');
    expect(r.render).toHaveBeenCalledWith({ gridSize: GRID_SIZE, highlight: HELLO_TILE });
    expect(r.play).toHaveBeenCalledTimes(1);
  });
});
