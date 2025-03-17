// Minimal implementation of an encore job
export type EncoreJob = {
  externalId?: string;
  profile: string;
  id?: string;
  outputFolder: string;
  baseName: string;
  progressCallbackUri: string;
  inputs: InputFile[];
  status?: EncoreStatus;
  output?: Output[];
};

export enum EncoreStatus {
  NEW = 'NEW',
  IN_PROGRESS = 'IN_PROGRESS',
  QUEUED = 'QUEUED',
  SUCCESSFUL = 'SUCCESSFUL',
  FAILED = 'FAILED',
  CANCELLED = 'CANCELLED'
}

export type Output = {
  type: string;
  format: string;
  file: string;
  fileSize: number;
  overallBitrate: number;
  videoStreams?: VideoStream[];
  audioStreams?: AudioStream[];
};

export type VideoStream = {
  codec: string;
  width: number;
  height: number;
  frameRate: string;
};

export type AudioStream = {
  codec: string;
  channels: number;
  samplingRate: number;
  profile?: string;
};

export type InputFile = {
  uri: string;
  seekTo: number;
  copyTs: boolean;
  type: InputType;
};

export enum InputType {
  AUDIO = 'Audio',
  VIDEO = 'Video',
  AUDIO_VIDEO = 'AudioVideo'
}
