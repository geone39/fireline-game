// Шелл приложения: жизненный цикл (вход → загрузка → экран базы), ошибки и content_outdated.
import type { VNode } from 'preact';
import { Suspense, lazy } from 'preact/compat';
import { useEffect, useMemo, useState } from 'preact/hooks';
import type { PlatformAdapter } from '../platform/adapter';
import type { App, AppState } from './app';
import { createApp } from './app';
import { BaseScreen } from './BaseScreen';
const BattleScreen = lazy(() => import('./BattleScreen').then((m) => ({ default: m.BattleScreen })));
import { detectLang, loadContent, type Lang } from './content';
import { createAppT } from './i18n';
import { SHELL_CSS } from './styles';

export interface ShellProps {
  platform: PlatformAdapter;
  baseUrl?: string;
  contentVersion?: string;
  lang?: Lang;
  map?: string;
  /** Стартовый экран: `battle` — локальный тестовый бой без сервера. */
  screen?: 'base' | 'battle';
}

function useAppState(app: App): AppState {
  const [s, set] = useState(app.store.getState());
  useEffect(() => app.store.subscribe(set), [app]);
  return s;
}

function Center(p: { children: VNode | VNode[] }): VNode {
  return (
    <div class="app-center" data-testid="app-center">
      {p.children}
    </div>
  );
}

export function Shell(p: ShellProps): VNode {
  const lang = p.lang ?? detectLang(typeof navigator === 'undefined' ? undefined : navigator.language);
  const content = useMemo(() => loadContent(p.map), [p.map]);
  const t = useMemo(() => createAppT(lang, content.names[lang]), [lang, content]);
  const app = useMemo(
    () => createApp({ platform: p.platform, baseUrl: p.baseUrl ?? '', contentVersion: p.contentVersion, content }),
    [p.platform, p.baseUrl, p.contentVersion, content],
  );
  useEffect(() => {
    if (p.screen === 'battle') return; // локальный бой не требует сервера
    void app.start();
    return () => app.dispose();
  }, [app]);
  const state = useAppState(app);
  const [screen, setScreen] = useState<'base' | 'battle'>(p.screen ?? 'base');

  let body: VNode;
  if (screen === 'battle') {
    // Выход: из автономного боя — перезагрузка без ?screen=battle, из шелла — назад на базу.
    const exit = (): void => (p.screen === 'battle' ? location.assign(location.pathname) : setScreen('base'));
    return (
      <>
        <style>{SHELL_CSS}</style>
        <Suspense fallback={<Center><p>{t('battle.loading')}</p></Center>}>
          <BattleScreen t={t} onExit={exit} />
        </Suspense>
      </>
    );
  }
  switch (state.phase) {
    case 'ready':
      body = (
        <>
          <BaseScreen app={app} content={content} t={t} />
          <button type="button" class="battle-open" data-testid="battle-open" onClick={() => setScreen('battle')}>
            {t('battle.open')}
          </button>
        </>
      );
      break;
    case 'outdated':
      body = (
        <Center>
          <p data-testid="outdated">{t('app.outdated')}</p>
          <button type="button" onClick={() => location.reload()}>
            {t('app.reload')}
          </button>
        </Center>
      );
      break;
    case 'error':
      body = (
        <Center>
          <p data-testid="error">
            {t(state.error?.code === 'network' ? 'app.offline' : 'app.error')}
            <br />
            <small>{state.error?.message}</small>
          </p>
          <button type="button" onClick={() => void app.start()}>
            {t('app.retry')}
          </button>
        </Center>
      );
      break;
    default:
      body = (
        <Center>
          <p data-testid="loading">{t('app.loading')}</p>
        </Center>
      );
  }
  return (
    <>
      <style>{SHELL_CSS}</style>
      {body}
    </>
  );
}
