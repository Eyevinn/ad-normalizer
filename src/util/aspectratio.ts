export const calculateAspectRatio = (width: number, height: number): string => {
  const divisor = gcd(width, height);
  return `${width / divisor}:${height / divisor}`;
};

const gcd = (a: number, b: number): number => {
  while (b != 0) {
    const t = b;
    b = a % b;
    a = t;
  }
  return a;
};
