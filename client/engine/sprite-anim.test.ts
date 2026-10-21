import { describe, expect, it } from 'vitest';
import blender from '../../art/spike/blender/out/manifest.json';
import procedural from '../../art/spike/procedural/out/manifest.json';
import type { ArtSprite, LegacyAnimMeta } from './atlas';
import {
  buildSpriteArt,
  clipDuration,
  clipFrame,
  frameIndex,
  resolveClip,
  SpriteAnimator,
  type AnimClip,
} from './sprite-anim';

const unit = (m: { sprites: unknown[]; _anims: Record<string, unknown> }) => {
  const def = m.sprites.find((s) => (s as ArtSprite).id === 'u_rifleman') as ArtSprite;
  return buildSpriteArt(def, (m._anims as Record<string, LegacyAnimMeta>)[def.id]);
};

describe('frameIndex', () => {
  it('петля: кадры идут по fps и возвращаются к началу', () => {
    // 10 fps -> кадр каждые 100 мс
    expect([0, 99, 100, 250, 399, 400, 1000].map((t) => frameIndex(4, 10, true, t))).toEqual([0, 0, 1, 2, 3, 0, 2]);
  });
  it('без петли держит последний кадр', () => {
    expect([0, 500, 799, 800, 5000].map((t) => frameIndex(4, 8, false, t))).toEqual([0, 3, 3, 3, 3]);
  });
  it('один кадр, нулевой fps и отрицательное время — кадр 0', () => {
    expect(frameIndex(1, 10, true, 999)).toBe(0);
    expect(frameIndex(4, 0, true, 999)).toBe(0);
    expect(frameIndex(4, 10, true, -5)).toBe(0);
  });
});

describe('buildClips (спайк ART-1)', () => {
  it('Blender: 8 направлений, 4 состояния, fps/loop из _anims', () => {
    const a = unit(blender as never);
    expect([...a.clips.keys()].sort()).toEqual(['attack', 'die', 'idle', 'walk']);
    const walk = a.clips.get('walk') as AnimClip;
    expect(walk).toMatchObject({ dirs: 8, fps: 10, loop: true });
    expect(walk.frames).toHaveLength(8);
    expect(walk.frames.every((l) => l.length === 4)).toBe(true);
    expect(walk.frames[3]?.[2]).toBe('u_rifleman_walk_d3_2');
    expect(a.clips.get('die')).toMatchObject({ loop: false, fps: 8 });
    expect(clipDuration(a.clips.get('die') as AnimClip)).toBe(500);
  });
  it('процедурный: 4 направления', () => {
    const a = unit(procedural as never);
    expect(a.clips.get('idle')?.dirs).toBe(4);
  });
  it('здание из одного кадра статично: клипов нет', () => {
    const def = (blender.sprites as ArtSprite[]).find((s) => s.id === 'b_hq') as ArtSprite;
    expect(buildSpriteArt(def).clips.size).toBe(0);
  });
});

describe('buildClips (другие форматы)', () => {
  const frames = (...names: string[]) => names.map((name, i) => ({ name, x: i * 8, y: 0, w: 8, h: 8 }));
  const base = { atlas: 'a', anchors: { ground: { x: 4, y: 7 } } };

  it('поле anims: плоский список направление-мажорно и списки по направлениям', () => {
    const def: ArtSprite = {
      ...base,
      id: 'u_x',
      frames: frames('a0', 'a1', 'b0', 'b1', 'c0'),
      anims: {
        idle: { fps: 2, dirs: 2, frames: ['a0', 'a1', 'b0', 'b1'] },
        walk: { frames: [['a0'], ['b0', 'b1']], loop: true },
        bad: { frames: ['zzz'] },
      },
    };
    const a = buildSpriteArt(def);
    expect(a.clips.get('idle')?.frames).toEqual([
      ['a0', 'a1'],
      ['b0', 'b1'],
    ]);
    expect(a.clips.get('walk')?.dirs).toBe(2);
    expect(a.clips.has('bad')).toBe(false); // кадров нет в манифесте — клип отброшен
  });

  it('несколько кадров и fps > 0: зацикленный idle без направлений', () => {
    const def: ArtSprite = { ...base, id: 'b_fan', fps: 4, frames: frames('b_fan_0', 'b_fan_1', 'b_fan_2') };
    const clip = buildSpriteArt(def).clips.get('idle') as AnimClip;
    expect(clip).toMatchObject({ dirs: 1, fps: 4, loop: true });
    expect(clipFrame(clip, 5, 260)).toBe('b_fan_1');
  });

  it('fps 0 и несколько кадров (заглушки) — статично', () => {
    const def: ArtSprite = { ...base, id: 'b_s', fps: 0, frames: frames('b_s_0', 'b_s_1') };
    expect(buildSpriteArt(def).clips.size).toBe(0);
  });
});

describe('clipFrame / resolveClip / SpriteAnimator', () => {
  const a = unit(blender as never);

  it('кадр по сектору направления и времени', () => {
    const walk = a.clips.get('walk') as AnimClip;
    expect(clipFrame(walk, 0, 0)).toBe('u_rifleman_walk_d0_0');
    expect(clipFrame(walk, 6, 100)).toBe('u_rifleman_walk_d6_1');
    expect(clipFrame(walk, 6, 400)).toBe('u_rifleman_walk_d6_0');
  });

  it('4 направления: сектор 2 (влево) -> d1', () => {
    const idle = unit(procedural as never).clips.get('idle') as AnimClip;
    expect(clipFrame(idle, 2, 0)).toBe('u_rifleman_idle_d1_0');
  });

  it('нет нужного состояния — запасной idle, затем любой клип', () => {
    expect(resolveClip(a.clips, 'dance')?.name).toBe('idle');
    const only = new Map([['walk', a.clips.get('walk') as AnimClip]]);
    expect(resolveClip(only, 'attack')?.name).toBe('walk');
    expect(resolveClip(new Map(), 'idle')).toBeUndefined();
  });

  it('аниматор: время идёт через advance, смена состояния сбрасывает фазу, поворот — нет', () => {
    const an = new SpriteAnimator(a.clips);
    an.set('walk', 0);
    an.advance(100);
    expect(an.frame()).toBe('u_rifleman_walk_d0_1');
    an.set('walk', 7); // поворот на ходу
    expect(an.frame()).toBe('u_rifleman_walk_d7_1');
    an.set('attack', 7); // новое состояние -> с нуля
    expect(an.frame()).toBe('u_rifleman_attack_d7_0');
    an.advance(125);
    an.set('attack', 7, true); // новый выстрел перезапускает
    expect(an.frame()).toBe('u_rifleman_attack_d7_0');
  });

  it('die не зациклен и сообщает о завершении', () => {
    const an = new SpriteAnimator(a.clips);
    an.set('die', 2, true);
    expect(an.finished).toBe(false);
    an.advance(10_000);
    expect(an.frame()).toBe('u_rifleman_die_d2_3');
    expect(an.finished).toBe(true);
  });

  it('без клипов кадр не определён (рисуется frames[0])', () => {
    const an = new SpriteAnimator(new Map());
    expect(an.hasClips).toBe(false);
    expect(an.frame()).toBeUndefined();
  });
});
