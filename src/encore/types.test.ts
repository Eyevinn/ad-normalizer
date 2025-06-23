import { EncoreJob, InputFile, InputType } from "./types";

const testJob: EncoreJob = {
  externalId: "123",
  profile: "ad",
  outputFolder: "test",
  baseName: "ad",
  progressCallbackUri: "http://localhost:3000",
  inputs: [
    {
      uri: "test",
      seekTo: 0,
      copyTs: true,
      type: InputType.AUDIO_VIDEO,
    } as InputFile,
  ],
};

describe("EncoreJob", () => {
  it("should serialize to JSON correctly", () => {
    const expected =
      '{"externalId":"123","profile":"ad","outputFolder":"test","baseName":"ad","progressCallbackUri":"http://localhost:3000","inputs":[{"uri":"test","seekTo":0,"copyTs":true,"type":"AudioVideo"}]}';
    const serialized = JSON.stringify(testJob);
    expect(serialized).toEqual(expected);
  });
});
