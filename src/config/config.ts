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
  const encoreUrl = process.env.ENCORE_URL;
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
  const redisUrl = process.env.REDIS_URL;
  const bucketRaw = process.env.OUTPUT_BUCKET_URL;
  if (!bucketRaw) {
    throw new Error('OUTPUT_BUCKET_URL is required');
  }
  const bucket = new URL(bucketRaw);
  const bucketPath = bucket.pathname
    ? bucket.hostname + '/' + bucket.pathname
    : bucket.hostname;
  const oscToken = process.env.OSC_ACCESS_TOKEN;
  const configuration = {
    encoreUrl: encoreUrl,
    callbackListenerUrl: callbackListenerUrl.toString(),
    s3Endpoint: endpoint,
    s3AccessKey: accessKey,
    s3SecretKey: secretKey,
    adServerUrl: adServerUrl,
    redisUrl: redisUrl,
    bucket: bucketPath,
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
