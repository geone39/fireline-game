// HUD: кошелёк, строители, накопленное в сборщиках, таймеры построек. Чистые функции от пропсов (как ui/base).
import type { VNode } from 'preact';
import { formatDuration, type T } from '../ui/base/i18n';
import type { HudVM } from './hud-model';

const box = {
  position: 'absolute',
  top: 0,
  left: 0,
  right: 0,
  display: 'flex',
  flexWrap: 'wrap',
  gap: '6px 16px',
  padding: '6px 10px',
  color: '#fff',
  font: '14px/1.3 system-ui, sans-serif',
  background: 'rgba(0,0,0,.55)',
  pointerEvents: 'none',
} as const;

export function ResourceBar(p: { wallet: HudVM['wallet']; t: T }): VNode {
  return (
    <ul data-testid="wallet" style={{ display: 'flex', gap: '14px', margin: 0, padding: 0, listStyle: 'none' }}>
      {p.wallet.map((w) => (
        <li key={w.res} data-testid={`wallet-${w.res}`} title={p.t(`res.${w.res}`)}>
          <span>{p.t(`res.${w.res}`)}</span>: <strong>{w.amount.toLocaleString('en-US')}</strong>
        </li>
      ))}
    </ul>
  );
}

export function BuilderIndicator(p: { builders: HudVM['builders']; t: T }): VNode {
  const { total, free } = p.builders;
  return (
    <span data-testid="hud-builders" aria-label={p.t('hud.builders')}>
      {p.t('hud.builders')}: <strong>{free}</strong>/{total} {p.t('hud.free')}{' '}
      <span aria-hidden="true">{Array.from({ length: total }, (_, i) => (i < free ? '●' : '○')).join('')}</span>
    </span>
  );
}

export function CollectorIndicators(p: { collectors: HudVM['collectors']; t: T }): VNode {
  return (
    <span data-testid="hud-collectors">
      {p.t('hud.collectors')}:{' '}
      {p.collectors.length === 0
        ? p.t('hud.collectors_empty')
        : p.collectors.map((c) => (
            <span key={c.res} data-testid={`collector-${c.res}`} style={{ marginRight: '8px' }}>
              {p.t(`res.${c.res}`)} <strong>{c.amount.toLocaleString('en-US')}</strong>
            </span>
          ))}
    </span>
  );
}

export function TimerList(p: { timers: HudVM['timers']; t: T }): VNode {
  return (
    <ul data-testid="hud-timers" aria-label={p.t('hud.timers')} style={{ margin: 0, padding: 0, listStyle: 'none', display: 'flex', gap: '12px' }}>
      {p.timers.length === 0 && <li data-testid="hud-timers-empty">{p.t('hud.timers_empty')}</li>}
      {p.timers.map((x) => (
        <li key={x.id} data-testid={`timer-${x.uid}`}>
          {p.t(x.nameKey)} ({p.t(x.kind === 'build' ? 'ui.building' : 'ui.upgrading')}
          {x.kind === 'build' ? '' : `, ${p.t('hud.upgrade_to', { n: x.toLevel })}`}):{' '}
          <strong>{x.remainingS > 0 ? formatDuration(x.remainingS, p.t) : p.t('hud.ready')}</strong>
        </li>
      ))}
    </ul>
  );
}

export function Hud(p: { vm: HudVM; t: T }): VNode {
  const { vm, t } = p;
  return (
    <header data-testid="hud" style={box}>
      {vm.profile && (
        <span data-testid="hud-profile">
          {vm.profile.nickname} · {t('hud.hq', { n: vm.profile.hqLevel })} · {t('hud.trophies')} {vm.profile.trophies}
        </span>
      )}
      <ResourceBar wallet={vm.wallet} t={t} />
      <BuilderIndicator builders={vm.builders} t={t} />
      <CollectorIndicators collectors={vm.collectors} t={t} />
      <TimerList timers={vm.timers} t={t} />
    </header>
  );
}
