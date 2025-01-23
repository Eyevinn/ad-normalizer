import { RedisClient } from './redisclient';
import { createClient } from 'redis';
import logger from '../util/logger';

jest.mock('redis', () => ({
  createClient: jest.fn(() => ({
    on: jest.fn().mockReturnThis(),
    connect: jest.fn(),
    disconnect: jest.fn(),
    get: jest.fn(),
    set: jest.fn()
  }))
}));

jest.mock('../util/logger', () => ({
  info: jest.fn(),
  error: jest.fn()
}));

describe('RedisClient', () => {
  let redisClient: RedisClient;
  const mockUrl = 'redis://localhost:6379';

  beforeEach(() => {
    redisClient = new RedisClient(mockUrl);
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('should connect to Redis', async () => {
    await redisClient.connect();
    expect(createClient).toHaveBeenCalledWith({ url: mockUrl });
    expect(logger.info).toHaveBeenCalledWith('Connecting to Redis', {
      url: mockUrl
    });
  });

  it('should get a value from Redis', async () => {
    const mockKey = 'testKey';
    const mockValue = 'testValue';
    redisClient['client'] = {
      get: jest.fn().mockResolvedValue(mockValue)
    } as any;

    const value = await redisClient.get(mockKey);
    expect(redisClient['client']?.get).toHaveBeenCalledWith(mockKey);
    expect(value).toBe(mockValue);
  });

  it('should set a value in Redis and set TTL if needed', async () => {
    const mockKey = 'testKey';
    const mockValue = 'testValue';
    redisClient['client'] = {
      set: jest.fn(),
      expire: jest.fn()
    } as any;

    await redisClient.set(mockKey, mockValue, 10);
    expect(redisClient['client']?.set).toHaveBeenCalledWith(mockKey, mockValue);
    expect(redisClient['client']?.expire).toHaveBeenCalledWith(mockKey, 10);
  });

  it('should set a value in Redis and persist if it has an expire time', async () => {
    const mockKey = 'testKey';
    const mockValue = 'testValue';
    redisClient['client'] = {
      set: jest.fn(),
      expire: jest.fn(),
      expireTime: jest.fn().mockResolvedValue(10),
      persist: jest.fn()
    } as any;

    await redisClient.set(mockKey, mockValue, 0);
    expect(redisClient['client']?.set).toHaveBeenCalledWith(mockKey, mockValue);
    expect(redisClient['client']?.expireTime).toHaveBeenCalledWith(mockKey);
    expect(redisClient['client']?.persist).toHaveBeenCalledWith(mockKey);
  });

  it('should set a value in Redis and not persist if expire time is -1', async () => {
    const mockKey = 'testKey';
    const mockValue = 'testValue';
    redisClient['client'] = {
      set: jest.fn(),
      expire: jest.fn(),
      expireTime: jest.fn().mockResolvedValue(-1),
      persist: jest.fn()
    } as any;

    await redisClient.set(mockKey, mockValue, 0);
    expect(redisClient['client']?.set).toHaveBeenCalledWith(mockKey, mockValue);
    expect(redisClient['client']?.expireTime).toHaveBeenCalledWith(mockKey);
  });

  it('should log an error if client is not connected when getting a key', async () => {
    redisClient['client'] = null;
    await redisClient.get('testKey');
    expect(logger.error).toHaveBeenCalledWith('Redis client not connected');
  });
});
