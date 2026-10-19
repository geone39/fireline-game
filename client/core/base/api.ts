import type { ApiClient } from '../api';
import type { BaseApi, BaseItem, BaseStateDto } from './types';

type Http = Pick<ApiClient, 'get' | 'post' | 'put'>;

/** Адаптер ApiClient → BaseApi (docs/03-api.md: /v1/base, /v1/base/commands, PUT /v1/base/layout). */
export function createBaseApi(http: Http): BaseApi {
  return {
    fetchBase: () => http.get<BaseStateDto>('/v1/base'),
    sendCommands: (commands, idempotencyKey) => http.post<BaseStateDto>('/v1/base/commands', commands, { idempotencyKey }),
    saveLayout: (items: readonly BaseItem[]) => http.put<BaseStateDto>('/v1/base/layout', { layout: { items } }),
  };
}
