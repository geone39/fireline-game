import { UnsupportedError, type PlatformAdapter } from './adapter';

/**
 * Заглушка адаптера VK (P2).
 * TODO(platform-rules): правила VK (подпись запуска, платежи, реклама, шаринг) —
 * свести в docs/platform-notes.md и реализовать; до тех пор все вызовы бросают UnsupportedError.
 */
export function createVKAdapter(): PlatformAdapter {
  const no = (what: string) => (): never => {
    throw new UnsupportedError(`vk: ${what}`);
  };
  return {
    id: () => 'vk',
    signedAuth: no('signedAuth'),
    purchase: no('purchase'),
    share: no('share'),
    storage: { get: no('storage.get'), set: no('storage.set'), remove: no('storage.remove') },
    lifecycle: { on: no('lifecycle.on') },
    adsPolicy: { rewardedAllowed: false, interstitialAllowed: false },
  };
}
