import { FastifyPluginCallback } from 'fastify';
import { Static, Type } from '@sinclair/typebox';
import { XMLParser } from 'fast-xml-parser';
import logger from '../util/logger';

export const ManifestAsset = Type.Object({
  creativeId: Type.String(),
  masterPlaylistUrl: Type.String()
});

export const ManifestResponse = Type.Object({
  assets: Type.Array(ManifestAsset)
});

export type ManifestAsset = Static<typeof ManifestAsset>;
export type ManifestResponse = Static<typeof ManifestResponse>;

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
  fastify.get<{ Reply: Static<typeof ManifestResponse> }>(
    '/api/v1/vast',
    {
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
      const creatives = await getCreatives(opts.adServerUrl, path);
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
                logger.error('Failed to submit encore job', {
                  code,
                  reason,
                  url
                });
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
      reply.send({ assets: withBaseUrl } as ManifestResponse); // TODO: add endpoint here!
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
const getCreatives = async (
  adServerUrl: string,
  path: string
): Promise<ManifestAsset[]> => {
  const url = adServerUrl + path;
  logger.info('Fetching VAST request from', { adServerUrl, path, url });
  return fetch(url, {
    method: 'GET',
    headers: {
      'Content-Type': 'application/xml'
    }
  })
    .then((response) => {
      if (!response.ok) {
        throw new Error('Response from ad server was not OK');
      }
      const contentType = response.headers.get('content-type');
      logger.debug('Received response from ad server', { contentType });
      return response.text();
    })
    .then((body) => {
      const parser = new XMLParser();
      const parsedVAST = parser.parse(body);
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
    })
    .catch((error) => {
      logger.error('Failed to fetch VAST request', { error });
      return [];
    });
};
