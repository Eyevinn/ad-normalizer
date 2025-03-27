import { TranscodeInfo, TranscodeStatus } from '../data/transcodeinfo';
import { EncoreClient } from '../encore/encoreclient';
import { EncoreJob } from '../encore/types';
import { RedisClient } from '../redis/redisclient';
import logger from '../util/logger';
import { createPackageUrl } from '../util/string';

export type PackagingSuccessBody = {
  url: string;
  jobId: string;
  outputPath: string;
};

export type PackagingFailureBody = {
  message: string;
};

export class PackagingService {
  constructor(
    private redisClient: RedisClient,
    private encoreClient: EncoreClient,
    private assetServerUrl: string,
    private redisTtl: number
  ) {}

  async handlePackagingFailed(fail: PackagingFailureBody): Promise<void> {
    const parsedMsg = JSON.parse(fail.message);
    if (parsedMsg.jobId) {
      const encoreJob = await this.encoreClient.getEncoreJob(parsedMsg.jobId);
      if (encoreJob.externalId) {
        const job = await this.redisClient.get(encoreJob.externalId);
        if (!job) {
          logger.error('Job not found in Redis', encoreJob.externalId);
          return;
        }

        return this.redisClient.delete(encoreJob.externalId);
      }
    }
  }
  async handlePackagingCompleted(success: PackagingSuccessBody): Promise<void> {
    const encoreJob: EncoreJob = await this.encoreClient.getEncoreJob(
      success.jobId
    );
    if (!encoreJob.externalId) {
      logger.error('Encore job has no external ID', encoreJob.id);
      return;
    }
    const job = await this.redisClient.get(encoreJob.externalId);
    if (!job) {
      logger.error('Job not found in Redis', success.jobId);
      return;
    }
    const transcodeInfo = JSON.parse(job) as TranscodeInfo;
    const packageUrl = createPackageUrl(
      this.assetServerUrl,
      success.outputPath,
      'index'
    );
    transcodeInfo.url = packageUrl;
    transcodeInfo.status = TranscodeStatus.COMPLETED;
    return this.redisClient.saveTranscodeStatus(
      encoreJob.externalId,
      transcodeInfo,
      this.redisTtl
    );
  }
}
