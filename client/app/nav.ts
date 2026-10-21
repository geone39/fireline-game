// Навигация шелла: База / Армия / Лаборатория / Бой. Армия и лаборатория — вкладки одного экрана client/ui/army.
import type { Tab } from '../ui/army/store/view-model';

export type Screen = 'base' | 'army' | 'battle';
export type NavId = 'base' | 'army' | 'lab' | 'battle';

export const NAV_ITEMS: readonly NavId[] = ['base', 'army', 'lab', 'battle'];

export interface NavTarget {
  screen: Screen;
  /** Вкладка экрана армии, которую надо выбрать. */
  tab?: Tab;
}

export function navTarget(id: NavId): NavTarget {
  switch (id) {
    case 'army':
      return { screen: 'army', tab: 'army' };
    case 'lab':
      return { screen: 'army', tab: 'lab' };
    default:
      return { screen: id };
  }
}

/** Какая кнопка подсвечена: на экране армии — по выбранной вкладке (её можно переключить и внутри экрана). */
export function activeNav(screen: Screen, tab: Tab): NavId {
  if (screen !== 'army') return screen;
  return tab === 'lab' ? 'lab' : 'army';
}
