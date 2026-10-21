import { useEffect, useState } from 'preact/hooks';
import type { ArmyController } from './store/controller';
import type { ArmyViewModel } from './store/view-model';

/**
 * Подписывает компонент на контроллер армии; возвращает свежий view-model.
 * Раз в tickMs перерисовывает (таймеры идут без событий стора) и вызывает checkDue: дозревшее по finish_at подтягивается с сервера.
 * При монтировании запрашивает состояние (GET /v1/army, /v1/lab).
 */
export function useArmyViewModel(c: ArmyController, tickMs = 1000): ArmyViewModel {
  const [, setTick] = useState(0);
  const bump = (): void => setTick((n) => n + 1);
  useEffect(() => {
    const off = c.subscribe(bump);
    void c.actions.refresh();
    const id =
      tickMs > 0
        ? setInterval(() => {
            c.checkDue();
            bump();
          }, tickMs)
        : undefined;
    return () => {
      off();
      if (id !== undefined) clearInterval(id);
    };
  }, [c, tickMs]);
  return c.viewModel();
}
