import { FastifyPluginCallback } from 'fastify';
import { JobProgress } from '../data/transcodeinfo';
import { EncoreService } from './encoreservice';

export interface EncoreCallbackOptions {
  encoreService: EncoreService;
  onCallback: (job: JobProgress) => void;
  onFail: (job: JobProgress) => void;
  onSuccess: (job: JobProgress) => void;
}

export const encoreCallbackApi: FastifyPluginCallback<EncoreCallbackOptions> = (
  fastify,
  opts,
  next
) => {
  fastify.post<{ Body: JobProgress }>(
    '/encoreCallback',
    async (request, reply) => {
      const job = request.body;
      await opts.encoreService.handleCallback(job);
      reply.send();
    }
  );
  next;
};
