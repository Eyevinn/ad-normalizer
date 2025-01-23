import { createClient } from 'redis';
import logger from '../util/logger';

export const IN_PROGRESS = 'IN_PROGRESS';
export const DEFAULT_TTL = 1800; // TTL of 30 minutes to account for queue time

export class RedisClient {
  private client: Awaited<ReturnType<typeof createClient>> | null = null;
  constructor(private url: string) {}

  async connect() {
    if (this.client != null) {
      logger.info('Redis client already connected');
      return;
    }
    logger.info('Connecting to Redis', { url: this.url });
    this.client = await createClient({ url: this.url })
      .on('error', (err) => logger.error('Redis error', err))
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
}
