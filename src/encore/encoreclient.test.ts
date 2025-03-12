import { EncoreClient } from './encoreclient';
import { EncoreJob, InputType } from './types';

jest.mock('../util/logger', () => ({
  info: jest.fn()
}));

global.fetch = jest.fn(() =>
  Promise.resolve({
    json: () => Promise.resolve({})
  })
) as jest.Mock;

describe('EncoreClient', () => {
  const url = 'http://encore-url.osaas.io';
  const callbackUrl = 'http://callback-url.osaas.io';
  const serviceAccessToken = 'test-token';
  let encoreClient: EncoreClient;

  beforeEach(() => {
    encoreClient = new EncoreClient(url, callbackUrl, serviceAccessToken);
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('should create and submit an encore job', async () => {
    const job: EncoreJob = {
      externalId: '123',
      profile: 'program',
      outputFolder: '/usercontent/',
      baseName: '123',
      progressCallbackUri: callbackUrl,
      inputs: [
        {
          uri: 'input.mp4',
          seekTo: 0,
          copyTs: true,
          type: InputType.AUDIO_VIDEO
        }
      ]
    };

    await encoreClient.submitJob(job, serviceAccessToken);

    expect(global.fetch).toHaveBeenCalledWith(`${url}/encoreJobs`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/hal+json',
        'x-jwt': `Bearer ${serviceAccessToken}`
      },
      body: JSON.stringify(job)
    });
  });

  it('Should not append access token if none is provided', async () => {
    encoreClient = new EncoreClient(url, callbackUrl, 'test-profile');
    const job: EncoreJob = {
      externalId: '123',
      profile: 'program',
      outputFolder: '/usercontent/',
      baseName: '123',
      progressCallbackUri: callbackUrl,
      inputs: [
        {
          uri: 'http://example.com/playlist.m3u8',
          seekTo: 0,
          copyTs: true,
          type: InputType.AUDIO_VIDEO
        }
      ]
    };

    await encoreClient.submitJob(job);

    expect(global.fetch).toHaveBeenCalledWith(`${url}/encoreJobs`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/hal+json'
      },
      body: JSON.stringify(job)
    });
  });
});
