// Строки шелла и HUD (ru/en). Названия зданий и ресурсов — через name_key (content/strings) и словарь UI базы.
// Новых ключей в content/strings пока нет: запрос content — docs/open-questions.md (OQ-A1).
import { createT, type T } from '../ui/base/i18n';
import type { Lang } from './content';

export const APP_STRINGS: Record<Lang, Record<string, string>> = {
  ru: {
    'app.loading': 'Загрузка…',
    'app.signing_in': 'Входим…',
    'app.retry': 'Повторить',
    'app.reload': 'Обновить',
    'app.error': 'Не удалось подключиться к серверу',
    'app.outdated': 'Вышла новая версия игры. Обновите страницу.',
    'app.offline': 'Нет связи с сервером',
    'hud.builders': 'Строители',
    'hud.free': 'свободно',
    'hud.collectors': 'Накоплено',
    'hud.collectors_empty': 'Нечего собирать',
    'hud.timers': 'Стройки',
    'hud.timers_empty': 'Строек нет',
    'hud.upgrade_to': 'до ур. {n}',
    'hud.trophies': 'Кубки',
    'hud.hq': 'Штаб {n}',
    'hud.ready': 'готово',
    'reason.not_found': 'База не найдена или объект уже удалён',
    'reason.unauthorized': 'Сессия истекла, войдите заново',
    'reason.rate_limited': 'Слишком часто, подождите немного',
    'reason.shielded': 'Действие недоступно под щитом',
    'reason.cooldown': 'Подождите, действие на перезарядке',
  },
  en: {
    'app.loading': 'Loading…',
    'app.signing_in': 'Signing in…',
    'app.retry': 'Retry',
    'app.reload': 'Reload',
    'app.error': 'Could not reach the server',
    'app.outdated': 'A new version of the game is available. Please reload.',
    'app.offline': 'No connection to the server',
    'hud.builders': 'Builders',
    'hud.free': 'free',
    'hud.collectors': 'Accrued',
    'hud.collectors_empty': 'Nothing to collect',
    'hud.timers': 'Construction',
    'hud.timers_empty': 'No construction',
    'hud.upgrade_to': 'to lvl {n}',
    'hud.trophies': 'Trophies',
    'hud.hq': 'HQ {n}',
    'hud.ready': 'ready',
    'reason.not_found': 'Base or object not found',
    'reason.unauthorized': 'Session expired, please sign in again',
    'reason.rate_limited': 'Too many requests, wait a moment',
    'reason.shielded': 'Not available while shielded',
    'reason.cooldown': 'On cooldown, try again later',
  },
};

export function createAppT(lang: Lang, names: Readonly<Record<string, string>>): T {
  const base = createT(lang, names);
  const own = APP_STRINGS[lang];
  return (key, vars) => {
    const s = own[key];
    if (s === undefined) return base(key, vars);
    return vars ? Object.entries(vars).reduce((acc, [k, v]) => acc.replace(`{${k}}`, String(v)), s) : s;
  };
}
