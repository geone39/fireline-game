// Форма Rules (sim.Rules, Go-имена полей), которую читает UI: только то, что нужно для подсветки и подписей.
// Числа приходят с Snapshot (их собирает сервер из content/*.yaml, OQ-SIM10); в клиенте они не хардкодятся.
export interface SimRect {
  X: number;
  Y: number;
  W: number;
  H: number;
}

/** sim.UnitClass. */
export const CLASS = { INF: 0, VEH: 1, AIR: 2, SEA: 3 } as const;
/** sim.EntityKind. */
export const KIND = { building: 0, defence: 1, wall: 2, troop: 3, vehicle: 4, ship: 5, hero: 6, decoy: 7 } as const;
/** sim.BlockKind. */
export const BLOCK = { all: 0, none: 1, vehicles: 2 } as const;

export interface UnitDef {
  ID: string;
  Class: number;
  Hero: boolean;
  HP: number;
  HPGrowthPermille: number;
  AP: number;
  MoveCells: number;
  RangeCells: number;
  MinRangeCells: number;
  Dmg: number;
  Burst: number;
  Carries: number;
  Group: number;
  Paradrop: boolean;
  Heal: number;
  HealVs: number;
  JamTurns: number;
  Ability: { ID: string; Kind: number; DecoyUnit: string; RadiusCells: number };
}

export interface BuildingDef {
  ID: string;
  Kind: number;
  W: number;
  H: number;
  HP: number;
  HPGrowthPermille: number;
  Block: number;
  SlowPct: number;
  Hidden: boolean;
  Untargetable: boolean;
  Dmg: number;
  Garrison: number;
  Trap: number;
}

export interface SpellDef {
  ID: string;
  RadiusCells: number;
}

export interface SimRules {
  GridW: number;
  GridH: number;
  BuildZone: SimRect;
  Maps: { ID: string; Water: SimRect[] | null }[];
  MaxTurns: number;
  ParadropEdgeCells: number;
  Spells: SpellDef[] | null;
  Units: UnitDef[];
  Buildings: BuildingDef[];
}

/** max_hp(L) = floor(hp * M[L] / 1000), M[1] = 1000, M[n] = floor(M[n-1] * g / 1000) (docs/design/battle-sim.md §3.2). */
export function scaledHp(hp: number, growthPermille: number, level: number): number {
  const g = growthPermille > 0 ? growthPermille : 1000;
  let m = 1000;
  for (let l = 2; l <= level; l++) m = Math.floor((m * g) / 1000);
  return Math.floor((hp * m) / 1000);
}

export const classBit = (c: number): number => 1 << c;
