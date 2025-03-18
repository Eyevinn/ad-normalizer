import { FastifyPluginCallback } from 'fastify';
import logger from '../util/logger';
import { PackagingProgress, PackagingService } from './packagingservice';

export interface PackagerCallbackOptions {
  packagingService: PackagingService;
}

export const packagingCallbackApi: FastifyPluginCallback<
  PackagerCallbackOptions
> = (fastify, opts, next) => {
  fastify.post<{ Body: PackagingProgress }>(
    '/packagerCallback',
    async (request, reply) => {
      logger.info('Packager callback received');
      const job = request.body;
      await opts.packagingService.handleCallback(job);
      reply.send();
    }
  );
  next;
};
