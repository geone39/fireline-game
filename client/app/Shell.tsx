// Шелл приложения: жизненный цикл (вход → загрузка → экраны База / Армия / Лаборатория / Бой), ошибки и content_outdated.
import type { VNode } from 'preact';
import { Suspense, lazy } from 'preact/compat';
import { useEffect, useMemo, useState } from 'preact/hooks';
import type { PlatformAdapter } from '../platform/adapter';
import type { Snapshot } from '../sim-wasm/api';
import { useArmyViewModel } from '../ui/army/hook';
import type { T } from '../ui/base/i18n';
import type { App, AppState } from './app';
import { createApp } from './app';
import { ArmyScreen } from './ArmyScreen';
import { BaseScreen } from './BaseScreen';
import { testSnapshot } from './battle-fixture';
import { armySnapshot, prepEntries, type Selection } from './battle-prep';
import { BattlePrep } from './BattlePrep';
const BattleScreen = lazy(() => import('./BattleScreen').then((m) => ({ default: m.BattleScreen })));
import { detectLang, loadContent, type Content, type Lang } from './content';
import { Hud } from './hud';
import { selectHud } from './hud-model';
import { createAppT } from './i18n';
import { activeNav, NAV_ITEMS, navTarget, type NavId, type Screen } from './nav';
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

/** Кнопки разделов. Подсвечена текущая; экран армии переключает вкладку контроллера армии. */
export function NavBar(p: { active: NavId; t: T; onSelect: (id: NavId) => void; floating: boolean }): VNode {
  return (
    <nav class={p.floating ? 'app-nav' : 'app-nav on-screen'} data-testid="nav" aria-label={p.t('nav.label')}>
      {NAV_ITEMS.map((id) => (
        <button key={id} type="button" data-testid={`nav-${id}`} aria-current={p.active === id ? 'page' : undefined} onClick={() => p.onSelect(id)}>
          {p.t(`nav.${id}`)}
        </button>
      ))}
    </nav>
  );
}

/** HUD поверх экранов армии и подготовки к бою; перерисовывается раз в секунду (таймеры). */
function ArmyHud(p: { app: App; content: Content; t: T }): VNode {
  const [, setTick] = useState(0);
  useEffect(() => {
    const off = p.app.army.subscribe(() => setTick((n) => n + 1));
    const id = setInterval(() => setTick((n) => n + 1), 1000);
    return () => {
      off();
      clearInterval(id);
    };
  }, [p.app]);
  const hud = selectHud({
    me: p.app.store.getState().me,
    world: p.app.base.model.store.getState().view,
    catalog: p.content.catalog,
    serverNowMs: p.app.serverNowMs(),
    army: p.app.army.viewModel(),
  });
  return <Hud vm={hud} t={p.t} />;
}

/** Выбор состава: запас берётся из контроллера армии (обновляется при открытии экрана). */
function BattlePrepScreen(p: { app: App; t: T; onStart: (s: Snapshot) => void; onTest: () => void; onToArmy: () => void }): VNode {
  const vm = useArmyViewModel(p.app.army);
  const target = useMemo(() => testSnapshot(), []);
  const entries = useMemo(() => prepEntries(vm.stock, target), [vm.stock, target]);
  return (
    <BattlePrep
      entries={entries}
      loaded={vm.loaded}
      t={p.t}
      onStart={(sel: Selection) => p.onStart(armySnapshot(target, entries, sel))}
      onTest={p.onTest}
      onToArmy={p.onToArmy}
    />
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
  const standalone = p.screen === 'battle';
  const [screen, setScreen] = useState<Screen>(p.screen ?? 'base');
  /** Идущий бой: snapshot не задан — тестовый (автономный режим и кнопка «Тестовый состав»). */
  const [match, setMatch] = useState<{ snapshot?: Snapshot } | null>(standalone ? {} : null);

  // Вкладку армии/лаборатории можно сменить и внутри экрана: подсветка раздела следует за контроллером.
  const [, setTabTick] = useState(0);
  useEffect(() => app.army.store.subscribe((s, prev) => s.ui.tab !== prev.ui.tab && setTabTick((n) => n + 1)), [app]);

  const select = (id: NavId): void => {
    const target = navTarget(id);
    if (target.tab) app.army.actions.selectTab(target.tab);
    setScreen(target.screen);
  };

  if (screen === 'battle' && match) {
    // Выход: из автономного боя — перезагрузка без ?screen=battle, из шелла — назад на базу.
    const exit = (): void => {
      if (standalone) return location.assign(location.pathname);
      setMatch(null);
      setScreen('base');
    };
    return (
      <>
        <style>{SHELL_CSS}</style>
        <Suspense fallback={<Center><p>{t('battle.loading')}</p></Center>}>
          <BattleScreen t={t} onExit={exit} snapshot={match.snapshot} />
        </Suspense>
      </>
    );
  }
  let body: VNode;
  switch (state.phase) {
    case 'ready': {
      const tab = app.army.store.getState().ui.tab;
      body = (
        <>
          {screen === 'base' && <BaseScreen app={app} content={content} t={t} />}
          {screen === 'army' && (
            <>
              <ArmyScreen app={app} lang={lang} names={content.names[lang]} />
              <ArmyHud app={app} content={content} t={t} />
            </>
          )}
          {screen === 'battle' && (
            <>
              <BattlePrepScreen app={app} t={t} onStart={(snapshot) => setMatch({ snapshot })} onTest={() => setMatch({})} onToArmy={() => select('army')} />
              <ArmyHud app={app} content={content} t={t} />
            </>
          )}
          <NavBar active={activeNav(screen, tab)} t={t} onSelect={select} floating={screen === 'base'} />
        </>
      );
      break;
    }
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
