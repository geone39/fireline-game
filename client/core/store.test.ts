import { describe, expect, it, vi } from 'vitest';
import { ApiError } from './api';
import { createGameStore, type Command } from './store';

type W = { cash: number; log: string[] };
const apply = (w: W, c: Command): W => ({ cash: w.cash - (c.args.cost as number), log: [...w.log, c.op] });
const cmd = (op: string, cost = 10): Command => ({ op, args: { cost } });

function make(send: (c: Command[], k: string) => Promise<W | void>, extra = {}) {
  let n = 0;
  return createGameStore<W>({ initial: { cash: 100, log: [] }, apply, send, newKey: () => `k${++n}`, ...extra });
}

describe('game store', () => {
  it('applies optimistically before the server answers', () => {
    const s = make(() => new Promise(() => {}));
    s.enqueue(cmd('build'));
    expect(s.getState().view.cash).toBe(90);
    expect(s.getState().confirmed.cash).toBe(100);
    expect(s.getState().pending).toHaveLength(1);
  });

  it('commits confirmed state on success', async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    const s = make(send);
    s.enqueue(cmd('build'));
    s.enqueue(cmd('upgrade'));
    await s.flush();
    expect(send).toHaveBeenCalledOnce();
    expect(send.mock.calls[0]![0]).toEqual([cmd('build'), cmd('upgrade')]);
    expect(s.getState().confirmed.cash).toBe(80);
    expect(s.getState().pending).toHaveLength(0);
  });

  it('prefers authoritative server state', async () => {
    const s = make(() => Promise.resolve({ cash: 42, log: ['srv'] }));
    s.enqueue(cmd('build'));
    await s.flush();
    expect(s.getState().view).toEqual({ cash: 42, log: ['srv'] });
  });

  it('rolls back a rejected command', async () => {
    const s = make(() => Promise.reject(new ApiError('insufficient_resources', 409, 'no cash')));
    s.enqueue(cmd('a'));
    expect(s.getState().view.cash).toBe(90);
    await s.flush();
    const st = s.getState();
    expect(st.view).toEqual({ cash: 100, log: [] });
    expect(st.pending).toHaveLength(0);
    expect(st.lastRejection?.code).toBe('insufficient_resources');
  });

  it('clears the previous rejection when a new command is queued', async () => {
    const send = vi.fn().mockRejectedValueOnce(new ApiError('insufficient_resources', 409, 'no cash')).mockResolvedValue(undefined);
    const s = make(send);
    s.enqueue(cmd('a'));
    await s.flush();
    expect(s.getState().lastRejection?.code).toBe('insufficient_resources');
    s.enqueue(cmd('b'));
    expect(s.getState().lastRejection).toBeNull();
    await s.flush();
    expect(s.getState().lastRejection).toBeNull();
  });

  it('rollback of a failed batch re-projects commands queued meanwhile', async () => {
    const send = vi.fn().mockRejectedValueOnce(new ApiError('conflict', 409, 'x')).mockResolvedValue(undefined);
    const s = make(send);
    s.enqueue(cmd('a', 10));
    const p = s.flush();
    s.enqueue(cmd('b', 5));
    await p;
    expect(s.getState().confirmed).toEqual({ cash: 95, log: ['b'] });
    expect(s.getState().view.cash).toBe(95);
  });

  it('keeps queue and idempotency key on network error, resends same key', async () => {
    const send = vi.fn().mockRejectedValueOnce(new ApiError('network', 0, 'down')).mockResolvedValue(undefined);
    const s = make(send);
    s.enqueue(cmd('a'));
    await s.flush();
    expect(s.getState().offline).toBe(true);
    expect(s.getState().pending).toHaveLength(1);
    expect(s.getState().view.cash).toBe(90);
    await s.flush();
    expect(send.mock.calls[0]![1]).toBe(send.mock.calls[1]![1]);
    expect(s.getState().offline).toBe(false);
    expect(s.getState().confirmed.cash).toBe(90);
  });

  it('drops commands older than 5 minutes and rolls them back', async () => {
    let t = 0;
    const send = vi.fn().mockRejectedValue(new ApiError('network', 0, 'down'));
    const s = make(send, { now: () => t });
    s.enqueue(cmd('a'));
    await s.flush();
    t = 5 * 60_000 + 1;
    await s.flush();
    expect(send).toHaveBeenCalledTimes(1);
    expect(s.getState().pending).toHaveLength(0);
    expect(s.getState().view.cash).toBe(100);
    expect(s.getState().lastRejection?.code).toBe('expired');
  });

  it('notifies subscribers until unsubscribed', () => {
    const s = make(() => Promise.resolve());
    const l = vi.fn();
    const off = s.subscribe(l);
    s.enqueue(cmd('a'));
    off();
    s.enqueue(cmd('b'));
    expect(l).toHaveBeenCalledOnce();
  });
});
