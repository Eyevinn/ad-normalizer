import { TranscodeInfo, TranscodeStatus } from './transcodeinfo';

describe('data serialization behavior', () => {
  it('should serialize transcode data correctly', () => {
    const transcodeInfo: TranscodeInfo = {
      url: 'http://example.com',
      aspectRatio: '16:9',
      framerates: [25, 50],
      status: TranscodeStatus.COMPLETED
    };
    const serialized = JSON.stringify(transcodeInfo);
    expect(serialized).toEqual(
      '{"url":"http://example.com","aspectRatio":"16:9","framerates":[25,50],"status":"COMPLETED"}'
    );
  });
  it('should deserialize transcode data correctly', () => {
    const serialized =
      '{"url":"http://example.com","aspectRatio":"16:9","framerates":[25,50],"status":"COMPLETED"}';
    const transcodeInfo: TranscodeInfo = JSON.parse(serialized);
    expect(transcodeInfo.aspectRatio).toEqual('16:9');
    expect(transcodeInfo.framerates).toEqual([25, 50]);
    expect(transcodeInfo.status).toEqual(TranscodeStatus.COMPLETED);
  });
});
