import { IncomingHttpHeaders } from 'http';

// Gets the value of a header from the request
// If the value doesn't exist, it returns undefined
// If there are multiple values for the key, it returns the first one
export const getHeaderValue = (
  headers: IncomingHttpHeaders,
  key: string
): string | undefined => {
  if (headers[key]) {
    const value = headers[key];
    if (Array.isArray(value)) {
      return value[0];
    }
    return value;
  }
  return undefined;
};
