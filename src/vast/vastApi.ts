import fastify, { FastifyPluginCallback } from 'fastify';
import { Static, Type } from '@sinclair/typebox';
import fastifyAcceptsSerializer from '@fastify/accepts-serializer';
import { XMLParser, XMLBuilder } from 'fast-xml-parser';
import logger from '../util/logger';
import { serialize } from 'v8';
import { timestampToSeconds } from '../util/time';

export const ManifestAsset = Type.Object({
  creativeId: Type.String(),
  masterPlaylistUrl: Type.String()
  // TODO: Figure out how to handle durations
});

export const ManifestResponse = Type.Object({
  assets: Type.Array(ManifestAsset),
  vastXml: Type.String({
    description: 'Original VAST XML received from adserver'
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
  lookUpAsset: (mediaFile: string) => Promise<string | null | undefined>;
  onMissingAsset?: (asset: ManifestAsset) => Promise<Response>;
  setupNotification?: (asset: ManifestAsset) => void;
}

export const vastApi: FastifyPluginCallback<AdApiOptions> = (
  fastify,
  opts,
  next
) => {
  fastify.register(fastifyAcceptsSerializer);

  fastify.get<{ Reply: Static<typeof ManifestResponse> }>(
    '/api/v1/vast',
    {
      config: {
        serializers: [
          {
            regex: /^application\/xml/,
            serializer: (data: ManifestResponse) => {
              return replaceMediaFiles(data.vastXml, data.assets);
            }
          },
          {
            regex: /^application\/json/,
            serializer: (data: ManifestResponse) => {
              return createAssetList(data.vastXml, data.assets);
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
      const response = await fetchVastAndDispatchJobs(path, opts);
      reply.send(response);
    }
  );
  next();
};

const partitionCreatives = async (
  creatives: ManifestAsset[],
  lookUpAsset: (mediaFile: string) => Promise<string | null | undefined>
): Promise<ManifestAsset[][]> => {
  const [found, missing]: [ManifestAsset[], ManifestAsset[]] = [[], []];
  for (const creative of creatives) {
    const asset = await lookUpAsset(creative.creativeId);
    logger.debug('Looking up asset', { creative, asset });
    if (asset) {
      found.push({ creativeId: creative.creativeId, masterPlaylistUrl: asset });
    } else {
      missing.push({
        creativeId: creative.creativeId,
        masterPlaylistUrl: creative.masterPlaylistUrl
      });
    }
  }
  return [found, missing];
};

const fetchVastAndDispatchJobs = async (
  path: string,
  opts: AdApiOptions
): Promise<ManifestResponse> => {
  const vastXml = await getVastXml(opts.adServerUrl, path);
  const creatives = await getCreatives(vastXml);
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
        .then((response) => {
          if (!response.ok) {
            const code = response.status;
            const url = response.url;
            const reason = response.statusText;
            if (code == 401) {
              logger.error(
                'Encore returned status code 401 Unauthorized. Check that your service access token is still valid.'
              );
            } else {
              logger.error('Failed to submit encore job', {
                code,
                reason,
                url
              });
            }
            throw new Error('Failed to submit encore job');
          }
          return response.json();
        })
        .then((data) => {
          const encoreJobId = data.id;
          logger.info('Submitted encore job', { encoreJobId, creative });
          if (opts.setupNotification) {
            logger.debug('Setting up notification');
            opts.setupNotification(creative);
            logger.debug("Notification set up. You're good to go!");
          }
        })
        .catch((error) => {
          logger.error('Failed to handle missing asset', error);
        });
    }
  });
  const withBaseUrl = found.map((asset: ManifestAsset) => {
    return {
      creativeId: asset.creativeId,
      masterPlaylistUrl: opts.assetServerUrl + '/' + asset.masterPlaylistUrl
    };
  });
  return { assets: withBaseUrl, vastXml };
};

const getVastXml = async (
  adServerUrl: string,
  path: string
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

const getCreatives = async (vastXml: string): Promise<ManifestAsset[]> => {
  try {
    const parser = new XMLParser();
    const parsedVAST = parser.parse(vastXml);
    if (parsedVAST.VAST.Ad) {
      const creatives = parsedVAST.VAST.Ad.reduce(
        (acc: ManifestAsset[], ad: any) => {
          const adId = ad.InLine.Creatives.Creative.UniversalAdId.replace(
            /[^a-zA-Z0-9]/g,
            ''
          );
          const mediaFile =
            ad.InLine.Creatives.Creative.Linear.MediaFiles.MediaFile;
          return [...acc, { creativeId: adId, masterPlaylistUrl: mediaFile }];
        },
        []
      );
      return creatives;
    }
    return [];
  } catch (error) {
    logger.error('Failed to parse VAST XML', { error });
    return [];
  }
};

const replaceMediaFiles = (vastXml: string, assets: ManifestAsset[]) => {
  try {
    const parser = new XMLParser({ ignoreAttributes: false });
    const parsedVAST = parser.parse(vastXml);
    if (parsedVAST.VAST.Ad) {
      for (const ad of parsedVAST.VAST.Ad) {
        const adId = ad.InLine.Creatives.Creative.UniversalAdId[
          '#text'
        ].replace(/[^a-zA-Z0-9]/g, '');
        const asset = assets.find((asset) => asset.creativeId === adId);
        if (asset) {
          ad.InLine.Creatives.Creative.Linear.MediaFiles.MediaFile['#text'] =
            asset.masterPlaylistUrl;
          ad.InLine.Creatives.Creative.Linear.MediaFiles.MediaFile['@_type'] =
            'application/x-mpegURL';
        }
      }
    }
    const builder = new XMLBuilder({ format: true, ignoreAttributes: false });
    const modifiedVastXml = builder.build(parsedVAST);
    return modifiedVastXml;
  } catch (error) {
    logger.error('Failed to replace media files', { error });
    return vastXml;
  }
};

const createAssetList = (vastXml: string, assets: ManifestAsset[]) => {
  let assetDescriptions = [];
  try {
    const parser = new XMLParser({ ignoreAttributes: false });
    const parsedVAST = parser.parse(vastXml);

    if (parsedVAST.VAST.Ad) {
      assetDescriptions = parsedVAST.VAST.Ad.map((ad: any) => {
        const adId = ad.InLine.Creatives.Creative.UniversalAdId[
          '#text'
        ].replace(/[^a-zA-Z0-9]/g, '');
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
    logger.error('Failed to create asset list', { error });
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
