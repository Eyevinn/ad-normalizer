import { Context } from '@osaas/client-core';
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
  let ctx: Context;

  beforeEach(() => {
    ctx = new Context({
      personalAccessToken: 'test-pat'
    });
    jest
      .spyOn(ctx, 'getServiceAccessToken')
      .mockResolvedValue(serviceAccessToken);
    encoreClient = new EncoreClient(
      url,
      callbackUrl,
      'test-profle',
      'my-token'
    );
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
    const encoreClientNoSat = new EncoreClient(
      url,
      callbackUrl,
      'test-profile'
    );
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

    await encoreClientNoSat.submitJob(job);

    expect(global.fetch).toHaveBeenCalledWith(`${url}/encoreJobs`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/hal+json'
      },
      body: JSON.stringify(job)
    });
  });

  it('Should fetch an encore job', async () => {
    global.fetch = jest.fn(() =>
      Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
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
          })
      } as unknown as Response)
    ) as jest.Mock;

    const jobId = '123';
    await encoreClient.fetchEncoreJob(jobId, serviceAccessToken);

    expect(global.fetch).toHaveBeenCalledWith(`${url}/encoreJobs/${jobId}`, {
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/hal+json',
        'x-jwt': `Bearer ${serviceAccessToken}`
      }
    });
  });
});
