// Экран «Армия / Лаборатория»: монтирует ArmyLabApp (client/ui/army) на контроллере шелла (OQ-UIARMY-1).
import type { VNode } from 'preact';
import { useMemo } from 'preact/hooks';
import artManifest from '../../art/manifest.json';
import { ArmyLabApp } from '../ui/army/app';
import { useArmyViewModel } from '../ui/army/hook';
import { createArmyT } from '../ui/army/i18n';
import { makeIconResolver } from '../ui/army/icons';
import type { App } from './app';
import type { Lang } from './content';

const atlasUrls = import.meta.glob('../../art/atlases/*.png', { eager: true, query: '?url', import: 'default' });
const atlasUrl = (img: string): string => (atlasUrls[`../../art/${img}`] as string | undefined) ?? img;
const icon = makeIconResolver(artManifest, atlasUrl);

export function ArmyScreen(p: { app: App; lang: Lang; names: Readonly<Record<string, string>> }): VNode {
  const t = useMemo(() => createArmyT(p.lang, p.names), [p.lang, p.names]);
  const vm = useArmyViewModel(p.app.army);
  return (
    <div class="app-screen" data-testid="army-screen">
      <ArmyLabApp vm={vm} actions={p.app.army.actions} t={t} icon={icon} />
    </div>
  );
}
