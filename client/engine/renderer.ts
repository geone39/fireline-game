// Контракт modules/client-engine/CONTRACT.md v0.3 (аддитивное расширение v0.2 и v0.1):
// interface Renderer { loadAtlas(m); render(world); on(input); setCamera(); play(fx) }
import type { BaseItem, Footprints, Rect } from './base-types';
import type { BattleEvent, BattleWorldState } from './battle-types';

/** Манифест атласов (art/manifest.json). Типизированная форма — ArtManifest в atlas.ts. */
export interface AtlasManifest {
  [key: string]: unknown;
}

/**
 * Снимок мира. v0.1: gridSize, highlight. v0.2 (все поля необязательны): items, water, footprints, selectedUid.
 * Рендер ничего не решает про игру: items/selectedUid приходят снаружи (store/UI), footprints — из контента.
 */
export interface WorldState {
  gridSize: number;
  highlight?: { x: number; y: number };
  /** layout.items из BaseState. */
  items?: readonly BaseItem[];
  /** Прямоугольники воды выбранной карты (content/base_grid.yaml). */
  water?: readonly Rect[];
  /** Размеры футпринтов по типу (content/buildings.yaml, defences.yaml). */
  footprints?: Footprints;
  /** uid выбранного здания: подсвечивается; только его можно тащить (drag). */
  selectedUid?: string | null;
}

export type InputEvent =
  | { type: 'tileTap'; x: number; y: number }
  | { type: 'pan'; dx: number; dy: number }
  | { type: 'zoom'; delta: number }
  // v0.2
  /** Тап по клетке сетки: uid здания под ней или null (снять выделение). */
  | { type: 'select'; uid: string | null; x: number; y: number }
  /** Клетка под курсором (null — курсор вне сетки/канваса). */
  | { type: 'hover'; tile: { x: number; y: number } | null }
  /** Начало перетаскивания выбранного здания: клетка под указателем. */
  | { type: 'dragStart'; uid: string; x: number; y: number }
  | { type: 'dragMove'; uid: string; x: number; y: number }
  /** Конец перетаскивания; клетка может быть вне сетки — валидирует потребитель. */
  | { type: 'dragEnd'; uid: string; x: number; y: number }
  // v0.3 (бой)
  /** Тап по своему юниту (выбор). */
  | { type: 'battleSelect'; id: number }
  /** Тап по сущности обороны (или по своей, если она в highlight.targets): цель атаки/способности. */
  | { type: 'battleTarget'; id: number; x: number; y: number }
  /** Тап по клетке без выбираемой сущности (куда идти / куда кастовать). */
  | { type: 'battleCell'; x: number; y: number }
  /** Очередь событий ядра полностью проиграна (или пропущена). */
  | { type: 'battleEventsDone' };

export type InputHandler = (e: InputEvent) => void;

export interface Camera {
  x: number;
  y: number;
  zoom: number;
}

export interface Effect {
  name: string;
  x: number;
  y: number;
}

export interface Renderer {
  loadAtlas(manifest: AtlasManifest): Promise<void>;
  render(world: WorldState): void;
  /** Подписка на ввод; возвращает функцию отписки. */
  on(handler: InputHandler): () => void;
  /** Камера ограничивается краями карты и допустимым зумом. */
  setCamera(camera: Camera): void;
  play(fx: Effect): void;
}

/** v0.3: рендер поля боя. Реализуется дополнительно к Renderer (PixiRenderer реализует оба). */
export interface BattleRenderer extends Renderer {
  /**
   * Показывает поле боя (переключает с базы на бой). Это авторитетное состояние «после»: если идёт проигрывание
   * событий, оно применится по его окончании. Камера сохраняется от базы.
   */
  setBattle(state: BattleWorldState): void;
  /** Ставит события ядра sim (Battle.Apply) в очередь воспроизведения; по окончании приходит `battleEventsDone`. */
  playEvents(events: readonly BattleEvent[]): void;
  /** Мгновенно доигрывает очередь и переходит к состоянию последнего setBattle. */
  skipEvents(): void;
  /** Постоянный режим «пропустить»: события применяются без анимации. */
  setSkipAnimations(on: boolean): void;
}
