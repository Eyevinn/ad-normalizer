import { timestampToSeconds } from './time';

describe('time utils', () => {
  it('deserializes timestamps correctly', () => {
    let timestamp = '00:00:15';
    let parsed = timestampToSeconds(timestamp);
    expect(parsed).toBe(15);

    timestamp = '00:01:00';
    parsed = timestampToSeconds(timestamp);
    expect(parsed).toBe(60);

    timestamp = '01:00:00';
    parsed = timestampToSeconds(timestamp);
    expect(parsed).toBe(3600);

    timestamp = '00:01:45';
    parsed = timestampToSeconds(timestamp);
    expect(parsed).toBe(105);
  });
});
