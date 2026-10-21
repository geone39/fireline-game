// Клиентская копия контента (content/*.yaml) для подписей и подсветки. Авторитетно всё считает сервер.
import baseGridYaml from '../../content/base_grid.yaml?raw';
import buildingsYaml from '../../content/buildings.yaml?raw';
import armyYaml from '../../content/army.yaml?raw';
import costsYaml from '../../content/building_costs.yaml?raw';
import defencesYaml from '../../content/defences.yaml?raw';
import enArmy from '../../content/strings/en/army.yaml?raw';
import enNames from '../../content/strings/en/names.yaml?raw';
import researchYaml from '../../content/research.yaml?raw';
import ruArmy from '../../content/strings/ru/army.yaml?raw';
import ruNames from '../../content/strings/ru/names.yaml?raw';
import troopsYaml from '../../content/troops.yaml?raw';
import { parseCatalog, parseGrid, parsePricing, parseStrings, type Catalog, type Grid, type Pricing } from '../core/base';
import { parseArmyCatalog, type ArmyCatalog } from '../ui/army/content';

export type Lang = 'ru' | 'en';

export interface Content {
  catalog: Catalog;
  pricing: Pricing;
  /** Сетка выбранной карты (`?map=` или default_map). */
  grid: Grid;
  /** Юниты, заклинания, исследования лаборатории (client/ui/army). */
  army: ArmyCatalog;
  /** Подписи names.yaml + army.yaml (названия юнитов, узлов лаборатории, reason.*). */
  names: Record<Lang, Record<string, string>>;
}

export function loadContent(map?: string): Content {
  return {
    catalog: parseCatalog(buildingsYaml, defencesYaml),
    pricing: parsePricing(costsYaml),
    grid: parseGrid(baseGridYaml, map),
    army: parseArmyCatalog(troopsYaml, researchYaml, armyYaml, costsYaml),
    names: { ru: { ...parseStrings(ruNames), ...parseStrings(ruArmy) }, en: { ...parseStrings(enNames), ...parseStrings(enArmy) } },
  };
}

/** Язык интерфейса: ru, если браузер говорит по-русски, иначе en. */
export function detectLang(navLang: string | undefined): Lang {
  return navLang?.toLowerCase().startsWith('ru') ? 'ru' : 'en';
}
