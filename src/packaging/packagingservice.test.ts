import { TranscodeInfo, TranscodeStatus } from '../data/transcodeinfo';
import { EncoreClient } from '../encore/encoreclient';
import { RedisClient } from '../redis/redisclient';
import {
  PackagingFailureBody,
  PackagingService,
  PackagingSuccessBody
} from './packagingservice';
import { EncoreJob } from '../encore/types';

describe('packaging service', () => {
  let redisClient: jest.Mocked<RedisClient>;
  let encoreClient: jest.Mocked<EncoreClient>;
  let packagingService: PackagingService;
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
    packagingService = new PackagingService(
      redisClient,
      encoreClient,
      'http://asset-server-url',
      1000
    );
  });
  it('should handle packaging failed', async () => {
    const tcInfo: TranscodeInfo = {
      url: '',
      aspectRatio: '16:9',
      framerates: [25, 50],
      status: TranscodeStatus.PACKAGING
    };

    jest.spyOn(redisClient, 'get').mockResolvedValue(JSON.stringify(tcInfo));
    jest.spyOn(redisClient, 'delete').mockResolvedValue(Promise.resolve());
    jest
      .spyOn(redisClient, 'saveTranscodeStatus')
      .mockResolvedValue(Promise.resolve());
    const failureEvent: PackagingFailureBody = {
      message: JSON.stringify({
        jobId: 'test-job-id',
        url: 'https://encore-instance'
      })
    };
    jest.spyOn(encoreClient, 'getEncoreJob').mockResolvedValue({
      id: 'test-job-id',
      externalId: 'test-external-id'
    } as EncoreJob);
    await packagingService.handlePackagingFailed(failureEvent);
    expect(redisClient.get).toHaveBeenCalledWith('test-external-id');
    expect(redisClient.delete).toHaveBeenCalledWith('test-external-id');
    expect(redisClient.saveTranscodeStatus).not.toHaveBeenCalled();
  });
  it('should handle packaging completed', async () => {
    const tcInfo: TranscodeInfo = {
      url: '',
      aspectRatio: '16:9',
      framerates: [25, 50],
      status: TranscodeStatus.PACKAGING
    };

    jest.spyOn(redisClient, 'get').mockResolvedValue(JSON.stringify(tcInfo));
    jest
      .spyOn(redisClient, 'saveTranscodeStatus')
      .mockResolvedValue(Promise.resolve());
    jest.spyOn(encoreClient, 'getEncoreJob').mockResolvedValue({
      id: 'test-job-id',
      externalId: 'test-external-id',
      outputFolder: 'output-folder',
      baseName: 'base-name'
    } as EncoreJob);
    const progress: PackagingSuccessBody = {
      jobId: 'test-job-id',
      url: 'https://encore-instance'
    };
    await packagingService.handlePackagingCompleted(progress);
    expect(redisClient.get).toHaveBeenCalledWith('test-external-id');
    const expectedTcInfo: TranscodeInfo = {
      ...tcInfo,
      url: 'http://asset-server-url/output-folder/base-name.m3u8',
      status: TranscodeStatus.COMPLETED
    };
    expect(redisClient.saveTranscodeStatus).toHaveBeenCalledWith(
      'test-external-id',
      expectedTcInfo,
      1000
    );
  });
});
