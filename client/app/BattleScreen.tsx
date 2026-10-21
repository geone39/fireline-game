// Экран боя: канвас PixiRenderer (BattleRenderer 0.3) + панель (ход, ОД, EndTurn/Retreat, итог) над ядром sim (WASM).
// Бой локальный, без сервера (серверная часть рейдов ещё не готова: OQ-C4b-2): на тестовом Snapshot или на составе из армии игрока (battle-prep.ts).
import type { VNode } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import artManifest from '../../art/manifest.json';
import { createBattle, type Battle } from '../core/battle';
import { PixiRenderer } from '../engine/pixi-renderer';
import { browserSources, loadSim } from '../sim-wasm';
import type { Snapshot } from '../sim-wasm/api';
import type { T } from '../ui/base/i18n';
import { TEST_SEED, testSnapshot } from './battle-fixture';
import { selectPanel, stars } from './battle-model';

const atlasUrls = import.meta.glob('../../art/atlases/*.png', { eager: true, query: '?url', import: 'default' });
const atlasUrl = (path: string): string => (atlasUrls[`../../art/${path}`] as string | undefined) ?? path;

export interface BattleScreenProps {
  t: T;
  onExit: () => void;
  /** Что играть; не задан — тестовый Snapshot (автономный бой без сервера). */
  snapshot?: Snapshot;
}

export function BattleScreen(p: BattleScreenProps): VNode {
  const host = useRef<HTMLDivElement>(null);
  const [battle, setBattle] = useState<Battle | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [round, setRound] = useState(0);
  const [, setTick] = useState(0);

  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const r = new PixiRenderer({ resolveAssetUrl: atlasUrl });
    let disposed = false;
    let inited = false;
    let b: Battle | null = null;
    const offs: (() => void)[] = [];
    void (async () => {
      try {
        await r.init(el);
        inited = true;
        if (disposed) return;
        const sim = await loadSim(browserSources(import.meta.env.BASE_URL));
        if (disposed) return;
        b = createBattle({ sim, snapshot: p.snapshot ?? testSnapshot(), seed: TEST_SEED });
        const ctl = b;
        r.setBattle(ctl.renderState());
        r.centerCamera(1);
        offs.push(
          ctl.store.subscribe((s, prev) => {
            if (s.step && s.step.seq !== prev.step?.seq) r.playEvents(s.step.events);
            r.setBattle(ctl.renderState());
            setTick((n) => n + 1);
          }),
          r.on((e) => {
            if (e.type === 'battleSelect') ctl.select(e.id);
            else if (e.type === 'battleTarget') ctl.target(e.id);
            else if (e.type === 'battleCell') ctl.cell(e.x, e.y);
          }),
        );
        if (import.meta.env.DEV) Object.assign(window, { __battle: ctl, __renderer: r }); // отладка в консоли браузера
        setBattle(ctl);
        await r.loadAtlas(artManifest as Record<string, unknown>);
      } catch (e) {
        if (!disposed) setError(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      disposed = true;
      offs.forEach((f) => f());
      b?.dispose();
      setBattle(null);
      if (inited) r.destroy();
    };
  }, [round]);

  const restart = (): void => {
    setError(null);
    setRound((n) => n + 1);
  };

  return (
    <div class="battle" data-testid="battle-screen">
      <div ref={host} class="battle-canvas" />
      {error ? (
        <div class="battle-panel" data-testid="battle-error">
          <p>
            {p.t('battle.error')}
            <br />
            <small>{error}</small>
          </p>
          <button type="button" onClick={restart}>{p.t('app.retry')}</button>
          <button type="button" onClick={p.onExit}>{p.t('battle.exit')}</button>
        </div>
      ) : battle ? (
        <Panel battle={battle} t={p.t} onExit={p.onExit} onAgain={restart} />
      ) : (
        <div class="battle-panel" data-testid="battle-loading">{p.t('battle.loading')}</div>
      )}
    </div>
  );
}

function Panel(p: { battle: Battle; t: T; onExit: () => void; onAgain: () => void }): VNode {
  const { battle: b, t } = p;
  const m = selectPanel(b.store.getState(), t);
  const armedDeploy = m.armed?.kind === 'deploy' ? m.armed.id : null;
  const armedSpell = m.armed?.kind === 'spell' ? m.armed.id : null;
  return (
    <div class="battle-panel" data-testid="battle-panel">
      <div class="battle-row">
        <b data-testid="battle-turn">{m.phase === 'deploy' ? t('battle.deploy') : t('battle.turn', { n: Math.min(m.turn, m.maxTurns), max: m.maxTurns })}</b>
        {m.phase !== 'over' && (
          <button type="button" data-testid="retreat" onClick={() => b.retreat()}>{t('battle.retreat')}</button>
        )}
      </div>

      {m.phase === 'deploy' && (
        <div class="battle-row" data-testid="stock">
          {m.stock.units.filter((s) => s.Count > 0).map((s) => (
            <button type="button" key={s.ID} aria-pressed={armedDeploy === s.ID} data-testid={`deploy-${s.ID}`} onClick={() => b.armDeploy(s.ID, s.Level)}>
              {s.ID} ×{s.Count}
            </button>
          ))}
          {m.stock.hero && !m.stock.hero.used && (
            <button type="button" aria-pressed={armedDeploy === m.stock.hero.id} data-testid={`deploy-${m.stock.hero.id}`} onClick={() => b.armDeploy(m.stock.hero!.id, m.stock.hero!.level)}>
              {t('battle.hero_label')}: {m.stock.hero.id}
            </button>
          )}
          <button type="button" data-testid="start" disabled={!m.canStart} onClick={() => b.start()}>{t('battle.start')}</button>
        </div>
      )}

      {m.phase === 'player' && (
        <>
          <div class="battle-row" data-testid="unit">
            {m.unit ? (
              <span>
                {m.unit.type} · HP {m.unit.hp}/{m.unit.maxHp} · {t('battle.ap')} {m.unit.ap}/{m.unit.maxAp}
              </span>
            ) : (
              <small>{t('battle.hint_select')}</small>
            )}
            {m.unit?.hasAbility && <button type="button" data-testid="hero-ability" onClick={() => b.useHero()}>{t('battle.hero')}</button>}
            {m.unit?.canUnload && <button type="button" data-testid="unload" onClick={() => b.unload()}>{t('battle.unload')}</button>}
          </div>
          <div class="battle-row">
            {m.stock.spells.filter((s) => s.Count > 0).map((s) => (
              <button type="button" key={s.ID} aria-pressed={armedSpell === s.ID} data-testid={`spell-${s.ID}`} onClick={() => b.armSpell(s.ID, s.Level)}>
                {s.ID} ×{s.Count}
              </button>
            ))}
            <button type="button" data-testid="end-turn" onClick={() => b.endTurn()}>{t('battle.end_turn')}</button>
          </div>
        </>
      )}

      {m.armed && m.phase !== 'over' && <small>{t('battle.pick_cell')}</small>}
      {m.reject && <div class="battle-reject" data-testid="battle-reject">{m.reject}</div>}

      {m.result && (
        <div class="battle-result" data-testid="battle-result">
          <h3>{t('battle.result')}</h3>
          <div class="battle-stars" data-testid="stars">{stars(m.result.stars)}</div>
          <div data-testid="destruction">{t('battle.destruction', { n: m.result.destruction })}</div>
          <div>{m.result.reason} · {t('battle.turns', { n: m.result.turns })}</div>
          <button type="button" data-testid="again" onClick={p.onAgain}>{t('battle.again')}</button>
          <button type="button" data-testid="exit" onClick={p.onExit}>{t('battle.exit')}</button>
        </div>
      )}
    </div>
  );
}
