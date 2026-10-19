// Компоненты редактора базы. Чистые функции от пропсов (без хуков и своего состояния): всё состояние — в
// client/core/base, подписку на него делает useBaseController (hook.ts). Поэтому их можно тестировать без DOM.
import type { VNode } from 'preact';
import type { BaseActions } from '../../core/base/controller';
import type { BaseViewModel, CardVM, OverlayVM, ShopEntry } from '../../core/base/view-model';
import { formatCost, formatDuration, type T } from './i18n';

export interface Point {
  x: number;
  y: number;
}
/** Тайловые координаты (дробные) → экранные. Даёт шелл/рендер (iso + камера), см. OQ-B10. */
export type Project = (tx: number, ty: number) => Point;

const COLOR = { ok: '#2fbf71', bad: '#e5484d', aura: '#4c8dff' };

export function ShopPanel(p: { entries: readonly ShopEntry[]; t: T; onPick: (type: string) => void }): VNode {
  return (
    <section data-testid="shop" aria-label={p.t('ui.shop')}>
      <h3>{p.t('ui.shop')}</h3>
      <ul>
        {p.entries.map((e) => (
          <li key={e.type} data-testid={`shop-${e.type}`}>
            <button type="button" disabled={e.blocked !== null} onClick={() => p.onPick(e.type)}>
              {p.t(e.nameKey)}
            </button>{' '}
            <span data-testid={`shop-${e.type}-info`}>
              {formatCost(e.cost, p.t)}
              {e.limit !== null ? ` · ${p.t('ui.count')} ${e.count}/${e.limit}` : ''}
            </span>
            {e.blocked && <small data-testid={`shop-${e.type}-block`}> {p.t(`block.${e.blocked}`, { n: e.unlockHq })}</small>}
          </li>
        ))}
      </ul>
    </section>
  );
}

export interface CardHandlers {
  onUpgrade: (uid: string) => void;
  onSpeedup: (uid: string) => void;
  onAskRemove: (uid: string | null) => void;
  onRemove: (uid: string) => void;
  onClear: (uid: string) => void;
  onRotate: () => void;
  onClose: () => void;
}

export function BuildingCard(p: { card: CardVM; t: T } & CardHandlers): VNode {
  const c = p.card;
  return (
    <section data-testid="card" aria-label={p.t(c.nameKey)}>
      <h3>{p.t(c.nameKey)}</h3>
      <p data-testid="card-level">
        {p.t('ui.level')} {c.level}/{c.maxLevel}
      </p>
      {c.timer && (
        <p data-testid="card-timer">
          {p.t(c.timer.kind === 'build' ? 'ui.building' : 'ui.upgrading')}: {formatDuration(c.timer.remainingS, p.t)} {p.t('ui.left')}
        </p>
      )}
      {c.kind !== 'obstacle' && c.upgrade.blocked !== 'no_price' && (
        <p>
          <button type="button" data-testid="card-upgrade" disabled={c.upgrade.blocked !== null} onClick={() => p.onUpgrade(c.uid)}>
            {p.t('ui.upgrade')}
          </button>{' '}
          <span data-testid="card-upgrade-cost">{c.upgrade.blocked === 'max_level' ? '' : formatCost(c.upgrade.cost, p.t)}</span>
          {c.upgrade.blocked && <small data-testid="card-upgrade-block"> {p.t(`block.${c.upgrade.blocked}`)}</small>}
        </p>
      )}
      {c.timer && c.speedupGems !== null && (
        <button type="button" data-testid="card-speedup" onClick={() => p.onSpeedup(c.uid)}>
          {p.t('ui.speedup')} · {c.speedupGems} {p.t('ui.gems')}
        </button>
      )}
      {c.clear && (
        <p>
          <button type="button" data-testid="card-clear" onClick={() => p.onClear(c.uid)}>
            {p.t('ui.clear')} · {formatCost(c.clear.cost, p.t)}
          </button>{' '}
          <small data-testid="card-reward">
            {p.t('ui.reward')}: {formatCost(c.clear.reward, p.t)}
          </small>
        </p>
      )}
      {c.canRotate && (
        <button type="button" data-testid="card-rotate" onClick={p.onRotate}>
          {p.t('ui.rotate')}
        </button>
      )}
      {c.canRemove &&
        (c.confirmRemove ? (
          <span>
            <button type="button" data-testid="card-remove-yes" onClick={() => p.onRemove(c.uid)}>
              {p.t('ui.remove_confirm')}
            </button>
            <button type="button" data-testid="card-remove-no" onClick={() => p.onAskRemove(null)}>
              {p.t('ui.close')}
            </button>
          </span>
        ) : (
          <button type="button" data-testid="card-remove" onClick={() => p.onAskRemove(c.uid)}>
            {p.t('ui.remove')}
          </button>
        ))}
      <button type="button" data-testid="card-close" onClick={p.onClose}>
        {p.t('ui.close')}
      </button>
    </section>
  );
}

export function Toolbar(p: { tb: BaseViewModel['toolbar']; t: T; actions: Pick<BaseActions, 'toggleEdit' | 'undo' | 'redo' | 'save' | 'cancel' | 'collect'> }): VNode {
  const { tb, actions } = p;
  return (
    <nav data-testid="toolbar">
      <span data-testid="builders">
        {p.t('ui.builders')}: {tb.builders.total - tb.builders.busy}/{tb.builders.total}
      </span>{' '}
      <button type="button" data-testid="collect" disabled={!tb.hasCollectable} onClick={actions.collect}>
        {p.t('ui.collect')}
      </button>{' '}
      <button type="button" data-testid="toggle-edit" aria-pressed={tb.editing} disabled={tb.editing && tb.dirty} onClick={actions.toggleEdit}>
        {p.t(tb.editing ? 'ui.done' : 'ui.edit')}
      </button>
      {tb.editing && (
        <span data-testid="edit-tools">
          <button type="button" data-testid="undo" disabled={!tb.canUndo} onClick={actions.undo}>
            {p.t('ui.undo')}
          </button>
          <button type="button" data-testid="redo" disabled={!tb.canRedo} onClick={actions.redo}>
            {p.t('ui.redo')}
          </button>
          <button type="button" data-testid="save" disabled={!tb.dirty || tb.saving} onClick={actions.save}>
            {p.t(tb.saving ? 'ui.saving' : 'ui.save')}
          </button>
          <button type="button" data-testid="discard" disabled={!tb.dirty || tb.saving} onClick={actions.cancel}>
            {p.t('ui.cancel')}
          </button>
        </span>
      )}
    </nav>
  );
}

export function PlacementBar(p: {
  placing: NonNullable<BaseViewModel['placing']>;
  t: T;
  onConfirm: () => void;
  onCancel: () => void;
  onRotate: () => void;
}): VNode {
  const { placing } = p;
  return (
    <div data-testid="placement-bar" role="status">
      <strong>{p.t(placing.nameKey)}</strong>{' '}
      <span data-testid="placement-state" data-ok={placing.ok}>
        {placing.ok ? '✓' : placing.reason ? p.t(`reason.${placing.reason}`) : ''}
      </span>{' '}
      {placing.canRotate && (
        <button type="button" data-testid="placement-rotate" onClick={p.onRotate}>
          {p.t('ui.rotate')}
        </button>
      )}
      <button type="button" data-testid="placement-confirm" disabled={!placing.ok} onClick={p.onConfirm}>
        {p.t('ui.place')}
      </button>
      <button type="button" data-testid="placement-cancel" onClick={p.onCancel}>
        {p.t('ui.close')}
      </button>
    </div>
  );
}

const CIRCLE_STEPS = 48;

function ring(project: Project, cx: number, cy: number, r: number): string {
  const pts: string[] = [];
  for (let i = 0; i < CIRCLE_STEPS; i++) {
    const a = (i / CIRCLE_STEPS) * Math.PI * 2;
    const q = project(cx + Math.cos(a) * r, cy + Math.sin(a) * r);
    pts.push(`${q.x.toFixed(1)},${q.y.toFixed(1)}`);
  }
  return pts.join(' ');
}

/** Подсветка клеток (зелёный — можно, красный — нельзя) и радиусов огня поверх канваса. */
export function PlacementOverlay(p: { overlay: OverlayVM; project: Project }): VNode {
  const { overlay, project } = p;
  return (
    <svg data-testid="overlay" width="100%" height="100%" style={{ position: 'absolute', inset: 0, pointerEvents: 'none' }} aria-hidden="true">
      {overlay.tiles.map((tile) => {
        const pts = [project(tile.x, tile.y), project(tile.x + 1, tile.y), project(tile.x + 1, tile.y + 1), project(tile.x, tile.y + 1)]
          .map((q) => `${q.x.toFixed(1)},${q.y.toFixed(1)}`)
          .join(' ');
        return (
          <polygon
            key={`${tile.x},${tile.y}`}
            data-testid="overlay-tile"
            data-ok={tile.ok}
            points={pts}
            fill={tile.ok ? COLOR.ok : COLOR.bad}
            fill-opacity="0.45"
          />
        );
      })}
      {overlay.radii.map((r, i) => (
        <polygon
          key={`${r.kind}-${i}`}
          data-testid={`overlay-radius-${r.kind}`}
          points={ring(project, r.cx, r.cy, r.r)}
          fill={r.kind === 'dead' ? COLOR.bad : r.kind === 'range' ? COLOR.ok : COLOR.aura}
          fill-opacity={r.kind === 'dead' ? '0.3' : '0.12'}
          stroke={r.kind === 'dead' ? COLOR.bad : r.kind === 'range' ? COLOR.ok : COLOR.aura}
          stroke-dasharray={r.kind === 'dead' ? '4 3' : undefined}
        />
      ))}
    </svg>
  );
}

export function Notice(p: { vm: Pick<BaseViewModel, 'notice' | 'rejection'>; t: T }): VNode | null {
  const key = p.vm.notice ?? p.vm.rejection?.code ?? null;
  if (!key) return null;
  const k = `reason.${key}`;
  const text = p.t(k) === k ? (p.vm.rejection?.message ?? key) : p.t(k);
  return (
    <div data-testid="notice" role="alert">
      {text}
    </div>
  );
}

/** Корневой компонент: панель постройки, карточка, тулбар, подсветка. */
export function BaseEditorView(p: { vm: BaseViewModel; actions: BaseActions; t: T; project: Project }): VNode {
  const { vm, actions, t } = p;
  return (
    <div data-testid="base-editor">
      <PlacementOverlay overlay={vm.overlay} project={p.project} />
      <Toolbar tb={vm.toolbar} t={t} actions={actions} />
      <Notice vm={vm} t={t} />
      {vm.placing ? (
        <PlacementBar placing={vm.placing} t={t} onConfirm={actions.confirmPlacing} onCancel={actions.cancelPlacing} onRotate={actions.rotate} />
      ) : vm.card ? (
        <BuildingCard
          card={vm.card}
          t={t}
          onUpgrade={actions.upgrade}
          onSpeedup={actions.speedup}
          onAskRemove={actions.askRemove}
          onRemove={actions.remove}
          onClear={actions.clearObstacle}
          onRotate={actions.rotate}
          onClose={() => actions.select(null)}
        />
      ) : (
        <ShopPanel entries={vm.shop} t={t} onPick={actions.pickBuilding} />
      )}
    </div>
  );
}
