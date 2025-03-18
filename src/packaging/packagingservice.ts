import { create } from 'domain';
import {
  JobProgress,
  TranscodeInfo,
  TranscodeStatus
} from '../data/transcodeinfo';
import { RedisClient } from '../redis/redisclient';
import logger from '../util/logger';
import { createPackageUrl } from '../util/string';

export type PackagingProgress = {
  externalId: string;
  jobId: string;
  status: string;
  progress: number;
  outputFolder: string;
  baseName: string;
};

export class PackagingService {
  constructor(
    private redisClient: RedisClient,
    private assetServerUrl: string,
    private redisTtl: number
  ) {}

  // We can use the same job progress type as the encore service
  async handleCallback(jobProgress: PackagingProgress): Promise<void> {
    switch (jobProgress.status) {
      case 'COMPLETED':
        return this.handlePackagingCompleted(jobProgress);
      case 'FAILED':
        return this.handlePackagingFailed(jobProgress);
      default:
        logger.info("Job status doesn't match any known status", jobProgress);
        return Promise.resolve();
    }
  }

  async handlePackagingFailed(jobProgress: PackagingProgress): Promise<void> {
    const job = await this.redisClient.get(jobProgress.externalId);
    if (!job) {
      logger.error('Job not found in Redis', jobProgress);
      return;
    }
    return this.redisClient.delete(jobProgress.externalId);
  }
  async handlePackagingCompleted(
    jobProgress: PackagingProgress
  ): Promise<void> {
    const job = await this.redisClient.get(jobProgress.externalId);
    if (!job) {
      logger.error('Job not found in Redis', jobProgress);
      return;
    }
    const transcodeInfo = JSON.parse(job) as TranscodeInfo;
    const packageUrl = createPackageUrl(
      this.assetServerUrl,
      jobProgress.outputFolder,
      jobProgress.baseName
    );
    transcodeInfo.url = packageUrl;
    transcodeInfo.status = TranscodeStatus.COMPLETED;
    return this.redisClient.saveTranscodeStatus(
      jobProgress.externalId,
      transcodeInfo,
      this.redisTtl
    );
  }
}
