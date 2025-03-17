import fastify, { FastifyPluginCallback } from 'fastify';
import logger from '../util/logger';

export interface PackagerCallbackOptions {
  onFail: (job: any) => void;
  onSuccess: (job: any) => void;
}

export const PackagerCallbackApi: FastifyPluginCallback<
  PackagerCallbackOptions
> = (fastify, opts, next) => {
  fastify.post<{ Body: any }>('/packagerCallback', async (request, reply) => {
    logger.info('Packager callback received');
  });
  next;
};
