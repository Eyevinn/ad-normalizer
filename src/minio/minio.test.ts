import { MinioClient } from './minio';
import * as Minio from 'minio';
import logger from '../util/logger';

jest.mock('minio');
jest.mock('../util/logger');

describe('MinioClient', () => {
  let minioClient: MinioClient;
  const url = 'http://localhost:9000';
  const accessKey = 'accessKey';
  const secretKey = 'secretKey';

  beforeEach(() => {
    minioClient = new MinioClient(url, accessKey, secretKey);
  });

  describe('setupClient', () => {
    it('should set up the Minio client', () => {
      minioClient.setupClient();
      expect(logger.debug).toHaveBeenCalledWith('Setting up Minio Client', {
        url
      });
      expect(minioClient['minioclient']).toBeInstanceOf(Minio.Client);
    });

    it('should log an error if the client is already connected', () => {
      minioClient['minioclient'] = new Minio.Client({
        endPoint: url,
        accessKey,
        secretKey,
        useSSL: true
      });
      minioClient.setupClient();
      expect(logger.error).toHaveBeenCalledWith(
        'Minio client already connected'
      );
    });
  });

  describe('listenForNotifications', () => {
    const bucketName = 'test-bucket';
    const assetId = 'test-asset';
    const masterPlaylistName = 'test-playlist';
    const onNotification = jest.fn().mockResolvedValue(undefined);

    it('should log an error if the client is not connected', () => {
      minioClient.listenForNotifications(
        bucketName,
        assetId,
        masterPlaylistName,
        onNotification
      );
      expect(logger.error).toHaveBeenCalledWith('Minio client not connected');
    });

    it('should log an error if poller creation fails', () => {
      Minio.Client.prototype.listenBucketNotification = jest
        .fn()
        .mockReturnValue(undefined);
      minioClient.setupClient();
      minioClient.listenForNotifications(
        bucketName,
        assetId,
        masterPlaylistName,
        onNotification
      );
      expect(logger.error).toHaveBeenCalledWith('Failed to create poller');
    });

    it('should log debug when poller is created and handle notifications', () => {
      const mockPoller = {
        on: jest.fn((event, callback) => {
          if (event === 'notification') {
            callback({
              s3: { bucket: { name: bucketName }, object: { key: 'test-key' } }
            });
          }
        }),
        stop: jest.fn()
      };
      Minio.Client.prototype.listenBucketNotification = jest
        .fn()
        .mockReturnValue(mockPoller);
      minioClient.setupClient();
      minioClient.listenForNotifications(
        bucketName,
        assetId,
        masterPlaylistName,
        onNotification
      );
      expect(mockPoller.on).toHaveBeenCalledWith(
        'notification',
        expect.any(Function)
      );
      expect(onNotification).toHaveBeenCalledWith({
        s3: { bucket: { name: bucketName }, object: { key: 'test-key' } }
      });
      expect(mockPoller.stop).toHaveBeenCalled();
    });
  });
});
