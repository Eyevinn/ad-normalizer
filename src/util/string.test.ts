import { createPackageUrl, replaceSubDomain } from './string';

describe('string utils', () => {
  it('can create a package url with asset server url', () => {
    const assetServerUrl = 'http://asset-server-url';
    const outputFolder = 'output-folder';
    const baseName = 'base-name';
    const expectedUrl = 'http://asset-server-url/output-folder/base-name.m3u8';
    expect(createPackageUrl(assetServerUrl, outputFolder, baseName)).toEqual(
      expectedUrl
    );
  });

  it('can create a package url with asset server url including path', () => {
    const assetServerUrl = 'http://asset-server-url/path';
    const outputFolder = 'output-folder';
    const baseName = 'base-name';
    const expectedUrl =
      'http://asset-server-url/path/output-folder/base-name.m3u8';
    expect(createPackageUrl(assetServerUrl, outputFolder, baseName)).toEqual(
      expectedUrl
    );
  });

  it('can replace subdomain', () => {
    const url = new URL('http://old-subdomain.example.com/path');
    const newSubDomain = 'new-subdomain';
    const expectedUrl = 'http://new-subdomain.example.com/path';
    expect(replaceSubDomain(url, newSubDomain).href).toEqual(expectedUrl);
  });
});
