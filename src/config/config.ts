import path from 'path';
import { removeTrailingSlash } from '../util/string';

export interface AdNormalizerConfiguration {
  encoreUrl: string;
  callbackListenerUrl: string;
  s3Endpoint: string;
  s3AccessKey: string;
  s3SecretKey: string;
  bucket: string;
  adServerUrl: string;
  redisUrl: string;
  oscToken?: string;
}

let config: AdNormalizerConfiguration | null = null;

const loadConfiguration = (): AdNormalizerConfiguration => {
  if (!process.env.ENCORE_URL) {
    throw new Error('ENCORE_URL is required');
  }
  const encoreUrl = new URL(removeTrailingSlash(process.env.ENCORE_URL));
  if (!process.env.CALLBACK_LISTENER_URL) {
    throw new Error('CALLBACK_LISTENER_URL is required');
  }
  // Handle whether CALLBACK_LISTENER_URL contains /encoreCallback
  // or not
  const callbackListenerUrl = new URL(
    '/encoreCallback',
    process.env.CALLBACK_LISTENER_URL
  );
  const endpoint = process.env.S3_ENDPOINT;
  const accessKey = process.env.S3_ACCESS_KEY;
  const secretKey = process.env.S3_SECRET_KEY;
  const adServerUrl = process.env.AD_SERVER_URL;
  if (!process.env.REDIS_URL) {
    throw new Error('REDIS_URL is required');
  }
  const redisUrl = process.env.REDIS_URL;
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
  const configuration = {
    encoreUrl: removeTrailingSlash(encoreUrl.toString()),
    callbackListenerUrl: callbackListenerUrl.toString(),
    s3Endpoint: endpoint,
    s3AccessKey: accessKey,
    s3SecretKey: secretKey,
    adServerUrl: adServerUrl,
    redisUrl: redisUrl,
    bucket: removeTrailingSlash(bucketPath),
    oscToken: oscToken
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
