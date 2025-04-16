import { FastifyPluginCallback } from 'fastify';
import { Static, Type } from '@sinclair/typebox';
import fastifyAcceptsSerializer from '@fastify/accepts-serializer';
import { XMLParser, XMLBuilder } from 'fast-xml-parser';
import logger from '../util/logger';
import { timestampToSeconds } from '../util/time';
import { TranscodeInfo, TranscodeStatus } from '../data/transcodeinfo';
import { EncoreService } from '../encore/encoreservice';
import { getHeaderValue } from '../util/headers';

export const deviceUserAgentHeader = 'X-Device-User-Agent';

export const ManifestAsset = Type.Object({
  creativeId: Type.String(),
  masterPlaylistUrl: Type.String()
});

export const ManifestResponse = Type.Object({
  assets: Type.Array(ManifestAsset),
  xml: Type.String({
    description: 'Original VAST/VMAP XML received from adserver'
  })
});

export const AssetDescription = Type.Object({
  URI: Type.String(),
  DURATION: Type.Number()
});

// Representation of the expected response to an X-ASSET-LIST url provided in an HLS interstitial tag
export const InterstitialResponse = Type.Object({
  ASSETS: Type.Array(AssetDescription)
});

export type ManifestAsset = Static<typeof ManifestAsset>;
export type ManifestResponse = Static<typeof ManifestResponse>;

export type AssetDescription = Static<typeof AssetDescription>;
export type InterstitialResponse = Static<typeof InterstitialResponse>;

export interface AdApiOptions {
  adServerUrl: string;
  assetServerUrl: string;
  keyField: string;
  keyRegex: RegExp;
  encoreService: EncoreService;
  lookUpAsset: (mediaFile: string) => Promise<TranscodeInfo | null | undefined>;
  onMissingAsset?: (
    asset: ManifestAsset
  ) => Promise<TranscodeInfo | null | undefined>;
}

export const AlwaysArray = [
  'VAST.Ad',
  'vmap:VMAP.vmap:AdBreak',
  'vmap:AdBreak.vmap:AdSource'
];

export const isArray = (
  name: string,
  jpath: string,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  isLeafNode: boolean,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  isAttribute: boolean
): boolean => {
  return AlwaysArray.includes(jpath);
};

export interface VastAd {
  InLine: {
    Creatives: {
      Creative: {
        UniversalAdId: {
          '#text': string;
          '@_idRegistry': string;
        };
        Linear: {
          MediaFiles: VastAdMediaFiles;
          Duration: string;
        };
      };
    };
  };
}

interface VastXml {
  VAST: {
    Ad?: VastAd[];
  };
}

interface VastAdMediaFiles {
  MediaFile: MediaFile | MediaFile[];
}

export interface MediaFile {
  '#text': string;
  '@_type': string;
  '@_bitrate'?: string;
  '@_width': string;
  '@_height': string;
  '@_codec'?: string;
  '@_id'?: string;
  '@_delivery': string;
}

export const vastApi: FastifyPluginCallback<AdApiOptions> = (
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
        const parsed = parseVast(body.toString());
        done(null, parsed);
      } catch (error) {
        logger.error('Failed to parse VAST XML', error);
        done(new Error('Failed to parse VAST XML'), undefined);
      }
    }
  );
  fastify.get<{ Reply: Static<typeof ManifestResponse> }>(
    '/api/v1/vast',
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
          },
          {
            regex: /^application\/json/,
            serializer: (data: ManifestResponse) => {
              return createAssetList(
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
          'Queries ad server for creatives and returns manifest URLs for creatives with transcoded assets',
        response: {
          200: ManifestResponse
        }
      }
    },
    async (req, reply) => {
      const path = req.url;
      const headers = req.headers;
      let vastReqHeaders = {};
      const deviceUserAgent = getHeaderValue(headers, deviceUserAgentHeader);
      const forwardedFor = getHeaderValue(headers, 'X-Forwarded-For');
      if (deviceUserAgent) {
        vastReqHeaders = {
          ...vastReqHeaders,
          [deviceUserAgentHeader]: deviceUserAgent
        };
      }
      if (forwardedFor) {
        vastReqHeaders = { ...vastReqHeaders, 'X-Forwarded-For': forwardedFor };
      }
      const vastStr = await getVastXml(opts.adServerUrl, path, vastReqHeaders);
      const vastXml = parseVast(vastStr);
      const response = await findMissingAndDispatchJobs(vastXml, opts);
      reply.send(response);
      return reply;
    }
  );

  fastify.post<{ Body: VastXml }>(
    '/api/v1/vast',
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
          },
          {
            regex: /^application\/json/,
            serializer: (data: ManifestResponse) => {
              return createAssetList(
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
          'Accepts VAST XML and returns data containing manifest URLs for creatives with transcoded assets.',

        response: {
          200: ManifestResponse
        }
      }
    },
    async (req, reply) => {
      const vastXml = req.body;
      const response = await findMissingAndDispatchJobs(vastXml, opts);
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
  vastXmlObj: VastXml,
  opts: AdApiOptions
): Promise<ManifestResponse> => {
  const creatives = await getCreatives(
    vastXmlObj,
    opts.keyField,
    opts.keyRegex
  );
  const [found, missing] = await partitionCreatives(
    creatives,
    opts.lookUpAsset
  );
  logger.debug('Partitioned creatives', { found, missing });
  logger.debug('Received creatives', { creatives });
  logger.debug('Received VAST request');
  missing.forEach(async (creative) => {
    if (opts.onMissingAsset) {
      opts
        ?.onMissingAsset(creative)
        .then((data: TranscodeInfo | null | undefined) => {
          if (!data) {
            logger.error(
              "Encore job missing external ID, we won't be able to keep track of it!"
            );
          } else {
            logger.info('Submitted encore job', { creative });
          }
        })
        .catch((error) => {
          logger.error('Failed to handle missing asset', error);
          throw new Error('Failed to submit encore job');
        });
    }
  });
  const builder = new XMLBuilder({ format: true, ignoreAttributes: false });
  const vastXml = builder.build(vastXmlObj);
  return { assets: found, xml: vastXml };
};

const getVastXml = async (
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
    logger.info(`Fetching VAST request from ${url.toString()}`);
    const response = await fetch(url, {
      method: 'GET',
      headers: {
        ...headers,
        'Content-Type': 'application/xml'
      }
    });
    if (!response.ok) {
      throw new Error('Response from ad server was not OK');
    }
    return await response.text();
  } catch (error) {
    logger.error('Failed to fetch VAST request', { error });
    return `<?xml version="1.0" encoding="utf-8"?><VAST version="4.0"/>`;
  }
};

const getCreatives = async (
  vastXml: VastXml,
  keyField: string,
  keyRegex: RegExp
): Promise<ManifestAsset[]> => {
  try {
    if (vastXml.VAST.Ad) {
      const creatives = vastXml.VAST.Ad.reduce(
        (acc: ManifestAsset[], ad: VastAd) => {
          const adId = getKey(keyField, keyRegex, ad);
          const mediaFile = getBestMediaFileFromVastAd(ad);
          return [
            ...acc,
            { creativeId: adId, masterPlaylistUrl: mediaFile['#text'] }
          ];
        },
        []
      );
      return creatives;
    }
    return [];
  } catch (error) {
    logger.error('Failed to parse VAST XML', error);
    return [];
  }
};

const replaceMediaFiles = (
  vastXml: string,
  assets: ManifestAsset[],
  keyRegex: RegExp,
  keyField: string
): string => {
  try {
    const parser = new XMLParser({ ignoreAttributes: false, isArray: isArray });
    const parsedVAST = parser.parse(vastXml);
    if (parsedVAST.VAST.Ad) {
      const vastAds = Array.isArray(parsedVAST.VAST.Ad)
        ? parsedVAST.VAST.Ad
        : [parsedVAST.VAST.Ad];

      parsedVAST.VAST.Ad = vastAds.reduce((acc: VastAd[], vastAd: VastAd) => {
        const adId = getKey(keyField, keyRegex, vastAd);
        const asset = assets.find((a) => a.creativeId === adId);
        if (asset) {
          const mediaFile = getBestMediaFileFromVastAd(vastAd);
          mediaFile['#text'] = asset.masterPlaylistUrl;
          mediaFile['@_type'] = 'application/x-mpegURL';
          vastAd.InLine.Creatives.Creative.Linear.MediaFiles.MediaFile =
            mediaFile;
          acc.push(vastAd);
        }
        return acc;
      }, []);
    }

    const builder = new XMLBuilder({ format: true, ignoreAttributes: false });
    return builder.build(parsedVAST);
  } catch (error) {
    logger.error('Failed to replace media files in VAST', error);
    return vastXml;
  }
};

export const getBestMediaFileFromVastAd = (vastAd: VastAd): MediaFile => {
  const mediaFiles =
    vastAd.InLine.Creatives.Creative.Linear.MediaFiles.MediaFile;
  const mediaFileArray = Array.isArray(mediaFiles) ? mediaFiles : [mediaFiles];
  let highestBitrateMediaFile = mediaFileArray[0];
  for (const mediaFile of mediaFileArray) {
    const currentBitrate = parseInt(mediaFile['@_bitrate'] || '0');
    const highestBitrate = parseInt(
      highestBitrateMediaFile['@_bitrate'] || '0'
    );
    if (currentBitrate > highestBitrate) {
      highestBitrateMediaFile = mediaFile;
    }
  }
  return highestBitrateMediaFile;
};

const parseVast = (vastXml: string) => {
  try {
    const parser = new XMLParser({ ignoreAttributes: false, isArray: isArray });
    const parsedVAST = parser.parse(vastXml);
    return parsedVAST;
  } catch (error) {
    logger.error('Failed to parse VAST XML', { error });
    return {};
  }
};

const createAssetList = (
  vastXml: string,
  assets: ManifestAsset[],
  keyRegex: RegExp,
  keyField: string
) => {
  let assetDescriptions = [];
  try {
    const parser = new XMLParser({ ignoreAttributes: false, isArray: isArray });
    const parsedVAST = parser.parse(vastXml);
    if (parsedVAST.VAST.Ad) {
      assetDescriptions = parsedVAST.VAST.Ad.map((ad: VastAd) => {
        const adId = getKey(keyField, keyRegex, ad);
        const asset = assets.find((asset) => asset.creativeId === adId);
        if (asset) {
          return {
            URI: asset.masterPlaylistUrl,
            DURATION: timestampToSeconds(
              ad.InLine.Creatives.Creative.Linear.Duration
            )
          };
        }
        // filter out assets that don't have a corresponding creative
      }).filter((asset: AssetDescription | undefined) => asset !== undefined);
    }
  } catch (error) {
    const fallbackDuration = 10;
    logger.error('Failed to create asset list', error);
    assetDescriptions = assets.map((asset) => {
      return {
        URI: asset.masterPlaylistUrl,
        DURATION: fallbackDuration
      };
    });
  }
  return JSON.stringify({
    ASSETS: assetDescriptions
  } as InterstitialResponse);
};

export const getKey = (
  keyString: string,
  keyRegex: RegExp,
  vastAd: VastAd
): string => {
  switch (keyString) {
    case 'resolution':
      return (
        getBestMediaFileFromVastAd(vastAd)['@_width'] +
        'x' +
        getBestMediaFileFromVastAd(vastAd)['@_height']
      );
    case 'url':
      return getBestMediaFileFromVastAd(vastAd)['#text'].replace(keyRegex, '');
    default:
      return vastAd.InLine.Creatives.Creative.UniversalAdId['#text'].replace(
        keyRegex,
        ''
      );
  }
};
