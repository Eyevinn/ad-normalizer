import fastify from 'fastify';
import cors from '@fastify/cors';
import swagger from '@fastify/swagger';
import swaggerUI from '@fastify/swagger-ui';
import { TypeBoxTypeProvider } from '@fastify/type-provider-typebox';
import { Static, Type } from '@sinclair/typebox';
import { FastifyPluginCallback } from 'fastify';
import { ManifestAsset, vastApi } from './vast/vastApi';
import { vmapApi } from './vmap/vmapApi';
import getConfiguration from './config/config';
import { DEFAULT_TTL, RedisClient } from './redis/redisclient';
import logger from './util/logger';
import { EncoreClient } from './encore/encoreclient';
import { MinioClient } from './minio/minio';
import { TranscodeInfo, TranscodeStatus } from './data/transcodeinfo';
import { EncoreService } from './encore/encoreservice';
import { PackagingService } from './packaging/packagingservice';
import { encoreCallbackApi } from './encore/encorecallbackapi';
import { packagingCallbackApi } from './packaging/packagingcallbackapi';
import { EncoreJob } from './encore/types';

const HelloWorld = Type.String({
  status: 'HEALTHY'
});

export interface HealthcheckOptions {
  title: string;
}

const healthcheck: FastifyPluginCallback<HealthcheckOptions> = (
  fastify,
  opts,
  next
) => {
  fastify.get<{ Reply: Static<typeof HelloWorld> }>(
    '/',
    {
      schema: {
        description: 'Health check',
        response: {
          200: HelloWorld
        }
      }
    },
    async (_, reply) => {
      reply.send('Hello, world! I am ' + opts.title);
    }
  );
  next();
};

export interface ApiOptions {
  title: string;
}

export default (opts: ApiOptions) => {
  logger.info('starting server');
  const config = getConfiguration();

  if (!config.oscToken) {
    logger.info(
      "No service access token provided. If you're running the app outside of OSC, you won't be able to access the API."
    );
  } else {
    logger.info(
      'Service access token provided. You should be able to access the API.'
    );
  }

  const redisclient = new RedisClient(
    config.redisUrl,
    config.packagingQueueName
  );

  redisclient.connect();

  const saveToRedis = (key: string, value: string, ttl: number) => {
    logger.info('Saving to Redis', { key, value });
    redisclient.set(key, value, ttl);
  };
  logger.debug('callback listener URL:', config.callbackListenerUrl);
  const encoreClient = new EncoreClient(
    config.encoreUrl,
    config.callbackListenerUrl,
    config.encoreProfile,
    config.oscToken
  );

  const minioClient = new MinioClient(
    config.s3Endpoint,
    config.s3AccessKey,
    config.s3SecretKey
  );

  const encoreService = new EncoreService(
    encoreClient,
    config.jitPackaging,
    redisclient,
    `https://${config.s3Endpoint}/${config.bucket}/`,
    config.inFlightTtl ? config.inFlightTtl : DEFAULT_TTL,
    config.rootUrl,
    config.encoreUrl,
    config.bucketUrl
  );

  const packagingService = new PackagingService(
    redisclient,
    encoreClient,
    config.assetServerUrl
      ? config.assetServerUrl
      : `https://${config.s3Endpoint}/${config.bucket}/`,
    config.inFlightTtl ? config.inFlightTtl : DEFAULT_TTL
  );

  minioClient.setupClient();

  const api = fastify({
    ignoreTrailingSlash: true
  }).withTypeProvider<TypeBoxTypeProvider>();

  // register the cors plugin, configure it for better security
  api.register(cors);

  // register the swagger plugins, it will automagically do magic
  api.register(swagger, {
    swagger: {
      info: {
        title: opts.title,
        description: 'hello',
        version: 'v1'
      }
    }
  });
  api.register(swaggerUI, {
    routePrefix: '/docs'
  });

  api.register(healthcheck, { title: opts.title });
  // register other API routes here

  api.register(vastApi, {
    adServerUrl: config.adServerUrl,
    assetServerUrl: config.assetServerUrl
      ? config.assetServerUrl
      : `https://${config.s3Endpoint}/${config.bucket}/`,
    keyField: config.keyField,
    keyRegex: new RegExp(config.keyRegex, 'g'),
    encoreService: encoreService,
    lookUpAsset: async (mediaFile: string) =>
      redisclient.getTranscodeStatus(mediaFile),
    onMissingAsset: async (asset: ManifestAsset) => {
      const inProgressInfo: TranscodeInfo = {
        url: '',
        aspectRatio: '',
        framerates: [],
        status: TranscodeStatus.IN_PROGRESS
      };
      return encoreService.createEncoreJob(asset).then((res: Response) => {
        if (res.ok) {
          redisclient.saveTranscodeStatus(
            asset.creativeId,
            inProgressInfo,
            config.inFlightTtl ? config.inFlightTtl : DEFAULT_TTL
          );
          res.json().then((data: EncoreJob) => {
            logger.info('Encore job created', { jobId: data.id });
          });
          return Promise.resolve(inProgressInfo);
        } else {
          logger.error('Failed to start job', { asset });
          return Promise.resolve(null);
        }
      });
    }
  });

  api.register(vmapApi, {
    adServerUrl: config.adServerUrl,
    assetServerUrl: config.assetServerUrl
      ? config.assetServerUrl
      : `https://${config.s3Endpoint}/${config.bucket}/`,
    keyField: config.keyField,
    keyRegex: new RegExp(config.keyRegex, 'g'),
    encoreService: encoreService,
    lookUpAsset: async (mediaFile: string) =>
      redisclient.getTranscodeStatus(mediaFile),
    onMissingAsset: async (asset: ManifestAsset) => {
      const inProgressInfo: TranscodeInfo = {
        url: '',
        aspectRatio: '',
        framerates: [],
        status: TranscodeStatus.IN_PROGRESS
      };
      return encoreService.createEncoreJob(asset).then((res: Response) => {
        if (res.ok) {
          redisclient.saveTranscodeStatus(
            asset.creativeId,
            inProgressInfo,
            config.inFlightTtl ? config.inFlightTtl : DEFAULT_TTL
          );
          res.json().then((data: EncoreJob) => {
            logger.info('Encore job created', { jobId: data.id });
          });
          return Promise.resolve(inProgressInfo);
        } else {
          logger.error('Failed to start job', { asset });
          return Promise.resolve(null);
        }
      });
    }
  });

  api.register(encoreCallbackApi, {
    encoreService: encoreService
  });

  api.register(packagingCallbackApi, {
    packagingService: packagingService
  });

  return api;
};
