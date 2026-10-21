// Экран лаборатории: текущее исследование (прогресс, ускорение, отмена), узлы по предметам со статусами, перки.
// Чистые функции от пропсов, состояние — в client/ui/army/store (одно общее состояние армии и лаборатории).
import type { VNode } from 'preact';
import { shortText } from '../army/components';
import { formatCost, formatDuration, formatGems, type T } from '../army/i18n';
import type { IconResolver } from '../army/icons';
import { ArmyStyles, Icon, Meter, NoticeBar } from '../army/shared';
import type { ArmyActions } from '../army/store/controller';
import type { ActiveResearchVM, ArmyViewModel, LabGroupVM, LabNodeVM } from '../army/store/view-model';

export function nodeStatusText(n: LabNodeVM, t: T): string {
  switch (n.status) {
    case 'done':
      return t('lab.done');
    case 'researching':
      return t('ui.lab.status.researching');
    case 'soon':
      return t('lab.coming_soon');
    case 'lab_level':
      return t('ui.lab.status.lab_level', { n: n.needLab ?? 0, have: n.haveLab });
    case 'prerequisite':
      return t('ui.lab.status.prerequisite', { name: n.prereqNameKey ? t(n.prereqNameKey) : '' });
    case 'locked_hq':
      return t('ui.lab.status.locked_hq', { n: n.needHq ?? 0 });
    case 'available':
      return t('ui.lab.status.available');
  }
}

function blockedText(n: LabNodeVM, t: T): string {
  switch (n.blocked) {
    case 'lab_busy':
      return t('reason.lab_busy');
    case 'no_funds':
      return n.short ? shortText(n.short, t) : t('reason.insufficient_resources');
    case 'no_lab':
      return t('ui.lab.no_lab');
    default:
      return '';
  }
}

export function ActiveResearch(p: { active: ActiveResearchVM; t: T; actions: Pick<ArmyActions, 'askCancel' | 'cancelResearch' | 'speedupResearch'> }): VNode {
  const { active: a, t } = p;
  return (
    <section class="fl-card" data-testid="lab-active" aria-label={t('lab.active')}>
      <h3>{t('lab.active')}</h3>
      <strong data-testid="lab-active-name">{t(a.nameKey)}</strong>
      <div class="fl-muted" data-testid="lab-active-left">
        {formatDuration(a.remainingS, t)} {t('ui.army.left')}
      </div>
      <Meter id="lab-active-meter" label={t(a.nameKey)} percent={a.percent} />
      <div class="fl-row">
        <button
          type="button"
          data-testid="lab-speedup"
          disabled={a.speedupBlocked !== null}
          aria-describedby={a.speedupBlocked ? 'lab-speedup-block' : undefined}
          onClick={() => void p.actions.speedupResearch()}
        >
          {t('ui.army.speedup_for', { gems: formatGems(a.speedupGems, t) })}
        </button>
        {a.confirming ? (
          <span class="fl-row" role="group" aria-label={t('ui.lab.confirm_cancel')}>
            <span>{t('ui.lab.confirm_cancel')}</span>
            <button type="button" data-testid="lab-cancel-yes" onClick={() => void p.actions.cancelResearch()}>
              {t('ui.army.yes')}
            </button>
            <button type="button" data-testid="lab-cancel-no" onClick={() => p.actions.askCancel(null)}>
              {t('ui.army.no')}
            </button>
          </span>
        ) : (
          <button type="button" data-testid="lab-cancel" onClick={() => p.actions.askCancel('research')}>
            {t('lab.cancel')}
          </button>
        )}
      </div>
      {a.gemsShort && (
        <small id="lab-speedup-block" data-testid="lab-speedup-block" class="fl-bad">
          {t('ui.army.no_gems', { need: a.gemsShort.need, have: a.gemsShort.have })}
        </small>
      )}
    </section>
  );
}

function NodeCard(p: { n: LabNodeVM; t: T; actions: Pick<ArmyActions, 'research'>; icon?: IconResolver }): VNode {
  const { n, t } = p;
  const blockId = `node-${n.id}-block`;
  const tone = n.status === 'done' ? 'fl-ok' : n.status === 'available' ? '' : 'fl-muted';
  return (
    <li class="fl-card" data-testid={`node-${n.id}`} data-status={n.status}>
      <div class="fl-row">
        {n.itemId && <Icon id={n.itemId} icon={p.icon} />}
        <div style={{ minWidth: 0 }}>
          <strong>{t(n.nameKey)}</strong>
          <div class="fl-muted" data-testid={`node-${n.id}-step`}>
            {n.fromLevel !== null ? t('ui.lab.step', { from: n.fromLevel, to: n.toLevel }) : n.perkKey ? t(`perk.${n.perkKey}`) : ''}
          </div>
        </div>
      </div>
      {n.perkKey && <div class="fl-muted">{t(`perk.${n.perkKey}.desc`)}</div>}
      <div class="fl-muted" data-testid={`node-${n.id}-price`}>
        {formatCost({ fuel: n.fuel }, t)} · {formatDuration(n.hours * 3600, t)}
      </div>
      <div class={tone} data-testid={`node-${n.id}-status`}>
        {nodeStatusText(n, t)}
      </div>
      {n.status === 'available' && (
        <p>
          <button
            type="button"
            data-testid={`node-${n.id}-start`}
            disabled={n.blocked !== null}
            aria-describedby={n.blocked ? blockId : undefined}
            onClick={() => void p.actions.research(n.id)}
          >
            {t('lab.research')}
          </button>
          {n.blocked && (
            <small id={blockId} data-testid={blockId} class="fl-bad">
              {' '}
              {blockedText(n, t)}
            </small>
          )}
        </p>
      )}
    </li>
  );
}

function groupTitleKey(g: LabGroupVM): string {
  return g.kind === 'perk' ? 'ui.lab.group.perk' : `ui.army.group.${g.kind}`;
}

export function LabScreen(p: { vm: ArmyViewModel; actions: ArmyActions; t: T; icon?: IconResolver }): VNode {
  const { vm, t, actions } = p;
  const lab = vm.lab;
  return (
    <div class="fl-army" data-testid="lab-screen" aria-busy={vm.busy}>
      <ArmyStyles />
      <h2>{t('lab.title')}</h2>
      <NoticeBar notice={vm.notice} t={t} onClose={actions.dismissNotice} />
      {!lab.loaded ? (
        <p data-testid="lab-loading" role="status">
          {t('ui.army.loading')}
        </p>
      ) : (
        <>
          <p data-testid="lab-level">{lab.hasLab ? t('ui.lab.level', { n: lab.labLevel ?? 0 }) : t('ui.lab.no_lab')}</p>
          {lab.active ? (
            <ActiveResearch active={lab.active} t={t} actions={actions} />
          ) : (
            <p class="fl-muted" data-testid="lab-idle">
              {t('lab.idle')}
            </p>
          )}
          {lab.groups.map((g) => (
            <section key={g.kind} data-testid={`lab-group-${g.kind}`} aria-label={t(groupTitleKey(g))}>
              <h3>{t(groupTitleKey(g))}</h3>
              <ul class="fl-grid">
                {g.nodes.map((n) => (
                  <NodeCard key={n.id} n={n} t={t} actions={actions} icon={p.icon} />
                ))}
              </ul>
            </section>
          ))}
        </>
      )}
    </div>
  );
}
