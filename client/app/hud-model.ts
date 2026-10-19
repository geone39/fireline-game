// Селектор HUD: /v1/me + слайс базы → то, что рисует <Hud>. Чистая функция, без DOM.
import { remainingSeconds, type BaseWorld, type Catalog } from '../core/base';
import type { Me } from './app';

/** Порядок валют в HUD (три игровых ресурса + премиум-валюта gems). */
export const WALLET_ORDER = ['cash', 'fuel', 'alloy', 'gems'] as const;

export interface HudVM {
  profile: { nickname: string; hqLevel: number; trophies: number } | null;
  wallet: { res: string; amount: number }[];
  builders: { total: number; free: number };
  /** Накопленное в сборщиках, готовое к сбору. */
  collectors: { res: string; amount: number }[];
  timers: { id: string; uid: string; nameKey: string; kind: string; toLevel: number; remainingS: number }[];
}

export interface HudInput {
  me: Me | null;
  world: BaseWorld;
  catalog: Catalog;
  /** Текущее время сервера, мс. */
  serverNowMs: number;
}

export function selectHud(i: HudInput): HudVM {
  const wallets = i.me?.wallets ?? {};
  const extra = Object.keys(wallets)
    .filter((r) => !(WALLET_ORDER as readonly string[]).includes(r))
    .sort();
  const wallet = [...WALLET_ORDER, ...extra].map((res) => ({ res, amount: wallets[res] ?? 0 }));

  // Слоты билдеров берём из /v1/me, но счётчик занятых — из слайса базы: он обновляется оптимистично.
  const total = Math.max(i.world.builders.total, i.me?.builders.length ?? 0);
  const busy = Math.min(total, i.world.builders.busy);

  const collectors = Object.entries(i.world.collectors)
    .filter(([, n]) => n > 0)
    .map(([res, amount]) => ({ res, amount }))
    .sort((a, b) => a.res.localeCompare(b.res));

  const itemType = new Map(i.world.items.map((it) => [it.uid, it.type]));
  const timers = i.world.timers
    .map((t) => {
      const type = itemType.get(t.target_id);
      return {
        id: t.id,
        uid: t.target_id,
        nameKey: (type ? i.catalog.get(type)?.nameKey : undefined) ?? type ?? t.target_id,
        kind: t.kind,
        toLevel: t.to_level,
        remainingS: remainingSeconds(t, i.serverNowMs),
      };
    })
    .sort((a, b) => a.remainingS - b.remainingS);

  const p = i.me?.profile;
  return {
    profile: p ? { nickname: p.nickname, hqLevel: p.hq_level, trophies: p.trophies } : null,
    wallet,
    builders: { total, free: total - busy },
    collectors,
    timers,
  };
}
