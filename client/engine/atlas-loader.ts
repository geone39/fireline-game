import { atlasCategory, type ArtAtlasEntry } from './atlas';
import { ART_SCREEN_CATEGORIES } from './config';

export type ArtScreen = keyof typeof ART_SCREEN_CATEGORIES;

export interface AtlasLoaderDeps<T> {
  /** Путь из манифеста -> URL. */
  resolve(path: string): string;
  load(url: string): Promise<T>;
  unload(texture: T, url: string): void;
  supportsWebp(): boolean | Promise<boolean>;
  /** Атлас выгружен: кэши кадров этого атласа надо сбросить. */
  onUnload?(name: string): void;
  /** Атлас не загрузился ни с одного адреса (рендер остаётся на заглушках). */
  onError?(name: string, error: unknown): void;
}

/** Адреса атласа по приоритету: WebP (если поддерживается), затем PNG. Явный `.webp` в `image` тоже получает PNG-запас. */
export function atlasCandidates(entry: Pick<ArtAtlasEntry, 'image' | 'webp'>, webpSupported: boolean): string[] {
  const out: string[] = [];
  const add = (p: string | undefined) => {
    if (p && !out.includes(p)) out.push(p);
  };
  const png = entry.image.replace(/\.webp$/i, '.png');
  if (webpSupported) {
    add(entry.webp);
    if (/\.webp$/i.test(entry.image)) add(entry.image);
    add(png);
  } else {
    add(png);
    add(entry.image);
  }
  return out;
}

export function screenWants(screen: ArtScreen, category: string): boolean {
  const list: readonly string[] = ART_SCREEN_CATEGORIES[screen];
  return list.includes('*') || list.includes(category);
}

interface Loaded<T> {
  texture: T;
  url: string;
}

/**
 * Загрузчик атласов: несколько атласов по категориям, ленивая загрузка по экранам, кэш и освобождение.
 * Атлас живёт, пока его держит хотя бы один экран (`acquire`/`release`). Без Pixi: текстуры — параметр `T`.
 */
export class AtlasLoader<T> {
  private entries = new Map<string, ArtAtlasEntry>();
  private loaded = new Map<string, Loaded<T>>();
  private pending = new Map<string, Promise<boolean>>();
  private holders = new Map<string, Set<ArtScreen>>();
  private generation = 0;
  private webp: boolean | undefined;

  constructor(private readonly deps: AtlasLoaderDeps<T>) {}

  /** Новый набор атласов. Всё прежнее выгружается; экраны надо взять заново (`acquire`). */
  setManifest(atlases: Record<string, ArtAtlasEntry>): void {
    this.generation++;
    for (const name of [...this.loaded.keys()]) this.drop(name);
    this.holders.clear();
    this.pending.clear(); // загрузки прежнего манифеста завершатся впустую (проверка поколения)
    this.entries = new Map(Object.entries(atlases));
  }

  /** Имена атласов экрана. */
  namesFor(screen: ArtScreen): string[] {
    return [...this.entries].filter(([n, e]) => screenWants(screen, atlasCategory(n, e))).map(([n]) => n);
  }

  get(name: string): T | undefined {
    return this.loaded.get(name)?.texture;
  }

  has(name: string): boolean {
    return this.loaded.has(name);
  }

  get loadedNames(): string[] {
    return [...this.loaded.keys()];
  }

  /** Загружает (если ещё не загружены) и удерживает атласы экрана. Возвращает имена загруженных сейчас. */
  async acquire(screen: ArtScreen): Promise<string[]> {
    const names = this.namesFor(screen);
    for (const n of names) {
      let h = this.holders.get(n);
      if (!h) this.holders.set(n, (h = new Set()));
      h.add(screen);
    }
    const fresh: string[] = [];
    await Promise.all(
      names.map(async (n) => {
        if (this.loaded.has(n)) return;
        if (await this.fetch(n)) fresh.push(n);
      }),
    );
    return fresh;
  }

  /** Отпускает экран; атласы без других держателей выгружаются. Возвращает имена выгруженных. */
  release(screen: ArtScreen): string[] {
    const gone: string[] = [];
    for (const [n, h] of this.holders) {
      if (!h.delete(screen) || h.size > 0) continue;
      this.holders.delete(n);
      if (this.loaded.has(n)) {
        this.drop(n);
        gone.push(n);
      }
    }
    return gone;
  }

  dispose(): void {
    this.generation++;
    for (const name of [...this.loaded.keys()]) this.drop(name);
    this.holders.clear();
    this.entries.clear();
  }

  private drop(name: string): void {
    const l = this.loaded.get(name);
    if (!l) return;
    this.loaded.delete(name);
    this.deps.onUnload?.(name);
    this.deps.unload(l.texture, l.url);
  }

  private fetch(name: string): Promise<boolean> {
    const inflight = this.pending.get(name);
    if (inflight) return inflight;
    const p: Promise<boolean> = this.fetchNow(name).finally(() => {
      if (this.pending.get(name) === p) this.pending.delete(name);
    });
    this.pending.set(name, p);
    return p;
  }

  private async fetchNow(name: string): Promise<boolean> {
    const entry = this.entries.get(name);
    if (!entry) return false;
    const gen = this.generation;
    this.webp ??= await this.deps.supportsWebp();
    let last: unknown;
    for (const path of atlasCandidates(entry, this.webp)) {
      const url = this.deps.resolve(path);
      try {
        const texture = await this.deps.load(url);
        if (gen !== this.generation || !this.holders.has(name)) {
          // Манифест сменили или экран ушёл, пока грузилось: результат не нужен.
          this.deps.unload(texture, url);
          return false;
        }
        this.loaded.set(name, { texture, url });
        return true;
      } catch (err) {
        last = err;
      }
    }
    this.deps.onError?.(name, last);
    return false;
  }
}
