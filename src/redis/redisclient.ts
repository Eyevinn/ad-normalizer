import { createClient, createCluster } from 'redis';
import logger from '../util/logger';
import { TranscodeInfo } from '../data/transcodeinfo';

export const IN_PROGRESS = 'IN_PROGRESS';
export const DEFAULT_TTL = 1800; // TTL of 30 minutes to account for queue time

export class RedisClient {
  private client: Awaited<ReturnType<typeof createClient>> | null = null;
  private cluster: Awaited<ReturnType<typeof createCluster>> | null = null;
  constructor(
    private url: string,
    private packagingQueueName?: string,
    private clusterMode: boolean = false
  ) {}

  async connectCluster() {
    logger.info('Connecting to Redis cluster', { url: this.url });
    if (this.cluster != null) {
      logger.info('Redis cluster already connected');
      return;
    }
    logger.info('Connecting to Redis cluster', { url: this.url });
    const urls = this.url.split(',');
    const rootNodes = urls.map((url) => {
      return { url: url };
    });
    this.cluster = createCluster({
      rootNodes: rootNodes
    }).on('error', (err) => logger.error('Redis cluster error:', err));
    await this.cluster.connect();
    return;
  }

  async connect() {
    if (this.clusterMode) {
      await this.connectCluster();
      return;
    }
    if (this.client != null) {
      logger.info('Redis client already connected');
      return;
    }
    logger.info('Connecting to Redis', { url: this.url });
    this.client = await createClient({
      url: this.url,
      socket: {
        reconnectStrategy: 1000
      },
      pingInterval: 3000
    })
      .on('error', (err) => logger.error('Redis error:', err))
      .connect();
    return;
  }

  async disconnect() {
    await this.client?.disconnect();
    this.client = null;
  }

  async get(key: string): Promise<string | null | undefined> {
    await this.connect();
    if (this?.client == null && this?.cluster == null) {
      logger.error('Redis client not connected');
    }
    logger.info('Getting key', { key });
    return this.clusterMode ? this.cluster?.get(key) : this.client?.get(key);
  }

  async getTranscodeStatus(key: string): Promise<TranscodeInfo | null> {
    const value = await this.get(key);
    if (value == null) {
      return null;
    } else {
      return JSON.parse(value);
    }
  }

  async set(key: string, value: string, ttl: number): Promise<void> {
    logger.info('Setting key', { key, value });
    await this.connect();
    this.clusterMode
      ? this.cluster?.set(key, value)
      : this.client?.set(key, value);
    await this.setTtl(key, ttl);
  }

  async setTtl(key: string, ttl: number): Promise<void> {
    if (ttl > 0) {
      logger.info('Setting key expiration', { key, ttl });
      this.clusterMode
        ? await this.cluster?.expire(key, ttl)
        : await this.client?.expire(key, ttl);
    } else {
      const expireTime = await this.client?.expireTime(key);
      if (expireTime != undefined) {
        switch (expireTime) {
          case -1:
            logger.info('Key has no expiration', { key });
            break;
          case -2:
            logger.error('Key does not exist', { key });
            break;
          default:
            this.clusterMode
              ? await this.cluster?.persist(key)
              : await this.client?.persist(key);
        }
      }
    }
  }

  async delete(key: string): Promise<void> {
    await this.connect();
    await this.client?.del(key);
  }

  async saveTranscodeStatus(
    key: string,
    status: TranscodeInfo,
    ttl: number
  ): Promise<void> {
    const stringifiedStatus = JSON.stringify(status);
    await this.set(key, stringifiedStatus, ttl);
  }

  async enqueuePackagingJob(stringifiedJob: string): Promise<void> {
    await this.connect();
    if (!this.packagingQueueName) {
      logger.error('No packaging queue name provided');
      return;
    }
    // Null check below needs to be handled way better
    await this.client?.zAdd(this.packagingQueueName, {
      score: Date.now(),
      value: stringifiedJob
    });
  }
}
