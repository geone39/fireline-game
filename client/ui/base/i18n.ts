// Локализация UI базы (ru/en). Названия зданий — через name_key из content/strings/<lang>/names.yaml;
// подписи интерфейса и причины отказов — этот словарь (в content/strings их пока нет, см. OQ-B10).
export type Lang = 'ru' | 'en';

const UI: Record<Lang, Record<string, string>> = {
  ru: {
    'ui.shop': 'Постройки',
    'ui.edit': 'Редактировать',
    'ui.done': 'Готово',
    'ui.undo': 'Отменить',
    'ui.redo': 'Вернуть',
    'ui.save': 'Сохранить',
    'ui.cancel': 'Сбросить',
    'ui.saving': 'Сохранение…',
    'ui.collect': 'Собрать',
    'ui.builders': 'Строители',
    'ui.level': 'Уровень',
    'ui.upgrade': 'Улучшить',
    'ui.speedup': 'Ускорить',
    'ui.remove': 'Снести',
    'ui.remove_confirm': 'Снести?',
    'ui.clear': 'Очистить',
    'ui.rotate': 'Повернуть',
    'ui.place': 'Построить',
    'ui.close': 'Отмена',
    'ui.building': 'Строится',
    'ui.upgrading': 'Улучшается',
    'ui.left': 'осталось',
    'ui.reward': 'Награда',
    'ui.count': 'Построено',
    'ui.gems': 'гемов',
    'res.cash': 'Деньги',
    'res.fuel': 'Топливо',
    'res.alloy': 'Сплав',
    'res.gems': 'Гемы',
    'time.d': 'д',
    'time.h': 'ч',
    'time.m': 'м',
    'time.s': 'с',
    'block.locked': 'Нужен штаб {n}',
    'block.limit': 'Лимит достигнут',
    'block.no_builder': 'Нет свободных строителей',
    'block.no_price': 'Цена не задана',
    'block.busy': 'Уже строится',
    'block.max_level': 'Максимальный уровень',
    'reason.out_of_bounds': 'За пределами карты',
    'reason.on_water': 'Нельзя строить на воде',
    'reason.overlap': 'Клетка занята',
    'reason.needs_water': 'Нужен берег',
    'reason.locked': 'Не открыто на этом уровне штаба',
    'reason.limit': 'Лимит по уровню штаба',
    'reason.not_movable': 'Нельзя переместить',
    'reason.no_hq': 'Нет штаба',
    'reason.bad_rot': 'Неверный поворот',
    'reason.unknown_type': 'Неизвестный тип',
    'reason.offline': 'Нет связи, повторите позже',
    'reason.insufficient_resources': 'Не хватает ресурсов',
    'reason.conflict': 'Конфликт: действие недоступно',
    'reason.invalid': 'Сервер отклонил раскладку',
    'reason.expired': 'Команда устарела',
    'reason.internal': 'Ошибка, повторите позже',
    'legend.range': 'Зона огня',
    'legend.dead': 'Мёртвая зона',
    'legend.aura': 'Радиус действия',
  },
  en: {
    'ui.shop': 'Build',
    'ui.edit': 'Edit',
    'ui.done': 'Done',
    'ui.undo': 'Undo',
    'ui.redo': 'Redo',
    'ui.save': 'Save',
    'ui.cancel': 'Discard',
    'ui.saving': 'Saving…',
    'ui.collect': 'Collect',
    'ui.builders': 'Builders',
    'ui.level': 'Level',
    'ui.upgrade': 'Upgrade',
    'ui.speedup': 'Speed up',
    'ui.remove': 'Demolish',
    'ui.remove_confirm': 'Demolish?',
    'ui.clear': 'Clear',
    'ui.rotate': 'Rotate',
    'ui.place': 'Build',
    'ui.close': 'Cancel',
    'ui.building': 'Under construction',
    'ui.upgrading': 'Upgrading',
    'ui.left': 'left',
    'ui.reward': 'Reward',
    'ui.count': 'Built',
    'ui.gems': 'gems',
    'res.cash': 'Cash',
    'res.fuel': 'Fuel',
    'res.alloy': 'Alloy',
    'res.gems': 'Gems',
    'time.d': 'd',
    'time.h': 'h',
    'time.m': 'm',
    'time.s': 's',
    'block.locked': 'Needs HQ {n}',
    'block.limit': 'Limit reached',
    'block.no_builder': 'No free builders',
    'block.no_price': 'No price set',
    'block.busy': 'Already in progress',
    'block.max_level': 'Max level',
    'reason.out_of_bounds': 'Outside the map',
    'reason.on_water': 'Cannot build on water',
    'reason.overlap': 'Tile is occupied',
    'reason.needs_water': 'Must be on the shore',
    'reason.locked': 'Not unlocked at this HQ level',
    'reason.limit': 'HQ limit reached',
    'reason.not_movable': 'Cannot be moved',
    'reason.no_hq': 'No HQ',
    'reason.bad_rot': 'Bad rotation',
    'reason.unknown_type': 'Unknown type',
    'reason.offline': 'Offline, try again later',
    'reason.insufficient_resources': 'Not enough resources',
    'reason.conflict': 'Conflict: action unavailable',
    'reason.invalid': 'Server rejected the layout',
    'reason.expired': 'Command expired',
    'reason.internal': 'Something went wrong, try later',
    'legend.range': 'Firing range',
    'legend.dead': 'Dead zone',
    'legend.aura': 'Effect radius',
  },
};

export type T = (key: string, vars?: Record<string, string | number>) => string;

/** names — content/strings/<lang>/names.yaml (name_key → текст); неизвестный ключ показывается как есть. */
export function createT(lang: Lang, names: Readonly<Record<string, string>>): T {
  const ui = UI[lang];
  return (key, vars) => {
    let s = ui[key] ?? names[key] ?? key;
    if (vars) for (const [k, v] of Object.entries(vars)) s = s.replace(`{${k}}`, String(v));
    return s;
  };
}

export const UI_KEYS: Record<Lang, readonly string[]> = { ru: Object.keys(UI.ru), en: Object.keys(UI.en) };

/** 3725 → «1 ч 2 м»: две старшие ненулевые единицы. */
export function formatDuration(totalS: number, t: T): string {
  const units: [number, string][] = [
    [86400, 'time.d'],
    [3600, 'time.h'],
    [60, 'time.m'],
    [1, 'time.s'],
  ];
  const parts: string[] = [];
  let left = Math.max(0, Math.floor(totalS));
  for (const [size, key] of units) {
    const n = Math.floor(left / size);
    left -= n * size;
    if (n > 0 && parts.length < 2) parts.push(`${n} ${t(key)}`);
  }
  return parts.length ? parts.join(' ') : `0 ${t('time.s')}`;
}

export function formatCost(cost: Readonly<Record<string, number>>, t: T): string {
  return Object.entries(cost)
    .map(([r, n]) => `${n} ${t(`res.${r}`) === `res.${r}` ? r : t(`res.${r}`)}`)
    .join(', ');
}
