import path from 'path';
import { removeTrailingSlash } from '../util/string';

export interface AdNormalizerConfiguration {
  encoreUrl: string;
  bucket: string;
  adServerUrl: string;
  redisUrl: string;
  rediscluster: boolean;
  oscToken?: string;
  inFlightTtl?: number;
  keyField: string;
  keyRegex: string;
  encoreProfile: string;
  jitPackaging: boolean;
  packagingQueueName?: string;
  rootUrl: string;
  bucketUrl: URL;
  assetServerUrl: URL;
}

let config: AdNormalizerConfiguration | null = null;

const loadConfiguration = (): AdNormalizerConfiguration => {
  if (!process.env.ENCORE_URL) {
    throw new Error('ENCORE_URL is required');
  }
  const encoreUrl = new URL(removeTrailingSlash(process.env.ENCORE_URL));
  if (!process.env.ASSET_SERVER_URL) {
    throw new Error('ASSET_SERVER_URL is required');
  }
  const assetServerUrl = new URL(
    removeTrailingSlash(process.env.ASSET_SERVER_URL)
  );

  const adServerUrl = process.env.AD_SERVER_URL;
  if (!process.env.REDIS_URL) {
    throw new Error('REDIS_URL is required');
  }
  const redisUrl = process.env.REDIS_URL;
  const redisCluster = process.env.REDIS_CLUSTER === 'true';
  if (!process.env.OUTPUT_BUCKET_URL) {
    throw new Error('OUTPUT_BUCKET_URL is required');
  }
  const bucketRaw = removeTrailingSlash(process.env.OUTPUT_BUCKET_URL);
  const bucket = new URL(bucketRaw);
  const bucketPath =
    bucket.pathname === ''
      ? path.join(bucket.hostname, bucket.pathname)
      : bucket.hostname;
  const oscToken = process.env.OSC_ACCESS_TOKEN;
  const inFlightTtl = process.env.IN_FLIGHT_TTL;

  const keyField = process.env.KEY_FIELD;
  const keyRegex = process.env.KEY_REGEX;

  const encoreProfile = process.env.ENCORE_PROFILE;
  const jitPackaging = process.env.JIT_PACKAGING === 'true';
  const packagingQueueName = process.env.PACKAGING_QUEUE;

  const rootUrl = process.env.ROOT_URL;
  if (!rootUrl) {
    throw new Error(
      'ROOT_URL is required, otherwise encore callbacks will not work'
    );
  }

  const configuration = {
    encoreUrl: removeTrailingSlash(encoreUrl.toString()),
    adServerUrl: adServerUrl,
    redisUrl: redisUrl,
    rediscluster: redisCluster,
    bucket: removeTrailingSlash(bucketPath),
    oscToken: oscToken,
    inFlightTtl: inFlightTtl ? parseInt(inFlightTtl) : null,
    keyField: keyField ? keyField.toLowerCase() : 'UniversalAdId'.toLowerCase(),
    keyRegex: keyRegex ? keyRegex : '[^a-zA-Z0-9]',
    encoreProfile: encoreProfile ? encoreProfile : 'program',
    jitPackaging: jitPackaging,
    packagingQueueName: packagingQueueName,
    rootUrl: rootUrl,
    bucketUrl: bucket,
    assetServerUrl: assetServerUrl
  } as AdNormalizerConfiguration;

  return configuration;
};

/**
 * Gets the application config. Configuration is treated as a singleton.
 * If the configuration has not been loaded yet, it will be loaded from environment variables.
 * @returns configuration object
 */
export default function getConfiguration(): AdNormalizerConfiguration {
  if (config === null) {
    config = loadConfiguration();
  }
  return config as AdNormalizerConfiguration;
}
