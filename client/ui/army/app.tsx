// Корневой компонент «Армия + Лаборатория» с вкладками: то, что шелл монтирует в слот (OQ-UIARMY-1).
import type { VNode } from 'preact';
import { LabScreen } from '../lab/components';
import { ArmyScreen } from './components';
import type { T } from './i18n';
import type { IconResolver } from './icons';
import { ArmyStyles } from './shared';
import type { ArmyActions } from './store/controller';
import type { ArmyViewModel, Tab } from './store/view-model';

const TABS: { id: Tab; labelKey: string }[] = [
  { id: 'army', labelKey: 'army.title' },
  { id: 'lab', labelKey: 'lab.title' },
];

export function ArmyLabApp(p: { vm: ArmyViewModel; actions: ArmyActions; t: T; icon?: IconResolver }): VNode {
  const { vm, t } = p;
  return (
    <div data-testid="army-lab-app">
      <ArmyStyles />
      <div class="fl-army">
        <div role="tablist" aria-label={t('ui.army.tabs')}>
          {TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              id={`tab-${tab.id}`}
              data-testid={`tab-${tab.id}`}
              aria-selected={vm.tab === tab.id}
              aria-controls={`panel-${tab.id}`}
              onClick={() => p.actions.selectTab(tab.id)}
            >
              {t(tab.labelKey)}
            </button>
          ))}
        </div>
      </div>
      <div role="tabpanel" id={`panel-${vm.tab}`} aria-labelledby={`tab-${vm.tab}`}>
        {vm.tab === 'army' ? <ArmyScreen vm={vm} actions={p.actions} t={t} icon={p.icon} /> : <LabScreen vm={vm} actions={p.actions} t={t} icon={p.icon} />}
      </div>
    </div>
  );
}
