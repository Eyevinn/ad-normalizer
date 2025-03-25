import { RedisClient } from './redisclient';
import { createClient, RedisClientType } from 'redis';
import logger from '../util/logger';

// Define a type for our mocked Redis client
type MockRedisClient = {
  get: jest.Mock;
  set: jest.Mock;
  expire: jest.Mock;
  expireTime?: jest.Mock;
  persist?: jest.Mock;
};

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
    redisClient = new RedisClient(mockUrl, 'test-queue');
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('should connect to Redis', async () => {
    await redisClient.connect();
    expect(createClient).toHaveBeenCalledWith({
      url: mockUrl,
      socket: { reconnectStrategy: 1000 },
      pingInterval: 3000
    });
    expect(logger.info).toHaveBeenCalledWith('Connecting to Redis', {
      url: mockUrl
    });
  });

  it('should get a value from Redis', async () => {
    const mockKey = 'testKey';
    const mockValue = 'testValue';
    redisClient['client'] = {
      get: jest.fn().mockResolvedValue(mockValue)
    } as unknown as RedisClientType;

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
    } as unknown as RedisClientType;

    await redisClient.set(mockKey, mockValue, 10);
    expect(redisClient['client']?.set).toHaveBeenCalledWith(mockKey, mockValue);
    expect(redisClient['client']?.expire).toHaveBeenCalledWith(mockKey, 10);
  });

  it('should set a value in Redis and persist if it has an expire time', async () => {
    const mockKey = 'testKey';
    const mockValue = 'testValue';
    const mockClient: MockRedisClient = {
      get: jest.fn(),
      set: jest.fn(),
      expire: jest.fn(),
      expireTime: jest.fn().mockResolvedValue(10),
      persist: jest.fn()
    };
    redisClient['client'] = mockClient as unknown as RedisClientType;

    await redisClient.set(mockKey, mockValue, 0);
    expect(mockClient.set).toHaveBeenCalledWith(mockKey, mockValue);
    expect(mockClient.expireTime).toHaveBeenCalledWith(mockKey);
    expect(mockClient.persist).toHaveBeenCalledWith(mockKey);
  });

  it('should set a value in Redis and not persist if expire time is -1', async () => {
    const mockKey = 'testKey';
    const mockValue = 'testValue';
    const mockClient: MockRedisClient = {
      get: jest.fn(),
      set: jest.fn(),
      expire: jest.fn(),
      expireTime: jest.fn().mockResolvedValue(-1),
      persist: jest.fn()
    };
    redisClient['client'] = mockClient as unknown as RedisClientType;

    await redisClient.set(mockKey, mockValue, 0);
    expect(mockClient.set).toHaveBeenCalledWith(mockKey, mockValue);
    expect(mockClient.expireTime).toHaveBeenCalledWith(mockKey);
    expect(mockClient.persist).not.toHaveBeenCalled();
  });

  it('should log an error if client is not connected when getting a key', async () => {
    redisClient['client'] = null;
    await redisClient.get('testKey');
    expect(logger.error).toHaveBeenCalledWith('Redis client not connected');
  });
});
