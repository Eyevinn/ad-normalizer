import { FastifyPluginCallback } from 'fastify';
import logger from '../util/logger';
import {
  PackagingFailureBody,
  PackagingService,
  PackagingSuccessBody
} from './packagingservice';

export interface PackagerCallbackOptions {
  packagingService: PackagingService;
}

export const packagingCallbackApi: FastifyPluginCallback<
  PackagerCallbackOptions
> = (fastify, opts, next) => {
  fastify.post<{ Body: PackagingSuccessBody }>(
    '/packagerCallback/success',
    async (request, reply) => {
      logger.info('Packager callback received');
      const event = request.body;
      await opts.packagingService.handlePackagingCompleted(event);
      reply.send();
    }
  );
  fastify.post<{ Body: PackagingFailureBody }>(
    '/packagerCallback/failure',
    async (request, reply) => {
      logger.info('Packager callback received');
      const event = request.body;
      await opts.packagingService.handlePackagingFailed(event);
      reply.send();
    }
  );
  next();
};
