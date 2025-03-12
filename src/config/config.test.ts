import getConfiguration from './config';

describe('config loading behavior', () => {
  it('should remove trailing slashes', () => {
    process.env.ENCORE_URL = 'http://encore-instance.io/';
    process.env.CALLBACK_LISTENER_URL = 'http://callback.com';
    process.env.S3_ENDPOINT = 'http://s3.host.io/';
    process.env.S3_ACCESS_KEY = 'access';
    process.env.S3_SECRET_KEY = 'secret';
    process.env.AD_SERVER_URL = 'http://adserver.com';
    process.env.REDIS_URL = 'http://redis.com';
    process.env.OUTPUT_BUCKET_URL = 's3://ads/';
    process.env.OSC_ACCESS_TOKEN = 'token';
    process.env.KEY_FIELD = 'Url';
    process.env.KEY_REGEX = '[a-zA-Z]';
    process.env.ENCORE_PROFILE = 'test-profile';

    const config = getConfiguration();
    const expectedBucketName = 'ads';
    const expectedEncoreUrl = 'http://encore-instance.io';
    // Assert
    expect(config.bucket).toEqual(expectedBucketName);
    expect(config.encoreUrl).toEqual(expectedEncoreUrl);
    expect(config.keyRegex).toEqual('[a-zA-Z]');
    expect(config.keyField).toEqual('url');
    expect(config.encoreProfile).toEqual('test-profile');
  });
});
