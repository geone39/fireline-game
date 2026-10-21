// Выбор состава для локального боя из реальной армии игрока (docs/design/army-lab.md §6.5, пресет «Вся армия»).
import type { VNode } from 'preact';
import { useState } from 'preact/hooks';
import type { T } from '../ui/base/i18n';
import { canStartBattle, selectAll, selectedTotal, setCount, type PrepEntry, type Selection } from './battle-prep';

export interface BattlePrepProps {
  entries: readonly PrepEntry[];
  loaded: boolean;
  t: T;
  onStart: (sel: Selection) => void;
  /** Бой на тестовом составе (как раньше, без армии игрока). */
  onTest: () => void;
  onToArmy: () => void;
}

export function BattlePrep(p: BattlePrepProps): VNode {
  const { t, entries } = p;
  // null — игрок ещё не менял выбор: по умолчанию «Вся армия» (следует за обновлением запаса).
  const [picked, setSel] = useState<Selection | null>(null);
  const sel = picked ?? selectAll(entries);
  const rows = entries.filter((e) => e.kind !== 'spell');
  const spells = entries.filter((e) => e.kind === 'spell');
  const row = (e: PrepEntry): VNode => (
    <li key={e.id} data-testid={`prep-${e.id}`}>
      <span>{t(e.nameKey)}</span> <small>{t('battle.prep.level', { n: e.level })}</small>{' '}
      {e.unsupported ? (
        <small data-testid={`prep-${e.id}-unsupported`}>{t(`battle.prep.${e.unsupported}`)}</small>
      ) : (
        <>
          <input
            type="number"
            min={0}
            max={e.available}
            value={sel[e.id] ?? 0}
            aria-label={t(e.nameKey)}
            data-testid={`prep-count-${e.id}`}
            onInput={(ev) => setSel(setCount(sel, e, Number((ev.target as HTMLInputElement).value)))}
          />{' '}
          <small>/ {e.available}</small>
        </>
      )}
    </li>
  );
  return (
    <div class="battle-prep" data-testid="battle-prep">
      <h2>{t('battle.prep.title')}</h2>
      <p><small>{t('battle.prep.local_note')}</small></p>
      {!p.loaded ? (
        <p data-testid="prep-unavailable">{t('battle.prep.unavailable')}</p>
      ) : entries.length === 0 ? (
        <p data-testid="prep-empty">{t('battle.prep.empty')}</p>
      ) : (
        <>
          <ul>{rows.map(row)}</ul>
          {spells.length > 0 && (
            <>
              <h3>{t('battle.prep.spells')}</h3>
              <ul>{spells.map(row)}</ul>
            </>
          )}
          <p data-testid="prep-total">{t('battle.prep.total', { n: selectedTotal(entries, sel) })}</p>
        </>
      )}
      <div class="battle-row">
        <button type="button" data-testid="prep-all" disabled={entries.length === 0} onClick={() => setSel(selectAll(entries))}>{t('battle.prep.all')}</button>
        <button type="button" data-testid="prep-none" disabled={entries.length === 0} onClick={() => setSel({})}>{t('battle.prep.none')}</button>
        <button type="button" data-testid="prep-start" disabled={!canStartBattle(entries, sel)} onClick={() => p.onStart(sel)}>{t('battle.start')}</button>
      </div>
      <div class="battle-row">
        <button type="button" data-testid="prep-to-army" onClick={p.onToArmy}>{t('battle.prep.to_army')}</button>
        <button type="button" data-testid="prep-test" onClick={p.onTest}>{t('battle.prep.test')}</button>
      </div>
    </div>
  );
}
