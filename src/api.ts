import fastify from 'fastify';
import cors from '@fastify/cors';
import swagger from '@fastify/swagger';
import swaggerUI from '@fastify/swagger-ui';
import { TypeBoxTypeProvider } from '@fastify/type-provider-typebox';
import { Static, Type } from '@sinclair/typebox';
import { FastifyPluginCallback } from 'fastify';
import { ManifestAsset, vastApi } from './vast/vastApi';
import getConfiguration from './config/config';
import { IN_PROGRESS, DEFAULT_TTL, RedisClient } from './redis/redisclient';
import logger from './util/logger';
import { EncoreClient } from './encore/encoreclient';
import { MinioClient, MinioNotification } from './minio/minio';

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

  const redisclient = new RedisClient(config.redisUrl);

  redisclient.connect();

  const saveToRedis = (key: string, value: string, ttl: number) => {
    logger.info('Saving to Redis', { key, value });
    redisclient.set(key, value, ttl);
  };
  logger.debug('callback listener URL:', config.callbackListenerUrl);
  const encoreClient = new EncoreClient(
    config.encoreUrl,
    config.callbackListenerUrl,
    config.oscToken
  );

  const minioClient = new MinioClient(
    config.s3Endpoint,
    config.s3AccessKey,
    config.s3SecretKey
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
    assetServerUrl: `https://${config.s3Endpoint}/${config.bucket}/`,
    lookUpAsset: async (mediaFile: string) => redisclient.get(mediaFile),
    onMissingAsset: async (asset: ManifestAsset) => {
      saveToRedis(
        asset.creativeId,
        IN_PROGRESS,
        config.inFlightTtl ? config.inFlightTtl : DEFAULT_TTL
      ); // Use the key as a lock to avoid multiple jobs being started
      return encoreClient.createEncoreJob(asset);
    },
    setupNotification: (asset: ManifestAsset) => {
      logger.debug('Setting up notification for asset', { asset });
      minioClient.listenForNotifications(
        config.bucket,
        asset.creativeId + '/', // TODO: Pass encore job id and add as part of the prefix
        'index.m3u8',
        async (notification: MinioNotification) =>
          await saveToRedis(asset.creativeId, notification.s3.object.key, 0)
      );
    }
  });
  return api;
};
