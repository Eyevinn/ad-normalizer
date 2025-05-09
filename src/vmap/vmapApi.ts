import { FastifyPluginCallback } from 'fastify';
import { Static } from '@sinclair/typebox';
import fastifyAcceptsSerializer from '@fastify/accepts-serializer';
import { XMLParser, XMLBuilder } from 'fast-xml-parser';
import {
  ManifestAsset,
  AdApiOptions,
  ManifestResponse,
  getBestMediaFileFromVastAd,
  MediaFile,
  isArray,
  VastAd,
  getKey,
  deviceUserAgentHeader
} from '../vast/vastApi';
import logger from '../util/logger';
import { TranscodeInfo, TranscodeStatus } from '../data/transcodeinfo';
import { getHeaderValue } from '../util/headers';

interface VmapAdBreak {
  '@_breakId'?: string;
  '@_breakType'?: string;
  '@_timeOffset'?: string;
  'vmap:AdSource'?: {
    '@_id'?: string;
    'vmap:VASTAdData'?: {
      VAST: {
        '@_version'?: string;
        Ad: VastAd | VastAd[];
      };
    };
  };
}

interface VmapXmlObject {
  'vmap:VMAP': {
    'vmap:AdBreak'?: VmapAdBreak[];
  };
}

export const vmapApi: FastifyPluginCallback<AdApiOptions> = (
  fastify,
  opts,
  next
) => {
  fastify.register(fastifyAcceptsSerializer);
  fastify.addContentTypeParser(
    ['text/xml', 'application/xml'],
    { parseAs: 'string' },
    (req, body, done) => {
      try {
        const parsed = parseVmap(body.toString());
        done(null, parsed);
      } catch (error) {
        logger.error('Failed to parse VMAP XML', error);
        done(new Error('Failed to parse VMAP XML'), undefined);
      }
    }
  );

  fastify.get<{ Reply: Static<typeof ManifestResponse> }>(
    '/api/v1/vmap',
    {
      config: {
        serializers: [
          {
            regex: /^application\/xml/,
            serializer: (data: ManifestResponse) => {
              return replaceMediaFiles(
                data.xml,
                data.assets,
                opts.keyRegex,
                opts.keyField
              );
            }
          }
        ]
      },
      schema: {
        description:
          'Queries ad server for VMAP and returns manifest URLs for creatives with transcoded assets',
        response: {
          200: ManifestResponse
        }
      }
    },
    async (req, reply) => {
      const path = req.url;
      const headers = req.headers;
      const deviceUserAgent = getHeaderValue(
        headers,
        deviceUserAgentHeader.toLowerCase()
      );
      const forwardedFor = getHeaderValue(
        headers,
        'X-Forwarded-For'.toLowerCase()
      );
      let vmapReqHeaders = {};
      if (deviceUserAgent) {
        vmapReqHeaders = {
          ...vmapReqHeaders,
          [deviceUserAgentHeader]: deviceUserAgent
        };
      } else {
        logger.error('Missing device user agent header');
      }
      if (forwardedFor) {
        vmapReqHeaders = { ...vmapReqHeaders, 'X-Forwarded-For': forwardedFor };
      } else {
        logger.error('Missing X-Forwarded-For header');
      }
      const vmapStr = await getVmapXml(opts.adServerUrl, path, vmapReqHeaders);
      const vmapXml = parseVmap(vmapStr);
      const response = await findMissingAndDispatchJobs(
        vmapXml as VmapXmlObject,
        opts
      );
      reply.send(response);
      return reply;
    }
  );

  fastify.post<{ Body: VmapXmlObject }>(
    '/api/v1/vmap',
    {
      config: {
        serializers: [
          {
            regex: /^application\/xml/,
            serializer: (data: ManifestResponse) => {
              return replaceMediaFiles(
                data.xml,
                data.assets,
                opts.keyRegex,
                opts.keyField
              );
            }
          }
        ]
      },
      schema: {
        description:
          'Accepts VMAP XML and returns data containing manifest URLs for creatives with transcoded assets.',
        response: {
          200: ManifestResponse
        }
      }
    },
    async (req, reply) => {
      const vmapXml = req.body;
      const response = await findMissingAndDispatchJobs(vmapXml, opts);
      reply.send(response);
      return reply;
    }
  );
  next();
};

const partitionCreatives = async (
  creatives: ManifestAsset[],
  lookUpAsset: (mediaFile: string) => Promise<TranscodeInfo | null | undefined>
): Promise<ManifestAsset[][]> => {
  const [found, missing]: [ManifestAsset[], ManifestAsset[]] = [[], []];
  for (const creative of creatives) {
    const asset = await lookUpAsset(creative.creativeId);
    logger.debug('Looking up asset', { creative, asset });
    if (asset) {
      if (asset.status == TranscodeStatus.COMPLETED) {
        found.push({
          creativeId: creative.creativeId,
          masterPlaylistUrl: asset.url
        });
      }
    } else {
      missing.push({
        creativeId: creative.creativeId,
        masterPlaylistUrl: creative.masterPlaylistUrl
      });
    }
  }
  return [found, missing];
};

const findMissingAndDispatchJobs = async (
  vmapXmlObj: VmapXmlObject,
  opts: AdApiOptions
): Promise<ManifestResponse> => {
  const creatives = await getCreatives(
    vmapXmlObj,
    opts.keyRegex,
    opts.keyField
  );
  const [found, missing] = await partitionCreatives(
    creatives,
    opts.lookUpAsset
  );

  logger.debug('Partitioned creatives', { found, missing });

  missing.forEach(async (creative) => {
    if (opts.onMissingAsset) {
      opts
        .onMissingAsset(creative)
        .then((data: TranscodeInfo | null | undefined) => {
          if (data) {
            logger.info('Submitted encore job', {
              creative
            });
          } else {
            logger.error('Failed to submit encore job', {
              creative
            });
            throw new Error('Failed to submit encore job');
          }
        })
        .catch((error) => {
          logger.error('Failed to handle missing asset', error);
        });
    }
  });

  const builder = new XMLBuilder({ format: true, ignoreAttributes: false });
  const vmapXml = builder.build(vmapXmlObj);
  return { assets: found, xml: vmapXml };
};

export const getVmapXml = async (
  adServerUrl: string,
  path: string,
  headers: Record<string, string> = {}
): Promise<string> => {
  try {
    const url = new URL(adServerUrl);
    const params = new URLSearchParams(path.split('?')[1]);
    for (const [key, value] of params) {
      url.searchParams.append(key, value);
    }
    logger.info(`Fetching VMAP request from ${url.toString()}`);
    const response = await fetch(url, {
      method: 'GET',
      headers: {
        ...headers,
        'Content-Type': 'application/xml',
        'User-Agent': 'eyevinn/ad-normalizer'
      }
    });
    if (!response.ok) {
      throw new Error('Response from ad server was not OK');
    }
    return await response.text();
  } catch (error) {
    logger.error('Failed to fetch VMAP request', { error });
    return `<?xml version="1.0" encoding="utf-8"?><vmap:VMAP version="1.0"/>`;
  }
};

export const getCreatives = async (
  vmapXml: VmapXmlObject,
  keyRegex: RegExp,
  keyField: string
): Promise<ManifestAsset[]> => {
  try {
    const creatives: ManifestAsset[] = [];
    if (vmapXml['vmap:VMAP']['vmap:AdBreak']) {
      for (const adBreak of vmapXml['vmap:VMAP']['vmap:AdBreak']) {
        if (adBreak['vmap:AdSource']?.['vmap:VASTAdData']?.VAST.Ad) {
          const vastAds = Array.isArray(
            adBreak['vmap:AdSource']['vmap:VASTAdData'].VAST.Ad
          )
            ? adBreak['vmap:AdSource']['vmap:VASTAdData'].VAST.Ad
            : [adBreak['vmap:AdSource']['vmap:VASTAdData'].VAST.Ad];

          for (const vastAd of vastAds) {
            const adId = getKey(keyField, keyRegex, vastAd);
            const mediaFile: MediaFile = getBestMediaFileFromVastAd(vastAd);
            const mediaFileUrl = mediaFile['#text'];
            creatives.push({
              creativeId: adId,
              masterPlaylistUrl: mediaFileUrl
            });
          }
        }
      }
    }
    return creatives;
  } catch (error) {
    logger.error('Failed to parse VMAP XML', error);
    return [];
  }
};

export const replaceMediaFiles = (
  vmapXml: string,
  assets: ManifestAsset[],
  keyRegex: RegExp,
  keyField: string
): string => {
  try {
    const parser = new XMLParser({ ignoreAttributes: false, isArray: isArray });
    const parsedVMAP = parser.parse(vmapXml);
    if (parsedVMAP['vmap:VMAP']['vmap:AdBreak']) {
      for (const adBreak of parsedVMAP['vmap:VMAP']['vmap:AdBreak']) {
        if (adBreak['vmap:AdSource']?.['vmap:VASTAdData'].VAST.Ad) {
          const vastAds = Array.isArray(
            adBreak['vmap:AdSource']['vmap:VASTAdData'].VAST.Ad
          )
            ? adBreak['vmap:AdSource']['vmap:VASTAdData'].VAST.Ad
            : [adBreak['vmap:AdSource']['vmap:VASTAdData'].VAST.Ad];

          adBreak['vmap:AdSource']['vmap:VASTAdData'].VAST.Ad = vastAds.reduce(
            (acc: VastAd[], vastAd: VastAd) => {
              const adId = getKey(keyField, keyRegex, vastAd);
              const asset = assets.find((a) => a.creativeId === adId);
              if (asset) {
                const mediaFile: MediaFile = getBestMediaFileFromVastAd(vastAd);
                mediaFile['#text'] = asset.masterPlaylistUrl;
                mediaFile['@_type'] = 'application/x-mpegURL';
                vastAd.InLine.Creatives.Creative.Linear.MediaFiles.MediaFile =
                  mediaFile;
                acc.push(vastAd);
              }
              return acc;
            },
            []
          );
        }
      }
    }

    const builder = new XMLBuilder({ format: true, ignoreAttributes: false });
    return builder.build(parsedVMAP);
  } catch (error) {
    console.error('Failed to replace media files in VMAP', error);
    return vmapXml;
  }
};

const parseVmap = (vmapXml: string): VmapXmlObject | object => {
  try {
    const parser = new XMLParser({ ignoreAttributes: false, isArray: isArray });
    return parser.parse(vmapXml);
  } catch (error) {
    logger.error('Failed to parse VMAP XML', { error });
    return {
      'vmap:VMAP': {}
    };
  }
};
