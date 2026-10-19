import { WHEEL_ZOOM_STEP } from './config';
import { zoomAt } from './camera';
import { GestureController, type Gesture } from './gestures';
import { screenToCell } from './iso';
import { inGrid, pickItem } from './picking';
import { footprintOf } from './base-types';
import type { Camera, InputEvent, WorldState } from './renderer';

export interface InteractionHost {
  getCamera(): Camera;
  /** Реализация обязана ограничить камеру краями карты. */
  setCamera(camera: Camera): void;
  getWorld(): WorldState | null;
  emit(e: InputEvent): void;
}

/** Превращает указатели канваса в события Renderer (select/hover/drag) и в движение камеры. Без DOM и Pixi. */
export class Interaction {
  private readonly gestures: GestureController;
  private hoverKey = '';
  private dragUid: string | null = null;

  constructor(private readonly host: InteractionHost) {
    this.gestures = new GestureController(
      (g) => this.onGesture(g),
      (sx, sy) => this.selectedAt(sx, sy) !== undefined,
    );
  }

  pointerDown(id: number, sx: number, sy: number): void {
    this.gestures.down(id, sx, sy);
  }
  pointerMove(id: number, sx: number, sy: number): void {
    this.gestures.move(id, sx, sy);
  }
  pointerUp(id: number, sx: number, sy: number): void {
    this.gestures.up(id, sx, sy);
  }
  pointerCancel(id: number): void {
    this.gestures.cancel(id);
  }
  wheel(deltaY: number, sx: number, sy: number): void {
    this.gestures.wheel(deltaY, sx, sy, WHEEL_ZOOM_STEP);
  }
  pointerLeave(): void {
    this.setHover(null);
  }

  private cellAt(sx: number, sy: number): { x: number; y: number } {
    const cam = this.host.getCamera();
    return screenToCell((sx - cam.x) / cam.zoom, (sy - cam.y) / cam.zoom);
  }

  private selectedAt(sx: number, sy: number) {
    const w = this.host.getWorld();
    if (!w?.items || !w.selectedUid) return undefined;
    const c = this.cellAt(sx, sy);
    const it = pickItem(c.x, c.y, w.items, w.footprints ?? {});
    return it?.uid === w.selectedUid ? it : undefined;
  }

  private setHover(tile: { x: number; y: number } | null): void {
    const key = tile ? `${tile.x},${tile.y}` : '';
    if (key === this.hoverKey) return;
    this.hoverKey = key;
    this.host.emit({ type: 'hover', tile });
  }

  private onGesture(g: Gesture): void {
    const world = this.host.getWorld();
    switch (g.type) {
      case 'hover': {
        const c = this.cellAt(g.sx, g.sy);
        this.setHover(world && inGrid(c.x, c.y, world.gridSize) ? c : null);
        break;
      }
      case 'tap': {
        if (!world) break;
        const c = this.cellAt(g.sx, g.sy);
        if (!inGrid(c.x, c.y, world.gridSize)) break;
        const it = pickItem(c.x, c.y, world.items ?? [], world.footprints ?? {});
        this.host.emit({ type: 'tileTap', x: c.x, y: c.y });
        this.host.emit({ type: 'select', uid: it?.uid ?? null, x: c.x, y: c.y });
        break;
      }
      case 'pan': {
        const cam = this.host.getCamera();
        this.host.setCamera({ ...cam, x: cam.x + g.dx, y: cam.y + g.dy });
        this.host.emit({ type: 'pan', dx: g.dx, dy: g.dy });
        break;
      }
      case 'zoom': {
        const cam = this.host.getCamera();
        this.host.setCamera(zoomAt(cam, g.factor, g.sx, g.sy));
        this.host.emit({ type: 'zoom', delta: this.host.getCamera().zoom - cam.zoom });
        break;
      }
      case 'dragStart': {
        const uid = world?.selectedUid ?? null;
        if (uid === null) break;
        this.dragUid = uid;
        const c = this.cellAt(g.sx, g.sy);
        this.host.emit({ type: 'dragStart', uid, x: c.x, y: c.y });
        break;
      }
      case 'dragMove':
      case 'dragEnd': {
        if (this.dragUid === null) break;
        const c = this.cellAt(g.sx, g.sy);
        this.host.emit({ type: g.type, uid: this.dragUid, x: c.x, y: c.y });
        if (g.type === 'dragEnd') this.dragUid = null;
        break;
      }
    }
  }
}

/** Футпринт выбранного здания (для подсветки), либо undefined. */
export function selectedFootprint(world: WorldState): { x: number; y: number; w: number; h: number } | undefined {
  const it = world.items?.find((i) => i.uid === world.selectedUid);
  if (!it) return undefined;
  return { x: it.x, y: it.y, ...footprintOf(it, world.footprints ?? {}) };
}
