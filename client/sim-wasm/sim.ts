// Типизированная обёртка над JSON-API `fireline_sim` (sim/wasm/api.d.ts). Всё общение с ядром — строки JSON;
// uint64 (seed, StateHash) — строки. Отклонённая команда — не исключение: смотрим `ApplyOutcome.rejected`.
import type { Command, Event, FirelineSim, RejectCode, Reply, Result, Snapshot } from './api';

export type { Cell, Command, Event, RejectCode, Result, Snapshot } from './api';

/** Ошибка ядра (`ok:false`): битый Snapshot/Rules, неизвестный handle, невалидный JSON. */
export class SimError extends Error {
  constructor(
    readonly op: string,
    message: string,
  ) {
    super(`sim.${op}: ${message}`);
    this.name = 'SimError';
  }
}

export interface ApplyOutcome {
  /** События для анимации (при отказе — только события неявного EndTurn, если были). */
  events: Event[];
  /** Код отказа ядра или null, если команда принята. */
  rejected: RejectCode | null;
}

export interface SimBattle {
  /** Применить команду. Состояние при отказе не меняется. */
  apply(cmd: Command): ApplyOutcome;
  /** Текущий итог (звёзды, разрушение, EndReason, Rejected, StateHash). */
  result(): Result;
  /** Освободить бой в ядре; после этого вызовы бросают SimError. */
  free(): void;
}

export interface Sim {
  readonly engineVersion: string;
  /** Полный реплей (проверка/прогон журнала целиком). */
  run(snapshot: Snapshot, commands: readonly Command[], seed: string | number): Result;
  /** Пошаговый бой. Snapshot несёт Rules; скрытых ловушек в клиентском виде нет (OQ-7.8). */
  newBattle(snapshot: Snapshot, seed: string | number): SimBattle;
}

function unwrap<T>(op: string, raw: string): T {
  let reply: Reply<T>;
  try {
    reply = JSON.parse(raw) as Reply<T>;
  } catch {
    throw new SimError(op, 'ответ не JSON');
  }
  if (!reply.ok) throw new SimError(op, reply.error);
  return reply.result;
}

export function createSim(api: FirelineSim): Sim {
  return {
    engineVersion: api.engineVersion,
    run: (snapshot, commands, seed) =>
      unwrap<Result>('run', api.run(JSON.stringify(snapshot), JSON.stringify(commands), seed)),
    newBattle(snapshot, seed) {
      const { handle } = unwrap<{ handle: number }>('newBattle', api.newBattle(JSON.stringify(snapshot), seed));
      let sent = 0;
      let freed = false;
      const alive = (op: string): void => {
        if (freed) throw new SimError(op, 'бой уже освобождён');
      };
      const result = (): Result => {
        alive('result');
        return unwrap<Result>('result', api.result(handle));
      };
      return {
        apply(cmd) {
          alive('apply');
          const index = sent++; // Rejection.Index = порядковый номер команды в журнале
          const { events } = unwrap<{ events: Event[] | null }>('apply', api.apply(handle, JSON.stringify(cmd)));
          const rej = result().Rejected?.find((r) => r.Index === index);
          return { events: events ?? [], rejected: rej ? rej.Code : null };
        },
        result,
        free() {
          if (freed) return;
          freed = true;
          unwrap('free', api.free(handle));
        },
      };
    },
  };
}

/** Загрузить wasm (лениво, один раз) и создать обёртку. */
export async function loadSim(sources?: import('./loader').SimSources): Promise<Sim> {
  const { loadFirelineSim } = await import('./loader');
  return createSim(await loadFirelineSim(sources));
}
