import { UnsupportedError, type PlatformAdapter } from './adapter';

/**
 * Заглушка адаптера Telegram (P2).
 * TODO(platform-rules): правила Telegram (подпись запуска, платежи, реклама, шаринг) —
 * свести в docs/platform-notes.md и реализовать; до тех пор все вызовы бросают UnsupportedError.
 */
export function createTelegramAdapter(): PlatformAdapter {
  const no = (what: string) => (): never => {
    throw new UnsupportedError(`telegram: ${what}`);
  };
  return {
    id: () => 'telegram',
    signedAuth: no('signedAuth'),
    purchase: no('purchase'),
    share: no('share'),
    storage: { get: no('storage.get'), set: no('storage.set'), remove: no('storage.remove') },
    lifecycle: { on: no('lifecycle.on') },
    adsPolicy: { rewardedAllowed: false, interstitialAllowed: false },
  };
}
