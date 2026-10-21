// Общие кирпичики экранов армии и лаборатории: стили, иконка, полоса прогресса, плашка отказа.
// Чистые функции от пропсов (без хуков), как в client/ui/base: тестируются без DOM.
import type { VNode } from 'preact';
import type { IconResolver } from './icons';
import { noticeText, type Notice } from './store/view-model';
import type { T } from './i18n';

/** Адаптив: сетки на auto-fill, на узком экране карточки в одну колонку; кнопки ≥ 40px по высоте для касания. */
export const ARMY_CSS = `
.fl-army{font:14px/1.4 system-ui,sans-serif;color:#e8eaed;background:rgba(0,0,0,.6);padding:8px 12px;box-sizing:border-box;max-width:100%;pointer-events:auto}
.fl-army h2,.fl-army h3,.fl-army h4{margin:.6em 0 .3em}
.fl-army ul{list-style:none;margin:0;padding:0}
.fl-army button{min-height:40px;min-width:40px;padding:4px 10px;cursor:pointer}
.fl-army button:disabled{cursor:not-allowed;opacity:.55}
.fl-army input[type=number]{width:5em;min-height:40px;box-sizing:border-box}
.fl-army .fl-grid{display:grid;gap:8px;grid-template-columns:repeat(auto-fill,minmax(min(100%,220px),1fr))}
.fl-army .fl-card{border:1px solid rgba(255,255,255,.2);border-radius:6px;padding:8px;min-width:0}
.fl-army .fl-row{display:flex;flex-wrap:wrap;gap:6px;align-items:center}
.fl-army .fl-muted{opacity:.75;font-size:12px}
.fl-army .fl-bad{color:#ff8a8a}
.fl-army .fl-ok{color:#7be0a5}
.fl-army .fl-meter{height:8px;background:rgba(255,255,255,.18);border-radius:4px;overflow:hidden}
.fl-army .fl-meter>span{display:block;height:100%;background:#4c8dff}
.fl-army .fl-meter.full>span{background:#e5484d}
.fl-army .fl-icon{display:inline-block;flex:none;border-radius:4px;background-color:rgba(255,255,255,.12);text-align:center;font-size:12px;line-height:40px;overflow:hidden}
.fl-army [role=tablist]{display:flex;gap:4px;flex-wrap:wrap}
.fl-army [role=tab][aria-selected=true]{outline:2px solid #4c8dff}
.fl-army .fl-notice{border:1px solid #e5484d;border-radius:6px;padding:6px 10px;margin:6px 0;display:flex;gap:8px;align-items:center;justify-content:space-between}
@media (max-width:600px){.fl-army{padding:6px 8px}.fl-army .fl-grid{grid-template-columns:1fr}}
`;

export function ArmyStyles(): VNode {
  return <style data-testid="army-styles">{ARMY_CSS}</style>;
}

const ICON_PX = 40;

export function Icon(p: { id: string; icon?: IconResolver }): VNode {
  const spec = p.icon?.(p.id) ?? null;
  if (!spec) {
    const label = p.id.replace(/^[a-z]_/, '').slice(0, 3);
    return (
      <span class="fl-icon" aria-hidden="true" data-testid={`icon-${p.id}`} style={{ width: `${ICON_PX}px`, height: `${ICON_PX}px` }}>
        {label}
      </span>
    );
  }
  const k = ICON_PX / Math.max(spec.w, spec.h);
  return (
    <span
      class="fl-icon"
      aria-hidden="true"
      data-testid={`icon-${p.id}`}
      style={{
        width: `${ICON_PX}px`,
        height: `${ICON_PX}px`,
        backgroundImage: `url(${spec.url})`,
        backgroundSize: `${spec.atlasW * k}px ${spec.atlasH * k}px`,
        backgroundPosition: `${-spec.x * k}px ${-spec.y * k}px`,
        backgroundRepeat: 'no-repeat',
      }}
    />
  );
}

export function Meter(p: { id: string; label: string; percent: number; full?: boolean; text?: string }): VNode {
  return (
    <div>
      <div
        class={`fl-meter${p.full ? ' full' : ''}`}
        role="progressbar"
        aria-label={p.label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={p.percent}
        aria-valuetext={p.text}
        data-testid={p.id}
      >
        <span style={{ width: `${p.percent}%` }} />
      </div>
    </div>
  );
}

/** Отказ сервера простым языком; role=alert, чтобы скринридер зачитал. */
export function NoticeBar(p: { notice: Notice | null; t: T; onClose: () => void }): VNode | null {
  if (!p.notice) return null;
  return (
    <div class="fl-notice" role="alert" data-testid="notice" data-code={p.notice.code} data-reason={p.notice.reason ?? undefined}>
      <span data-testid="notice-text">{noticeText(p.notice, p.t)}</span>
      <button type="button" data-testid="notice-close" onClick={p.onClose}>
        {p.t('ui.army.close')}
      </button>
    </div>
  );
}
