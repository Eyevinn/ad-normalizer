import {
  JobProgress,
  TranscodeInfo,
  TranscodeStatus
} from '../data/transcodeinfo';
import { RedisClient } from '../redis/redisclient';
import { EncoreClient } from './encoreclient';
import logger from '../util/logger';
import { EncoreJob, EncoreStatus, InputType, VideoStream } from './types';
import { calculateAspectRatio } from '../util/aspectratio';
import { ManifestAsset } from '../vast/vastApi';
import { createPackageUrl } from '../util/string';
import { default as PathUtils } from 'path';
export class EncoreService {
  constructor(
    private client: EncoreClient,
    private jitPackaging: boolean,
    private redisClient: RedisClient,
    private assetServerUrl: string,
    private redisTtl: number,
    private rootUrl: string,
    private encoreUrl: string,
    private outputBucket: URL
  ) {}

  async createEncoreJob(creative: ManifestAsset): Promise<Response> {
    const job: EncoreJob = {
      externalId: creative.creativeId,
      profile: this.client.profile,
      outputFolder: new URL(
        PathUtils.join(this.outputBucket.pathname, creative.creativeId),
        this.outputBucket
      ).href,
      baseName: creative.creativeId,
      progressCallbackUri: this.rootUrl + '/encoreCallback', // Should figure out how to set this for the configured server
      inputs: [
        {
          uri: creative.masterPlaylistUrl,
          seekTo: 0,
          copyTs: true,
          type: InputType.AUDIO_VIDEO
        }
      ]
    };
    return this.client.createEncoreJob(job);
  }

  async handleCallback(jobProgress: JobProgress): Promise<void> {
    switch (jobProgress.status) {
      case 'SUCCESSFUL':
        return this.handleTranscodeCompleted(jobProgress);
      case 'FAILED':
        return this.handleTranscodeFailed(jobProgress);
      case 'IN_PROGRESS':
        return this.handleTranscodeInProgress(jobProgress);
      default:
        logger.info("Job status doesn't match any known status", jobProgress);
        return Promise.resolve();
    }
  }

  async handleTranscodeCompleted(jobProgress: JobProgress): Promise<void> {
    return this.client.getEncoreJob(jobProgress.jobId).then((job) => {
      const transcodeInfo = this.transcodeInfoFromEncoreJob(job);
      this.redisClient.saveTranscodeStatus(
        jobProgress.externalId,
        transcodeInfo,
        this.redisTtl
      );
      if (!this.jitPackaging) {
        const packagingQueueMessage = {
          jobId: jobProgress.jobId,
          url: `${this.encoreUrl}/encoreJobs/${jobProgress.jobId}`
        };
        this.redisClient.enqueuePackagingJob(
          JSON.stringify(packagingQueueMessage)
        );
      }
    });
  }

  async handleTranscodeFailed(jobProgress: JobProgress): Promise<void> {
    return this.redisClient.delete(jobProgress.externalId);
  }

  async handleTranscodeInProgress(jobProgress: JobProgress): Promise<void> {
    // No-op for now
    logger.info('Transcoding progress updated', { jobProgress });
    return Promise.resolve();
  }

  transcodeInfoFromEncoreJob(job: EncoreJob): TranscodeInfo {
    const jobStatus = this.getTranscodeStatus(job);
    const firstVideoStream = job.output?.reduce(
      (videoStreams: VideoStream[], output) => {
        return output.videoStreams
          ? [...videoStreams, ...output.videoStreams]
          : videoStreams;
      },
      []
    )[0];
    const aspectRatio = calculateAspectRatio(
      firstVideoStream?.height || 1920,
      firstVideoStream?.width || 1080
    ); // fallback to 16:9
    return {
      url: this.jitPackaging
        ? createPackageUrl(this.assetServerUrl, job.outputFolder, job.baseName)
        : '', // If packaging is not JIT, we shouldn't set URL here
      aspectRatio: aspectRatio,
      framerates: this.getFrameRates(job),
      status: jobStatus
    };
  }

  getTranscodeStatus(job: EncoreJob): TranscodeStatus {
    switch (job.status) {
      case EncoreStatus.SUCCESSFUL:
        return this.jitPackaging
          ? TranscodeStatus.COMPLETED
          : TranscodeStatus.PACKAGING;
      case EncoreStatus.FAILED:
        return TranscodeStatus.FAILED;
      case EncoreStatus.IN_PROGRESS:
        return TranscodeStatus.IN_PROGRESS;
      case EncoreStatus.QUEUED:
        return TranscodeStatus.IN_PROGRESS;
      case EncoreStatus.NEW:
        return TranscodeStatus.IN_PROGRESS;
      case EncoreStatus.CANCELLED:
        return TranscodeStatus.FAILED;
      default:
        return TranscodeStatus.UNKNOWN;
    }
  }

  getFrameRates(job: EncoreJob): number[] {
    return (
      job.output?.reduce((frameRates: number[], output) => {
        const videoStreams = output.videoStreams || [];
        const rates = videoStreams.map((stream) =>
          this.parseFrameRate(stream.frameRate)
        );
        return [...frameRates, ...rates];
      }, []) || []
    );
  }

  parseFrameRate(frameRate: string): number {
    const [numerator, denominator] = frameRate.split('/');
    return parseInt(numerator) / parseInt(denominator);
  }

  saveTranscodeInfo(key: string, info: TranscodeInfo) {
    this.redisClient.saveTranscodeStatus(key, info, this.redisTtl);
  }
}
