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
    '/packagerCallback/success',
    async (request, reply) => {
      logger.info('Packager callback received');
      const job = request.body;
      await opts.packagingService.handlePackagingCompleted(job);
      reply.send();
    }
  );
  fastify.post<{ Body: PackagingProgress }>(
    '/packagerCallback/failure',
    async (request, reply) => {
      logger.info('Packager callback received');
      const job = request.body;
      await opts.packagingService.handlePackagingFailed(job);
      reply.send();
    }
  );
  next;
};
