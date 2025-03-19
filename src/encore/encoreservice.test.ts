import { JobProgress, TranscodeStatus } from '../data/transcodeinfo';
import { RedisClient } from '../redis/redisclient';
import { EncoreClient } from './encoreclient';
import { EncoreService } from './encoreservice';
import { EncoreJob, EncoreStatus } from './types';

describe('encore service with static packaging', () => {
  let redisClient: jest.Mocked<RedisClient>;
  let encoreClient: jest.Mocked<EncoreClient>;
  let encoreService: EncoreService;

  beforeEach(() => {
    redisClient = new RedisClient(
      'redis://test-url',
      'ad-packaging'
    ) as jest.Mocked<RedisClient>;
    encoreClient = new EncoreClient(
      'http://encore-url',
      'http://normalizer/callback',
      'ad-profile'
    ) as jest.Mocked<EncoreClient>;
    jest.spyOn(redisClient, 'connect').mockResolvedValue(Promise.resolve());
    encoreService = new EncoreService(
      encoreClient,
      false,
      redisClient,
      'assetServerUrl',
      1000,
      'https://eyevinn-ad-normalizer.osaas.io',
      'https://encore-url',
      new URL('s3://test-bucket')
    );
  });
  it('should react to success callbacks', async () => {
    jest.spyOn(encoreClient, 'getEncoreJob').mockResolvedValue({
      externalId: 'test-external-id',
      status: EncoreStatus.SUCCESSFUL,
      id: 'test-job-id'
    } as EncoreJob);

    jest.spyOn(redisClient, 'set').mockResolvedValue(Promise.resolve());
    const callback: JobProgress = {
      jobId: 'test-job-id',
      status: 'SUCCESSFUL',
      progress: 0.5,
      externalId: 'test-external-id'
    };
    await encoreService.handleCallback(callback);
    expect(encoreClient.getEncoreJob).toHaveBeenCalledWith('test-job-id');
    expect(redisClient.set).toHaveBeenCalledWith(
      'test-external-id',
      JSON.stringify({
        url: '',
        aspectRatio: '16:9',
        framerates: [],
        status: TranscodeStatus.PACKAGING
      }),
      1000
    );
  });
  it('should react to failure callbacks', async () => {
    jest.spyOn(encoreClient, 'getEncoreJob').mockResolvedValue({
      externalId: 'test-external-id',
      status: EncoreStatus.FAILED,
      id: 'test-job-id'
    } as EncoreJob);
    jest.spyOn(redisClient, 'delete').mockResolvedValue(Promise.resolve());
    const callback: JobProgress = {
      externalId: 'test-external-id',
      status: 'FAILED',
      progress: 0.5,
      jobId: 'test-job-id'
    };
    await encoreService.handleCallback(callback);
    expect(redisClient.delete).toHaveBeenCalledWith('test-external-id');
  });

  it('should react to in progress callbacks', async () => {
    jest.spyOn(encoreClient, 'getEncoreJob').mockResolvedValue({
      externalId: 'test-external-id',
      status: EncoreStatus.IN_PROGRESS,
      id: 'test-job-id'
    } as EncoreJob);
    jest.spyOn(redisClient, 'delete').mockResolvedValue(Promise.resolve());
    const callback: JobProgress = {
      externalId: 'test-external-id',
      status: 'IN_PROGRESS',
      progress: 0.5,
      jobId: 'test-job-id'
    };
    await encoreService.handleCallback(callback);
    expect(encoreClient.getEncoreJob).not.toHaveBeenCalled();
    expect(redisClient.delete).not.toHaveBeenCalled();
  });
});

describe('encore service with JIT packaging', () => {
  let redisClient: jest.Mocked<RedisClient>;
  let encoreClient: jest.Mocked<EncoreClient>;
  let encoreService: EncoreService;

  beforeEach(() => {
    redisClient = new RedisClient(
      'redis://test-url',
      'ad-packaging'
    ) as jest.Mocked<RedisClient>;
    encoreClient = new EncoreClient(
      'http://encore-url',
      'http://normalizer/callback',
      'ad-profile'
    ) as jest.Mocked<EncoreClient>;
    jest.spyOn(redisClient, 'connect').mockResolvedValue(Promise.resolve());
    encoreService = new EncoreService(
      encoreClient,
      true,
      redisClient,
      'http://assetServerUrl',
      1000,
      'https://eyevinn-ad-normalizer.osaas.io',
      'https://encore-url',
      new URL('s3://test-bucket')
    );
  });
  it('Should mark successful jobs as completed', async () => {
    jest.spyOn(encoreClient, 'getEncoreJob').mockResolvedValue({
      externalId: 'test-external-id',
      status: EncoreStatus.SUCCESSFUL,
      id: 'test-job-id',
      outputFolder: '/usercontent/',
      baseName: 'test-base-name'
    } as EncoreJob);

    jest.spyOn(redisClient, 'set').mockResolvedValue(Promise.resolve());
    const callback: JobProgress = {
      jobId: 'test-job-id',
      status: 'SUCCESSFUL',
      progress: 0.5,
      externalId: 'test-external-id'
    };
    await encoreService.handleCallback(callback);
    expect(encoreClient.getEncoreJob).toHaveBeenCalledWith('test-job-id');
    expect(redisClient.set).toHaveBeenCalledWith(
      'test-external-id',
      JSON.stringify({
        url: 'http://assetserverurl/usercontent/test-base-name.m3u8',
        aspectRatio: '16:9',
        framerates: [],
        status: TranscodeStatus.COMPLETED
      }),
      1000
    );
  });
});
