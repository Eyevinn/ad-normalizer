import { createClient } from 'redis';
import logger from '../util/logger';
import { TranscodeInfo } from '../data/transcodeinfo';

export const IN_PROGRESS = 'IN_PROGRESS';
export const DEFAULT_TTL = 1800; // TTL of 30 minutes to account for queue time

export class RedisClient {
  private client: Awaited<ReturnType<typeof createClient>> | null = null;
  constructor(private url: string, private packagingQueueName?: string) {}

  async connect() {
    if (this.client != null) {
      logger.info('Redis client already connected');
      return;
    }
    logger.info('Connecting to Redis', { url: this.url });
    this.client = await createClient({
      url: this.url,
      socket: {
        keepAlive: 1,
        reconnectStrategy: 1000
      }
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
    if (this?.client == null) {
      logger.error('Redis client not connected');
    }
    logger.info('Getting key', { key });
    return this.client?.get(key);
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
    this.client?.set(key, value);
    await this.setTtl(key, ttl);
  }

  async setTtl(key: string, ttl: number): Promise<void> {
    if (ttl > 0) {
      logger.info('Setting key expiration', { key, ttl });
      await this.client?.expire(key, ttl);
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
            await this.client?.persist(key);
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
