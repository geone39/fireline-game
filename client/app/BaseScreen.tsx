// Экран базы: канвас PixiRenderer + HUD + слот редактора базы (client/ui/base, модуль base).
import type { VNode } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import artManifest from '../../art/manifest.json';
import { PixiRenderer } from '../engine/pixi-renderer';
import type { Camera } from '../engine/renderer';
import { tileToScreen } from '../engine/iso';
import { BaseEditorView, type Project } from '../ui/base/components';
import type { T } from '../ui/base/i18n';
import type { App } from './app';
import { selectHud } from './hud-model';
import { Hud } from './hud';
import type { Content } from './content';

const atlasUrls = import.meta.glob('../../art/atlases/*.png', { eager: true, query: '?url', import: 'default' });
const atlasUrl = (path: string): string => (atlasUrls[`../../art/${path}`] as string | undefined) ?? path;

/**
 * Камера рендера. PixiRenderer пока не отдаёт её публично (запрос client-engine, OQ-A2), поэтому читаем
 * getCamera(), если он появится, иначе — поле camera (только чтение).
 */
export function cameraOf(r: unknown): Camera {
  const c = r as { getCamera?: () => Camera; camera?: Camera };
  return c.getCamera?.() ?? c.camera ?? { x: 0, y: 0, zoom: 1 };
}

export function makeProject(cam: Camera): Project {
  return (tx, ty) => {
    const p = tileToScreen(tx, ty);
    return { x: cam.x + p.x * cam.zoom, y: cam.y + p.y * cam.zoom };
  };
}

/** Смонтировать рендер в host, связать с контроллером базы; вернуть отписку. Камера опрашивается каждый кадр. */
function useRenderer(app: App, content: Content, onCamera: () => void) {
  const host = useRef<HTMLDivElement>(null);
  const renderer = useRef<PixiRenderer | null>(null);
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const r = new PixiRenderer({ resolveAssetUrl: atlasUrl });
    let disposed = false;
    let unbind: (() => void) | undefined;
    let raf = 0;
    let last = '';
    const poll = (): void => {
      const c = cameraOf(r);
      const key = `${c.x}|${c.y}|${c.zoom}`;
      if (key !== last) {
        last = key;
        onCamera();
      }
      raf = requestAnimationFrame(poll);
    };
    void (async () => {
      await r.init(el);
      if (disposed) return r.destroy();
      unbind = app.base.bindRenderer(r);
      r.centerCamera(1);
      renderer.current = r;
      raf = requestAnimationFrame(poll);
      await r.loadAtlas(artManifest as Record<string, unknown>);
    })();
    return () => {
      disposed = true;
      cancelAnimationFrame(raf);
      unbind?.();
      if (renderer.current === r) {
        renderer.current = null;
        r.destroy();
      }
    };
  }, [app, content]);
  return { host, renderer };
}

/** Сколько показывать отказ сервера (insufficient_resources, conflict, …), мс. Чисто UI-тайминг. */
export const REJECTION_TTL_MS = 6000;

export function BaseScreen(p: { app: App; content: Content; t: T }): VNode {
  const [, setTick] = useState(0);
  const [hidden, setHidden] = useState<unknown>(null);
  const bump = (): void => setTick((n) => n + 1);
  const { host, renderer } = useRenderer(p.app, p.content, bump);
  // Перерисовка: изменения слайса/редактора + секундный тик для таймеров. Свой хук вместо useBaseViewModel
  // (ui/base/hook.ts не проходит tsc в строгом режиме: Dispatch<unknown> → () => void; сообщено модулю base, OQ-A3).
  useEffect(() => {
    const off = p.app.base.subscribe(bump);
    const id = setInterval(bump, 1000);
    return () => {
      off();
      clearInterval(id);
    };
  }, [p.app]);
  // Отказ сервера показываем недолго: слайс хранит последний отказ до следующей команды.
  const rejection = p.app.base.model.store.getState().lastRejection;
  useEffect(() => {
    if (!rejection) return;
    const id = setTimeout(() => setHidden(rejection), REJECTION_TTL_MS);
    return () => clearTimeout(id);
  }, [rejection]);
  const vm0 = p.app.base.viewModel();
  const vm = rejection && rejection === hidden ? { ...vm0, rejection: null } : vm0;
  const hud = selectHud({
    me: p.app.store.getState().me,
    world: p.app.base.model.store.getState().view,
    catalog: p.content.catalog,
    serverNowMs: p.app.serverNowMs(),
    army: p.app.army.viewModel(),
  });
  const project = makeProject(renderer.current ? cameraOf(renderer.current) : { x: 0, y: 0, zoom: 1 });
  return (
    <div data-testid="base-screen" style={{ position: 'relative', width: '100%', height: '100%', overflow: 'hidden' }}>
      <div ref={host} style={{ position: 'absolute', inset: 0 }} />
      {/* Слот редактора базы: компоненты и состояние принадлежат модулю base (client/ui/base). */}
      <div className="base-slot" data-testid="base-slot">
        <BaseEditorView vm={vm} actions={p.app.base.actions} t={p.t} project={project} />
      </div>
      <Hud vm={hud} t={p.t} />
    </div>
  );
}
