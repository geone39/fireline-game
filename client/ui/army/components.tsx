// Экран армии: вместимость по пулам, состав, очереди по зданиям, заказ юнитов. Чистые функции от пропсов (как client/ui/base):
// всё состояние — в store/controller, подписку делает useArmyViewModel (hook.ts). Подключение в шелл: см. docs/open-questions.md (OQ-UIARMY-1).
import type { VNode } from 'preact';
import type { ArmyActions } from './store/controller';
import type { ArmyViewModel, OrderVM, PerkVM, PoolVM, QueueVM, StockVM, TrainUnitVM, TrainVM } from './store/view-model';
import { formatCost, formatDuration, formatGems, type T } from './i18n';
import type { IconResolver } from './icons';
import { ArmyStyles, Icon, Meter, NoticeBar } from './shared';

type Shortfall = Record<string, { need: number; have: number }>;

export function shortText(short: Shortfall, t: T): string {
  const need: Record<string, number> = {};
  const have: Record<string, number> = {};
  for (const [r, v] of Object.entries(short)) {
    need[r] = v.need;
    have[r] = v.have;
  }
  return t('ui.army.no_funds', { need: formatCost(need, t), have: formatCost(have, t) });
}

export function perkValueText(p: PerkVM, t: T): string {
  return typeof p.value === 'number' ? `+${p.value}%` : t('ui.lab.perk_on');
}

/** «Казарма» или «Казарма #2», если зданий этого типа несколько. */
export function buildingLabel(b: { nameKey: string; ordinal: number; sameType: number }, t: T): string {
  return b.sameType > 1 ? `${t(b.nameKey)} #${b.ordinal}` : t(b.nameKey);
}

export function PoolBars(p: { pools: readonly PoolVM[]; t: T }): VNode {
  return (
    <section data-testid="pools" aria-label={p.t('ui.army.pools')}>
      <h3>{p.t('ui.army.pools')}</h3>
      <ul class="fl-grid">
        {p.pools.map((pool) => (
          <li key={pool.id} class="fl-card" data-testid={`pool-${pool.id}`} data-full={pool.full ? 'true' : undefined}>
            <div class="fl-row" style={{ justifyContent: 'space-between' }}>
              <strong>{p.t(pool.labelKey)}</strong>
              <span data-testid={`pool-${pool.id}-value`}>
                {pool.used}/{pool.capacity}
              </span>
            </div>
            <Meter
              id={`pool-${pool.id}-meter`}
              label={p.t(pool.labelKey)}
              percent={pool.percent}
              full={pool.full}
              text={`${pool.used}/${pool.capacity}`}
            />
            <div class="fl-muted">
              {pool.full ? <span class="fl-bad">{p.t('ui.army.pool_full')}</span> : `${p.t('army.free')}: ${pool.free}`}
              {pool.perkPct > 0 && <span data-testid={`pool-${pool.id}-perk`}> · +{pool.perkPct}%</span>}
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function PerkList(p: { perks: readonly PerkVM[]; t: T }): VNode | null {
  if (p.perks.length === 0) return null;
  return (
    <section data-testid="perks" aria-label={p.t('ui.army.perks')}>
      <h3>{p.t('ui.army.perks')}</h3>
      <ul>
        {p.perks.map((k) => (
          <li key={k.key} data-testid={`perk-${k.key}`} title={p.t(k.descKey)}>
            <strong>{p.t(k.labelKey)}</strong>: {perkValueText(k, p.t)} <span class="fl-muted">{p.t(k.descKey)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function StockList(p: { stock: readonly StockVM[]; t: T; icon?: IconResolver }): VNode {
  return (
    <section data-testid="stock" aria-label={p.t('ui.army.stock')}>
      <h3>{p.t('ui.army.stock')}</h3>
      {p.stock.length === 0 ? (
        <p data-testid="stock-empty">{p.t('ui.army.empty')}</p>
      ) : (
        <ul class="fl-grid">
          {p.stock.map((s) => (
            <li key={s.id} class="fl-card fl-row" data-testid={`stock-${s.id}`}>
              <Icon id={s.id} icon={p.icon} />
              <div style={{ minWidth: 0 }}>
                <strong>{p.t(s.nameKey)}</strong>{' '}
                <span data-testid={`stock-${s.id}-level`} class="fl-muted">
                  {p.t('ui.army.lv', { n: s.level })}
                </span>
                <div>
                  <span data-testid={`stock-${s.id}-count`}>×{s.count}</span>
                  {s.inRaid > 0 && (
                    <span class="fl-muted" data-testid={`stock-${s.id}-raid`}>
                      {' '}
                      · {p.t('army.in_raid')} {s.inRaid}
                    </span>
                  )}
                  {s.queued > 0 && (
                    <span class="fl-muted" data-testid={`stock-${s.id}-queued`}>
                      {' '}
                      · {p.t('ui.army.queued', { n: s.queued })}
                    </span>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function OrderRow(p: { o: OrderVM; t: T; actions: Pick<ArmyActions, 'askCancel' | 'cancelOrder'>; icon?: IconResolver }): VNode {
  const { o, t } = p;
  return (
    <li key={o.orderId} class="fl-row" data-testid={`order-${o.orderId}`} data-state={o.state}>
      <Icon id={o.unitId} icon={p.icon} />
      <div style={{ flex: '1 1 8em', minWidth: 0 }}>
        <strong>{t(o.nameKey)}</strong> <span data-testid={`order-${o.orderId}-count`}>×{o.remaining}</span>
        <div class="fl-muted">
          {o.state === 'training' ? (
            <>
              {t('army.queue.training')}: {formatDuration(o.unitLeftS ?? 0, t)} {t('ui.army.left')}
            </>
          ) : (
            t('army.queue.waiting')
          )}
        </div>
        {o.state === 'training' && <Meter id={`order-${o.orderId}-meter`} label={t(o.nameKey)} percent={o.unitPercent} />}
      </div>
      {o.confirming ? (
        <span class="fl-row" role="group" aria-label={t('ui.army.confirm_cancel')}>
          <span>{t('ui.army.confirm_cancel')}</span>
          <button type="button" data-testid={`order-${o.orderId}-cancel-yes`} onClick={() => void p.actions.cancelOrder(o.orderId)}>
            {t('ui.army.yes')}
          </button>
          <button type="button" data-testid={`order-${o.orderId}-cancel-no`} onClick={() => p.actions.askCancel(null)}>
            {t('ui.army.no')}
          </button>
        </span>
      ) : (
        <button
          type="button"
          data-testid={`order-${o.orderId}-cancel`}
          aria-label={`${t('army.cancel')}: ${t(o.nameKey)} ×${o.remaining}`}
          onClick={() => p.actions.askCancel(`order:${o.orderId}`)}
        >
          {t('army.cancel')}
        </button>
      )}
    </li>
  );
}

export function QueueCard(p: { q: QueueVM; t: T; actions: Pick<ArmyActions, 'askCancel' | 'cancelOrder' | 'speedupQueue'>; icon?: IconResolver }): VNode {
  const { q, t } = p;
  const blockId = `queue-${q.buildingUid}-block`;
  return (
    <li class="fl-card" data-testid={`queue-${q.buildingUid}`}>
      <div class="fl-row" style={{ justifyContent: 'space-between' }}>
        <strong>{t('ui.army.queue_of', { name: buildingLabel(q, t) })}</strong>
        {q.level !== null && <span class="fl-muted">{t('army.level')} {q.level}</span>}
      </div>
      <ul>
        {q.orders.map((o) => (
          <OrderRow key={o.orderId} o={o} t={t} actions={p.actions} icon={p.icon} />
        ))}
      </ul>
      <div class="fl-muted" data-testid={`queue-${q.buildingUid}-left`}>
        {t('ui.army.queue_total')}: {formatDuration(q.remainingS, t)} {t('ui.army.left')}
      </div>
      <Meter id={`queue-${q.buildingUid}-meter`} label={t('ui.army.queue_total')} percent={q.percent} />
      <p>
        <button
          type="button"
          data-testid={`queue-${q.buildingUid}-speedup`}
          disabled={q.speedupBlocked !== null}
          aria-describedby={q.speedupBlocked ? blockId : undefined}
          onClick={() => void p.actions.speedupQueue(q.buildingUid)}
        >
          {t('ui.army.speedup_for', { gems: formatGems(q.speedupGems, t) })}
        </button>
        {q.gemsShort && (
          <small id={blockId} data-testid={blockId} class="fl-bad">
            {' '}
            {t('ui.army.no_gems', { need: q.gemsShort.need, have: q.gemsShort.have })}
          </small>
        )}
      </p>
    </li>
  );
}

export function blockText(u: TrainUnitVM, queueFull: boolean, t: T): string {
  switch (u.blocked) {
    case 'locked': {
      const parts: string[] = [];
      if (u.needBuilding) parts.push(`${t(`b.${u.needBuilding.type.replace(/^b_/, '')}`)} ${u.needBuilding.level}`);
      if (u.needHq !== null) parts.push(t('ui.army.need_hq', { n: u.needHq }));
      return t('ui.army.need', { need: parts.join(', ') });
    }
    case 'queue_full':
      return t('reason.queue_full');
    case 'bad_count':
      return u.group > 1 ? `${t('reason.not_multiple_of_group')} (${t('ui.army.group', { n: u.group })})` : t('ui.army.count');
    case 'housing_full':
      return `${t('reason.housing_full')}. ${t('ui.army.max_fits', { n: u.maxCount })}`;
    case 'no_funds':
      return u.short ? shortText(u.short, t) : t('reason.insufficient_resources');
    default:
      return queueFull ? t('reason.queue_full') : '';
  }
}

function TrainRow(p: { u: TrainUnitVM; queueFull: boolean; t: T; actions: Pick<ArmyActions, 'setCount' | 'setMax' | 'train'>; icon?: IconResolver }): VNode {
  const { u, t, actions } = p;
  const id = u.id;
  const blockId = `train-${id}-block`;
  const locked = u.blocked === 'locked';
  const reason = u.blocked ? blockText(u, p.queueFull, t) : '';
  return (
    <li class="fl-card" data-testid={`train-${id}`} data-blocked={u.blocked ?? undefined}>
      <div class="fl-row">
        <Icon id={id} icon={p.icon} />
        <div style={{ minWidth: 0 }}>
          <strong>{t(u.nameKey)}</strong> <span class="fl-muted">{t('ui.army.lv', { n: u.level })}</span>
          <div class="fl-muted">
            {t('ui.army.cost')}: {formatCost(u.unitCost, t)} · {t('ui.army.housing')}: {u.housing} · {t('ui.army.time')}: {formatDuration(u.trainS, t)}
            {u.group > 1 && ` · ${t('ui.army.group', { n: u.group })}`}
          </div>
        </div>
      </div>
      <div class="fl-row" role="group" aria-label={`${t('ui.army.count')}: ${t(u.nameKey)}`}>
        <button type="button" data-testid={`train-${id}-dec`} disabled={locked || u.count <= u.group} aria-label={t('ui.army.dec')} onClick={() => actions.setCount(id, u.count - u.group)}>
          −
        </button>
        <label>
          <span class="fl-muted">{t('ui.army.count')} </span>
          <input
            type="number"
            data-testid={`train-${id}-count`}
            min={u.group}
            step={u.group}
            value={u.count}
            disabled={locked}
            onInput={(e) => actions.setCount(id, Number((e.currentTarget as HTMLInputElement).value))}
          />
        </label>
        <button type="button" data-testid={`train-${id}-inc`} disabled={locked} aria-label={t('ui.army.inc')} onClick={() => actions.setCount(id, u.count + u.group)}>
          +
        </button>
        <button type="button" data-testid={`train-${id}-max`} disabled={locked || u.maxCount < u.group} onClick={() => actions.setMax(id)}>
          {t('ui.army.max')}
        </button>
      </div>
      <div class="fl-muted" data-testid={`train-${id}-total`}>
        {formatCost(u.totalCost, t)} · {formatDuration(u.totalTimeS, t)}
      </div>
      <p>
        <button
          type="button"
          data-testid={`train-${id}-submit`}
          disabled={u.blocked !== null}
          aria-describedby={u.blocked ? blockId : undefined}
          onClick={() => void actions.train(id)}
        >
          {t('army.train')}
        </button>
        {u.blocked && (
          <small id={blockId} data-testid={blockId} class={locked ? 'fl-muted' : 'fl-bad'}>
            {' '}
            {reason}
          </small>
        )}
      </p>
    </li>
  );
}

export function TrainPanel(p: { train: TrainVM; t: T; actions: Pick<ArmyActions, 'selectBuilding' | 'setCount' | 'setMax' | 'train'>; icon?: IconResolver }): VNode {
  const { train, t } = p;
  return (
    <section data-testid="train" aria-label={t('army.train')}>
      <h3>{t('army.train')}</h3>
      {train.buildings.length === 0 ? (
        <p data-testid="train-empty">{t('ui.army.no_producers')}</p>
      ) : (
        <>
          <div class="fl-row" role="group" aria-label={t('ui.army.building')}>
            {train.buildings.map((b) => (
              <button
                key={b.uid}
                type="button"
                data-testid={`train-building-${b.uid}`}
                aria-pressed={b.selected}
                onClick={() => p.actions.selectBuilding(b.uid)}
              >
                {buildingLabel(b, t)} · {b.level}
                {b.orders > 0 ? ` (${b.orders})` : ''}
              </button>
            ))}
          </div>
          <ul class="fl-grid">
            {train.units.map((u) => (
              <TrainRow key={u.id} u={u} queueFull={train.queueFull} t={t} actions={p.actions} icon={p.icon} />
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

export function ArmyScreen(p: { vm: ArmyViewModel; actions: ArmyActions; t: T; icon?: IconResolver }): VNode {
  const { vm, t, actions } = p;
  return (
    <div class="fl-army" data-testid="army-screen" aria-busy={vm.busy}>
      <ArmyStyles />
      <h2>{t('army.title')}</h2>
      <NoticeBar notice={vm.notice} t={t} onClose={actions.dismissNotice} />
      {!vm.loaded ? (
        <p data-testid="army-loading" role="status">
          {t('ui.army.loading')}
        </p>
      ) : (
        <>
          <PoolBars pools={vm.pools} t={t} />
          <PerkList perks={vm.perks} t={t} />
          <StockList stock={vm.stock} t={t} icon={p.icon} />
          <section data-testid="queues" aria-label={t('army.queue')}>
            <h3>{t('army.queue')}</h3>
            {vm.queues.length === 0 ? (
              <p data-testid="queues-empty">{t('army.queue.empty')}</p>
            ) : (
              <ul class="fl-grid">
                {vm.queues.map((q) => (
                  <QueueCard key={q.buildingUid} q={q} t={t} actions={actions} icon={p.icon} />
                ))}
              </ul>
            )}
          </section>
          <TrainPanel train={vm.train} t={t} actions={actions} icon={p.icon} />
        </>
      )}
    </div>
  );
}
