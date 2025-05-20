import logger from '../util/logger';
import { EncoreJob } from './types';
import { Context } from '@osaas/client-core';

export class EncoreClient {
  constructor(
    private url: string,
    public profile: string,
    private oscToken?: string
  ) {}

  async submitJob(
    job: EncoreJob,
    serviceAccessToken?: string
  ): Promise<Response> {
    logger.info('Submitting job to Encore', { job });
    const contentHeaders = {
      'Content-Type': 'application/json',
      Accept: 'application/hal+json'
    };
    const jwtHeader: { 'x-jwt': string } | Record<string, never> =
      serviceAccessToken ? { 'x-jwt': `Bearer ${serviceAccessToken}` } : {};
    return fetch(`${this.url}/encoreJobs`, {
      method: 'POST',
      headers: {
        ...contentHeaders,
        ...jwtHeader,
        'User-Agent': 'eyevinn/ad-normalizer'
      },
      body: JSON.stringify(job)
    });
  }

  async createEncoreJob(job: EncoreJob): Promise<Response> {
    let sat;
    if (this.oscToken) {
      const ctx = new Context({
        personalAccessToken: this.oscToken
      });
      sat = await ctx.getServiceAccessToken('encore');
    }
    return this.submitJob(job, sat);
  }

  async getEncoreJob(jobId: string): Promise<EncoreJob> {
    let sat;
    if (this.oscToken) {
      const ctx = new Context({
        personalAccessToken: this.oscToken
      });
      sat = await ctx.getServiceAccessToken('encore');
    }
    return this.fetchEncoreJob(jobId, sat);
  }

  async fetchEncoreJob(
    jobId: string,
    serviceAccessToken?: string
  ): Promise<EncoreJob> {
    const contentHeaders = {
      'Content-Type': 'application/json',
      Accept: 'application/hal+json'
    };
    const jwtHeader: { 'x-jwt': string } | Record<string, never> =
      serviceAccessToken ? { 'x-jwt': `Bearer ${serviceAccessToken}` } : {};
    const response = await fetch(`${this.url}/encoreJobs/${jobId}`, {
      headers: { ...contentHeaders, ...jwtHeader }
    });
    if (!response.ok) {
      logger.error(`Failed to get encore job: ${response.statusText}`);
      throw new Error(`Failed to get encore job: ${response.statusText}`);
    }
    return (await response.json()) as EncoreJob;
  }
}
