// PlatformAdapter — контракт modules/client-core/CONTRACT.md.

export interface AuthPayload {
  /** Провайдер: guest | vk | telegram | ... */
  provider: string;
  /** Подписанные данные платформы (для guest — device_id). */
  payload: Record<string, string>;
}

export interface Receipt {
  sku: string;
  provider: string;
  token: string;
}

export interface SharePayload {
  text: string;
  url?: string;
}

export interface KV {
  get(key: string): Promise<string | null>;
  set(key: string, value: string): Promise<void>;
  remove(key: string): Promise<void>;
}

export type LifecycleEvent = 'visible' | 'hidden' | 'online' | 'offline';

export interface Emitter<E extends string> {
  on(event: E, handler: () => void): () => void;
}

export interface AdsPolicy {
  rewardedAllowed: boolean;
  interstitialAllowed: boolean;
}

export interface PlatformAdapter {
  id(): string;
  signedAuth(): Promise<AuthPayload>;
  purchase(sku: string): Promise<Receipt>;
  share(payload: SharePayload): Promise<void>;
  storage: KV;
  lifecycle: Emitter<LifecycleEvent>;
  adsPolicy: AdsPolicy;
}

export class UnsupportedError extends Error {
  constructor(what: string) {
    super(`not supported: ${what}`);
    this.name = 'UnsupportedError';
  }
}
