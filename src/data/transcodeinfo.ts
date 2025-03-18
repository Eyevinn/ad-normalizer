export type TranscodeInfo = {
  url: string;
  aspectRatio: string;
  framerates: number[]; // Not sure we can get this information, maybe by parsing the multivariant playlist
  status: TranscodeStatus;
};

export enum TranscodeStatus {
  COMPLETED = 'COMPLETED',
  FAILED = 'FAILED',
  IN_PROGRESS = 'IN_PROGRESS',
  TRANSCODING = 'TRANSCODING',
  PACKAGING = 'PACKAGING',
  UNKNOWN = 'UNKNOWN'
}

export type JobProgress = {
  jobId: string;
  externalId: string;
  progress: number;
  status: string; // Encore job status is different from our internal representation
};

// TODO: Refactor to populate a lot of stuff here
export const JobProgressToTranscodeStatus = (
  job: JobProgress
): TranscodeInfo => {
  const info = {} as TranscodeInfo;
  switch (job.status) {
    case 'SUCCESSFUL':
      info.status = TranscodeStatus.COMPLETED;
      break;
    case 'FAILED':
      info.status = TranscodeStatus.FAILED;
      break;
    case 'IN_PROGRESS':
      info.status = TranscodeStatus.IN_PROGRESS;
      break;
    default:
      info.status = TranscodeStatus.TRANSCODING;
  }
  return info;
};
