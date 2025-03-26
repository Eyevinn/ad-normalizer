import { default as PathUtils } from 'path';

export const removeTrailingSlash = (url: string): string => {
  return url.endsWith('/') ? url.slice(0, -1) : url;
};
export const createPackageUrl = (
  assetServerUrl: string,
  outputFolder: string,
  baseName: string
): string => {
  return new URL(
    PathUtils.join(outputFolder, baseName + '.m3u8'),
    assetServerUrl
  ).href;
};

export const createOutputUrl = (bucket: string, folder: string): string | null => {
  const parsedUrl = URL.parse(folder, bucket);
  return parsedUrl ? parsedUrl.href : null;
}
