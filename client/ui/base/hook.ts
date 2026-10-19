import { useEffect, useReducer } from 'preact/hooks';
import type { BaseController } from '../../core/base/controller';
import type { BaseViewModel } from '../../core/base/view-model';

/**
 * Подписывает компонент на слайс и редактор; возвращает свежий view-model.
 * tickMs — период перерисовки таймеров (оставшееся время идёт без событий стора); 0 — без тика.
 */
export function useBaseViewModel(c: BaseController, tickMs = 1000): BaseViewModel {
  const [, bump] = useReducer((n: number) => n + 1, 0);
  useEffect(() => {
    const off = c.subscribe(bump);
    const id = tickMs > 0 ? setInterval(bump, tickMs) : undefined;
    return () => {
      off();
      if (id !== undefined) clearInterval(id);
    };
  }, [c, tickMs]);
  return c.viewModel();
}
