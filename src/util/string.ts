import { default as PathUtils } from 'path';

export const removeTrailingSlash = (url: string): string => {
  return url.endsWith('/') ? url.slice(0, -1) : url;
};
export const createPackageUrl = (
  assetServerUrl: string,
  outputFolder: string,
  baseName: string
): string => {
  const parsedAssetServerUrl = new URL(assetServerUrl);
  return new URL(
    PathUtils.join(
      parsedAssetServerUrl.pathname,
      outputFolder,
      baseName + '.m3u8'
    ),
    assetServerUrl
  ).href;
};

export const createOutputUrl = (bucket: URL, folder: string): string | null => {
  try {
    return new URL(PathUtils.join(bucket.pathname, folder), bucket).href + '/';
  } catch (e) {
    return null;
  }
};

export const replaceSubDomain = (url: URL, newSubDomain: string): URL => {
  const hostParts = url.hostname.split('.');
  if (hostParts.length > 2) {
    hostParts[0] = newSubDomain;
    url.hostname = hostParts.join('.');
  } else {
    url.hostname = newSubDomain + '.' + url.hostname;
  }
  return url;
};
