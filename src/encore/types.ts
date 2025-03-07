// Minimal implementation of an encore job
export type EncoreJob = {
  externalId: string;
  profile: string;
  outputFolder: string;
  baseName: string;
  progressCallbackUri: string;
  inputs: InputFile[];
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
