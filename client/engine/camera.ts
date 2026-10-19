import { MAX_ZOOM, MIN_ZOOM } from './config';
import { tileToScreen } from './iso';
import type { Camera } from './renderer';

export interface Viewport {
  width: number;
  height: number;
}

export interface WorldRect {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

/** Габариты изометрической сетки size×size в координатах мира (px). */
export function worldBounds(size: number): WorldRect {
  const l = tileToScreen(-0.5, size - 0.5);
  const r = tileToScreen(size - 0.5, -0.5);
  const t = tileToScreen(-0.5, -0.5);
  const b = tileToScreen(size - 0.5, size - 0.5);
  return { left: l.x, right: r.x, top: t.y, bottom: b.y };
}

export function clampZoom(zoom: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom));
}

function clampAxis(offset: number, viewLen: number, min: number, max: number, zoom: number): number {
  // Границы мира на экране: min*zoom+offset .. max*zoom+offset.
  const lo = viewLen - max * zoom; // правый/нижний край мира не уходит внутрь вьюпорта
  const hi = -min * zoom; // левый/верхний край мира не уходит внутрь вьюпорта
  if (lo > hi) return (lo + hi) / 2; // мир меньше вьюпорта -> по центру
  return Math.min(hi, Math.max(lo, offset));
}

/** Ограничивает камеру краями карты и допустимым зумом. */
export function clampCamera(cam: Camera, view: Viewport, bounds: WorldRect): Camera {
  const zoom = clampZoom(cam.zoom);
  return {
    zoom,
    x: clampAxis(cam.x, view.width, bounds.left, bounds.right, zoom),
    y: clampAxis(cam.y, view.height, bounds.top, bounds.bottom, zoom),
  };
}

/** Зум относительно экранной точки (px,py): точка мира под курсором остаётся на месте. */
export function zoomAt(cam: Camera, factor: number, px: number, py: number): Camera {
  const zoom = clampZoom(cam.zoom * factor);
  const k = zoom / cam.zoom;
  return { zoom, x: px - (px - cam.x) * k, y: py - (py - cam.y) * k };
}

/** Видимая область в координатах мира. */
export function visibleRect(cam: Camera, view: Viewport, margin = 0): WorldRect {
  return {
    left: (0 - cam.x) / cam.zoom - margin,
    top: (0 - cam.y) / cam.zoom - margin,
    right: (view.width - cam.x) / cam.zoom + margin,
    bottom: (view.height - cam.y) / cam.zoom + margin,
  };
}

export function intersects(a: WorldRect, b: WorldRect): boolean {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
}

/** Камера, центрирующая тайл (tx,ty) во вьюпорте. */
export function centerOn(tx: number, ty: number, zoom: number, view: Viewport): Camera {
  const c = tileToScreen(tx, ty);
  return { zoom, x: view.width / 2 - c.x * zoom, y: view.height / 2 - c.y * zoom };
}
