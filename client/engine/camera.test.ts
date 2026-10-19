import { describe, expect, it } from 'vitest';
import { centerOn, clampCamera, clampZoom, intersects, visibleRect, worldBounds, zoomAt } from './camera';
import { MAX_ZOOM, MIN_ZOOM } from './config';
import { tileToScreen } from './iso';
import { grid } from './test-content';

const view = { width: 800, height: 600 };
const bounds = worldBounds(grid.size);

describe('camera', () => {
  it('world bounds enclose the whole grid diamond', () => {
    expect(bounds.top).toBeLessThan(tileToScreen(0, 0).y);
    expect(bounds.bottom).toBeGreaterThan(tileToScreen(grid.size - 1, grid.size - 1).y);
    expect(bounds.left).toBeLessThan(tileToScreen(0, grid.size - 1).x);
    expect(bounds.right).toBeGreaterThan(tileToScreen(grid.size - 1, 0).x);
  });
  it('clamps zoom', () => {
    expect(clampZoom(100)).toBe(MAX_ZOOM);
    expect(clampZoom(0.001)).toBe(MIN_ZOOM);
  });
  it('does not let the map leave the viewport', () => {
    const far = clampCamera({ x: 99999, y: 99999, zoom: 1 }, view, bounds);
    expect(far.x).toBeCloseTo(-bounds.left);
    expect(far.y).toBeCloseTo(-bounds.top);
    const near = clampCamera({ x: -99999, y: -99999, zoom: 1 }, view, bounds);
    expect(near.x).toBeCloseTo(view.width - bounds.right);
    expect(near.y).toBeCloseTo(view.height - bounds.bottom);
  });
  it('keeps a valid camera untouched', () => {
    const c = centerOn(21.5, 21.5, 1, view);
    expect(clampCamera(c, view, bounds)).toEqual(c);
  });
  it('centres the map when it is smaller than the viewport', () => {
    const c = clampCamera({ x: 0, y: 0, zoom: MIN_ZOOM }, { width: 5000, height: 5000 }, bounds);
    const cx = c.x + ((bounds.left + bounds.right) / 2) * MIN_ZOOM;
    expect(cx).toBeCloseTo(2500);
  });
  it('zoomAt keeps the world point under the pivot fixed', () => {
    const cam = { x: -300, y: -100, zoom: 1 };
    const px = 400;
    const py = 300;
    const before = { x: (px - cam.x) / cam.zoom, y: (py - cam.y) / cam.zoom };
    const next = zoomAt(cam, 1.5, px, py);
    expect((px - next.x) / next.zoom).toBeCloseTo(before.x);
    expect((py - next.y) / next.zoom).toBeCloseTo(before.y);
    expect(zoomAt(cam, 1000, px, py).zoom).toBe(MAX_ZOOM);
  });
  it('visibleRect and intersects drive culling', () => {
    const cam = { x: 0, y: 0, zoom: 2 };
    const v = visibleRect(cam, view, 0);
    expect(v).toEqual({ left: 0, top: 0, right: 400, bottom: 300 });
    expect(intersects({ left: 100, top: 100, right: 150, bottom: 150 }, v)).toBe(true);
    expect(intersects({ left: 500, top: 100, right: 600, bottom: 150 }, v)).toBe(false);
    expect(visibleRect(cam, view, 10).left).toBe(-10);
  });
});
