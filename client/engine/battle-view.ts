import { Container, Graphics, Sprite } from 'pixi.js';
import { ArtSpriteView } from './art-sprite';
import {
  BATTLE_AP_EMPTY,
  BATTLE_AP_FULL,
  BATTLE_ATTACKER_COLOR,
  BATTLE_DEFENDER_COLOR,
  BATTLE_HP_ATTACKER,
  BATTLE_HP_BAR_HEIGHT,
  BATTLE_HP_BAR_WIDTH,
  BATTLE_HP_BG,
  BATTLE_HP_DEFENDER,
  BATTLE_MOVE_ALPHA,
  BATTLE_MOVE_COLOR,
  BATTLE_RANGE_ALPHA,
  BATTLE_RANGE_COLOR,
  BATTLE_TARGET_COLOR,
  BATTLE_TRAP_COLOR,
  HOVER_ALPHA,
  BATTLE_DIE_FADE_MS,
  HOVER_COLOR,
  SELECT_COLOR,
  SHADOW_ALPHA,
  SHADOW_HEIGHT,
  SHADOW_WIDTH,
  TILE_HEIGHT,
  TILE_WIDTH,
} from './config';
import { battleDepth, hpRatio, rangeCells, showHpBar, sortBattle } from './battle-logic';
import type { BattlePlayback, Fx } from './battle-playback';
import { facingOf } from './direction';
import { footprintSize, type BattleEntity, type BattleWorldState } from './battle-types';
import { diamond, footprintCenter, footprintDiamond, tileToScreen } from './iso';

interface View {
  node: Sprite | Graphics;
  /** Высота над точкой «земли» для полосок hp (px мира). */
  top: number;
  /** Спрайт из манифеста с анимацией; у заглушек-фигур отсутствует. */
  art?: ArtSpriteView;
  /** Последнее обработанное действие (выстрел): новый выстрел перезапускает клип attack. */
  action?: object;
}

export type SpriteFactory = (type: string) => Sprite | undefined;

/** Pixi-слой боя: сущности, подсветка клеток, полоски hp/AP, эффекты. Логика — в battle-logic/battle-playback. */
export class BattleView {
  readonly container = new Container();
  private readonly highlight = new Graphics();
  private readonly shadows = new Graphics();
  private readonly entities = new Container();
  private readonly bars = new Graphics();
  private readonly fxLayer = new Graphics();
  private readonly views = new Map<number, View>();
  /** Погибшие юниты, проигрывающие die (по id сущности). */
  private readonly ghosts = new Map<number, View>();

  constructor(private readonly spriteFor: SpriteFactory) {
    this.entities.sortableChildren = true;
    this.container.addChild(this.highlight, this.shadows, this.entities, this.bars, this.fxLayer);
    this.container.visible = false;
  }

  show(on: boolean): void {
    this.container.visible = on;
  }

  /** Сбросить спрайты (после смены атласа они пересоздаются). */
  resetSprites(): void {
    for (const v of this.views.values()) v.node.destroy();
    this.views.clear();
    for (const v of this.ghosts.values()) v.node.destroy();
    this.ghosts.clear();
  }

  /** Продвигает анимацию спрайтов (idle/walk/attack/die) каждый кадр; перерисовка сцены не нужна. */
  tickAnim(dtMs: number): void {
    for (const v of this.views.values()) v.art?.advance(dtMs);
    for (const v of this.ghosts.values()) v.art?.advance(dtMs);
  }

  draw(state: BattleWorldState, playback: BattlePlayback, hover: { x: number; y: number } | null): void {
    this.syncEntities(state, playback);
    this.drawHighlight(state, hover);
    this.drawShadows(state, playback);
    this.drawBars(state, playback);
    this.drawFx(playback.fx);
  }

  private syncEntities(state: BattleWorldState, playback: BattlePlayback): void {
    const alive = new Set<number>();
    for (const e of sortBattle(state.entities)) {
      alive.add(e.id);
      let v = this.views.get(e.id);
      if (!v) {
        v = this.makeView(e);
        this.views.set(e.id, v);
        this.entities.addChild(v.node);
      }
      const pos = playback.positionOf(e);
      const { w, h } = footprintSize(e);
      const c = footprintCenter(pos.x, pos.y, w, h);
      const p = tileToScreen(c.x, c.y);
      v.node.position.set(p.x, p.y);
      v.node.zIndex = battleDepth(e, pos);
      if (v.node instanceof Graphics) this.paintFallback(v.node, e);
      else this.pose(v, e, playback);
    }
    for (const [id, v] of this.views) {
      if (alive.has(id)) continue;
      this.views.delete(id);
      if (v.art?.hasDie && playback.dying.has(id) && !this.ghosts.has(id)) {
        v.art.play('die', v.art.facing, true);
        this.ghosts.set(id, v);
      } else {
        v.node.destroy();
      }
    }
    this.syncGhosts(playback);
  }

  /** Состояние анимации юнита: attack (по событию) > walk (идёт перемещение) > idle; направление — по вектору. */
  private pose(v: View, e: BattleEntity, playback: BattlePlayback): void {
    const art = v.art;
    if (!art?.animated) return;
    const act = playback.actions.get(e.id);
    if (act) {
      art.play('attack', facingOf(act.dx, act.dy) ?? art.facing, v.action !== act);
      v.action = act;
      return;
    }
    v.action = undefined;
    const mv = playback.moves.get(e.id);
    if (mv) art.play('walk', facingOf(mv.toX - mv.fromX, mv.toY - mv.fromY) ?? art.facing);
    else art.play('idle', art.facing);
  }

  private syncGhosts(playback: BattlePlayback): void {
    for (const [id, v] of this.ghosts) {
      const d = playback.dying.get(id);
      if (!d) {
        v.node.destroy();
        this.ghosts.delete(id);
        continue;
      }
      const { w, h } = footprintSize(d.entity);
      const c = footprintCenter(d.x, d.y, w, h);
      const p = tileToScreen(c.x, c.y);
      v.node.position.set(p.x, p.y);
      v.node.zIndex = battleDepth(d.entity, { x: d.x, y: d.y }) - 0.5;
      v.node.alpha = Math.min(1, (d.duration - d.age) / BATTLE_DIE_FADE_MS);
    }
  }

  /** Мягкое пятно под юнитами с арт-спрайтом (у заглушек тень рисуется самой фигурой, у зданий запечена в кадре). */
  private drawShadows(state: BattleWorldState, playback: BattlePlayback): void {
    const g = this.shadows.clear();
    const spot = (x: number, y: number, e: BattleEntity, alpha: number) => {
      const { w, h } = footprintSize(e);
      const c = footprintCenter(x, y, w, h);
      const p = tileToScreen(c.x, c.y);
      g.ellipse(p.x, p.y, SHADOW_WIDTH / 2, SHADOW_HEIGHT / 2).fill({ color: 0x000000, alpha });
    };
    for (const e of state.entities) {
      if (isStatic(e) || !this.views.get(e.id)?.art) continue;
      const pos = playback.positionOf(e);
      spot(pos.x, pos.y, e, SHADOW_ALPHA);
    }
    for (const [id, v] of this.ghosts) {
      const d = playback.dying.get(id);
      if (d && v.art) spot(d.x, d.y, d.entity, SHADOW_ALPHA * v.node.alpha);
    }
  }

  private makeView(e: BattleEntity): View {
    const sprite = this.spriteFor(e.type);
    if (sprite) {
      const art = sprite instanceof ArtSpriteView ? sprite : undefined;
      return { node: sprite, art, top: art ? art.topOffset : sprite.height * sprite.anchor.y };
    }
    const isBuilding = isStatic(e);
    return { node: new Graphics(), top: isBuilding ? 28 : 26 };
  }

  private paintFallback(g: Graphics, e: BattleEntity): void {
    g.clear();
    const color = e.kind === 'trap' ? BATTLE_TRAP_COLOR : e.side === 'attacker' ? BATTLE_ATTACKER_COLOR : BATTLE_DEFENDER_COLOR;
    if (isStatic(e)) {
      const { w, h } = footprintSize(e);
      const rel = footprintDiamond(0, 0, w, h);
      const c = footprintCenter(0, 0, w, h);
      const o = tileToScreen(c.x, c.y);
      g.poly(rel.map((v, i) => v - (i % 2 === 0 ? o.x : o.y))).fill({ color, alpha: e.kind === 'trap' ? 0.8 : 1 });
    } else {
      g.ellipse(0, 0, TILE_WIDTH / 4, TILE_HEIGHT / 3).fill(0x000000);
      g.circle(0, -10, 9).fill(color).stroke({ width: 2, color: 0x111111 });
    }
  }

  private drawHighlight(state: BattleWorldState, hover: { x: number; y: number } | null): void {
    const g = this.highlight.clear();
    const hl = state.highlight;
    if (hl?.range) {
      for (const c of rangeCells(hl.range, state.gridSize)) {
        const p = tileToScreen(c.x, c.y);
        g.poly(diamond(p.x, p.y)).fill({ color: BATTLE_RANGE_COLOR, alpha: BATTLE_RANGE_ALPHA });
      }
    }
    for (const c of hl?.moves ?? []) {
      const p = tileToScreen(c.x, c.y);
      g.poly(diamond(p.x, p.y)).fill({ color: BATTLE_MOVE_COLOR, alpha: BATTLE_MOVE_ALPHA });
    }
    for (const id of hl?.targets ?? []) {
      const e = state.entities.find((x) => x.id === id);
      if (!e) continue;
      const { w, h } = footprintSize(e);
      g.poly(footprintDiamond(e.x, e.y, w, h)).stroke({ width: 3, color: BATTLE_TARGET_COLOR });
    }
    const sel = state.entities.find((x) => x.id === state.selectedId);
    if (sel) {
      const { w, h } = footprintSize(sel);
      g.poly(footprintDiamond(sel.x, sel.y, w, h)).stroke({ width: 3, color: SELECT_COLOR });
    }
    if (hover) {
      const p = tileToScreen(hover.x, hover.y);
      g.poly(diamond(p.x, p.y)).fill({ color: HOVER_COLOR, alpha: HOVER_ALPHA });
    }
  }

  private drawBars(state: BattleWorldState, playback: BattlePlayback): void {
    const g = this.bars.clear();
    for (const e of state.entities) {
      const v = this.views.get(e.id);
      if (!v) continue;
      const hasBar = showHpBar(e);
      const pips = e.side === 'attacker' && e.maxAp !== undefined && e.maxAp > 0;
      if (!hasBar && !pips) continue;
      const pos = playback.positionOf(e);
      const { w, h } = footprintSize(e);
      const c = footprintCenter(pos.x, pos.y, w, h);
      const p = tileToScreen(c.x, c.y);
      const x = p.x - BATTLE_HP_BAR_WIDTH / 2;
      let y = p.y - v.top - BATTLE_HP_BAR_HEIGHT - 2;
      if (hasBar) {
        g.rect(x, y, BATTLE_HP_BAR_WIDTH, BATTLE_HP_BAR_HEIGHT).fill(BATTLE_HP_BG);
        g.rect(x, y, BATTLE_HP_BAR_WIDTH * hpRatio(e), BATTLE_HP_BAR_HEIGHT).fill(
          e.side === 'attacker' ? BATTLE_HP_ATTACKER : BATTLE_HP_DEFENDER,
        );
        y -= 6;
      }
      if (pips) {
        const max = e.maxAp as number;
        const step = Math.min(8, BATTLE_HP_BAR_WIDTH / max);
        const x0 = p.x - ((max - 1) * step) / 2;
        for (let i = 0; i < max; i++) {
          g.circle(x0 + i * step, y + 2, 2.5).fill(i < (e.ap ?? 0) ? BATTLE_AP_FULL : BATTLE_AP_EMPTY);
        }
      }
    }
  }

  private drawFx(list: readonly Fx[]): void {
    const g = this.fxLayer.clear();
    for (const f of list) {
      const t = Math.min(1, f.age / f.duration);
      const p = tileToScreen(f.x, f.y);
      const lift = 12;
      switch (f.kind) {
        case 'shot': {
          const a = tileToScreen(f.fromX ?? f.x, f.fromY ?? f.y);
          const bx = a.x + (p.x - a.x) * t;
          const by = a.y - lift + (p.y - lift - (a.y - lift)) * t;
          g.moveTo(a.x, a.y - lift).lineTo(bx, by).stroke({ width: 2, color: 0xffe9a0, alpha: 0.8 });
          g.circle(bx, by, 3).fill(0xffffff);
          break;
        }
        case 'hit':
          g.circle(p.x, p.y - lift, 6 + 10 * t).stroke({ width: 2, color: 0xffffff, alpha: 1 - t });
          break;
        case 'explosion': {
          const r = (f.big ? 38 : 20) * (0.3 + 0.7 * t);
          g.circle(p.x, p.y - lift, r).fill({ color: 0xff9a2e, alpha: 0.85 * (1 - t) });
          g.circle(p.x, p.y - lift, r * 0.55).fill({ color: 0xfff1a8, alpha: 0.9 * (1 - t) });
          break;
        }
        case 'smoke': {
          const n = f.big ? 4 : 2;
          for (let i = 0; i < n; i++) {
            const dx = (i - (n - 1) / 2) * 8;
            g.circle(p.x + dx, p.y - lift - 30 * t - i * 4, 8 + 10 * t).fill({ color: 0x777777, alpha: 0.55 * (1 - t) });
          }
          break;
        }
        case 'spawn':
          g.circle(p.x, p.y, 8 + 14 * t).stroke({ width: 2, color: 0x8dffb0, alpha: 1 - t });
          break;
        case 'spell':
          g.circle(p.x, p.y, 20 + 50 * t).fill({ color: 0xff7a3a, alpha: 0.35 * (1 - t) });
          g.circle(p.x, p.y, 20 + 50 * t).stroke({ width: 3, color: 0xffd23f, alpha: 1 - t });
          break;
        case 'tick':
          g.circle(p.x, p.y - lift - 10 * t, 6).fill({ color: 0xff6a1f, alpha: 1 - t });
          break;
        case 'heal':
          g.rect(p.x - 2, p.y - lift - 14 * t - 6, 4, 12).fill({ color: 0x6dff8a, alpha: 1 - t });
          g.rect(p.x - 6, p.y - lift - 14 * t - 2, 12, 4).fill({ color: 0x6dff8a, alpha: 1 - t });
          break;
        case 'trap':
          g.circle(p.x, p.y, 10 + 24 * t).fill({ color: BATTLE_TRAP_COLOR, alpha: 0.7 * (1 - t) });
          break;
      }
    }
  }
}

function isStatic(e: BattleEntity): boolean {
  return e.kind === 'building' || e.kind === 'defence' || e.kind === 'wall' || e.kind === 'decor' || e.kind === 'trap';
}
