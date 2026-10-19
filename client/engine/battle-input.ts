import { zoomAt } from './camera';
import { WHEEL_ZOOM_STEP } from './config';
import { GestureController, type Gesture } from './gestures';
import { screenToCell } from './iso';
import { routeTap } from './battle-logic';
import type { BattleWorldState } from './battle-types';
import { inGrid } from './picking';
import type { Camera, InputEvent } from './renderer';

export interface BattleInputHost {
  getCamera(): Camera;
  /** Реализация обязана ограничить камеру краями карты. */
  setCamera(camera: Camera): void;
  getState(): BattleWorldState | null;
  emit(e: InputEvent): void;
}

/** Указатели канваса -> события боя (battleSelect/Target/Cell, hover) и движение камеры. Без DOM и Pixi. */
export class BattleInput {
  private readonly gestures: GestureController;
  private hoverKey = '';

  constructor(private readonly host: BattleInputHost) {
    // Перетаскивания в бою нет: один палец двигает камеру, тап — команда.
    this.gestures = new GestureController((g) => this.onGesture(g), () => false);
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

  private setHover(tile: { x: number; y: number } | null): void {
    const key = tile ? `${tile.x},${tile.y}` : '';
    if (key === this.hoverKey) return;
    this.hoverKey = key;
    this.host.emit({ type: 'hover', tile });
  }

  private onGesture(g: Gesture): void {
    const state = this.host.getState();
    switch (g.type) {
      case 'hover': {
        const c = this.cellAt(g.sx, g.sy);
        this.setHover(state && inGrid(c.x, c.y, state.gridSize) ? c : null);
        break;
      }
      case 'tap': {
        if (!state) break;
        const c = this.cellAt(g.sx, g.sy);
        const tap = routeTap(state, c.x, c.y);
        if (tap) this.host.emit(tap);
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
      default:
        break;
    }
  }
}
