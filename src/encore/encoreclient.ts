import logger from '../util/logger';
import { ManifestAsset } from '../vast/vastApi';
import { EncoreJob, InputType } from './types';
import { Context } from '@osaas/client-core';

export class EncoreClient {
  constructor(
    private url: string,
    private callbackUrl: string,
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
      headers: { ...contentHeaders, ...jwtHeader },
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

  async getEncoreJob(
    jobId: string,
    serviceAccessToken?: string // TODO: Add the SAT when needed
  ): Promise<EncoreJob> {
    const response = await fetch(`${this.url}/encoreJobs/${jobId}`, {
      headers: {}
    });
    if (!response.ok) {
      throw new Error(`Failed to get encore job: ${response.statusText}`);
    }
    return (await response.json()) as EncoreJob;
  }
}
