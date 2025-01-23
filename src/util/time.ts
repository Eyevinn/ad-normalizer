export const timestampToSeconds = (timestamp: string): number => {
  const [hours, minutes, seconds] = timestamp.split(':').map(Number);
  return hours * 3600 + minutes * 60 + seconds;
};
