import type { ApiClient } from '../../core/api';
import type { ArmyApi, ArmyStateDto, LabStateDto } from './types';

type Http = Pick<ApiClient, 'get' | 'post'>;

/** Адаптер ApiClient → ArmyApi (docs/03-api.md, docs/design/army-lab.md §8). */
export function createArmyApi(http: Http): ArmyApi {
  return {
    fetchArmy: () => http.get<ArmyStateDto>('/v1/army'),
    train: (req, idempotencyKey) => http.post<ArmyStateDto>('/v1/army/train', req, { idempotencyKey }),
    cancel: (req, idempotencyKey) => http.post<ArmyStateDto>('/v1/army/cancel', req, { idempotencyKey }),
    speedup: (req, idempotencyKey) => http.post<ArmyStateDto>('/v1/army/speedup', req, { idempotencyKey }),
    fetchLab: () => http.get<LabStateDto>('/v1/lab'),
    research: (req, idempotencyKey) => http.post<LabStateDto>('/v1/lab/research', req, { idempotencyKey }),
    cancelResearch: (idempotencyKey) => http.post<LabStateDto>('/v1/lab/cancel', {}, { idempotencyKey }),
    speedupResearch: (idempotencyKey) => http.post<LabStateDto>('/v1/lab/speedup', {}, { idempotencyKey }),
  };
}
