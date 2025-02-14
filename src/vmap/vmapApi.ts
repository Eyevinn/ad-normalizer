import { FastifyPluginCallback } from 'fastify';
import { default as PathUtils } from 'path';
import { Static, Type } from '@sinclair/typebox';
import fastifyAcceptsSerializer from '@fastify/accepts-serializer';
import { XMLParser, XMLBuilder } from 'fast-xml-parser';
import logger from '../util/logger';
import { IN_PROGRESS } from '../redis/redisclient';

export const VmapManifestAsset = Type.Object({
    creativeId: Type.String(),
    masterPlaylistUrl: Type.String()
});

export const VmapManifestResponse = Type.Object({
    assets: Type.Array(VmapManifestAsset),
    vmapXml: Type.String({
        description: 'Original VMAP XML received from adserver'
    })
});

export type VmapManifestAsset = Static<typeof VmapManifestAsset>;
export type VmapManifestResponse = Static<typeof VmapManifestResponse>;

export interface VmapApiOptions {
    adServerUrl: string;
    assetServerUrl: string;
    lookUpAsset: (mediaFile: string) => Promise<string | null | undefined>;
    onMissingAsset?: (asset: VmapManifestAsset) => Promise<Response>;
    setupNotification?: (asset: VmapManifestAsset) => void;
}

// Always treat these paths as arrays when parsing XML
const alwaysArray = ['vmap:VMAP.vmap:AdBreak', 'vmap:AdBreak.vmap:AdSource'];

const isArray = (
    name: string,
    jpath: string,
    isLeafNode: boolean,
    isAttribute: boolean
): boolean => {
    return alwaysArray.includes(jpath);
};

export const vmapApi: FastifyPluginCallback<VmapApiOptions> = (
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

    fastify.get<{ Reply: Static<typeof VmapManifestResponse> }>(
        '/api/v1/vmap',
        {
            config: {
                serializers: [
                    {
                        regex: /^application\/xml/,
                        serializer: (data: VmapManifestResponse) => {
                            return replaceMediaFiles(data.vmapXml, data.assets);
                        }
                    }
                ]
            },
            schema: {
                description:
                    'Queries ad server for VMAP and returns manifest URLs for creatives with transcoded assets',
                response: {
                    200: VmapManifestResponse
                }
            }
        },
        async (req, reply) => {
            const path = req.url;
            const vmapStr = await getVmapXml(opts.adServerUrl, path);
            const vmapXml = parseVmap(vmapStr);
            const response = await findMissingAndDispatchJobs(vmapXml, opts);
            reply.send(response);
        }
    );

    fastify.post<{ Body: XMLDocument }>(
        '/api/v1/vmap',
        {
            config: {
                serializers: [
                    {
                        regex: /^application\/xml/,
                        serializer: (data: VmapManifestResponse) => {
                            return replaceMediaFiles(data.vmapXml, data.assets);
                        }
                    }
                ]
            },
            schema: {
                description:
                    'Accepts VMAP XML and returns data containing manifest URLs for creatives with transcoded assets.',
                response: {
                    200: VmapManifestResponse
                }
            }
        },
        async (req, reply) => {
            const vmapXml = req.body;
            const response = await findMissingAndDispatchJobs(vmapXml, opts);
            reply.send(response);
        }
    );
    next();
};

const partitionCreatives = async (
    creatives: VmapManifestAsset[],
    lookUpAsset: (mediaFile: string) => Promise<string | null | undefined>
): Promise<VmapManifestAsset[][]> => {
    const [found, missing]: [VmapManifestAsset[], VmapManifestAsset[]] = [[], []];
    for (const creative of creatives) {
        const asset = await lookUpAsset(creative.creativeId);
        logger.debug('Looking up asset', { creative, asset });
        if (asset) {
            if (asset !== IN_PROGRESS) {
                found.push({
                    creativeId: creative.creativeId,
                    masterPlaylistUrl: asset
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
    vmapXmlObj: any,
    opts: VmapApiOptions
): Promise<VmapManifestResponse> => {
    const creatives = await getCreatives(vmapXmlObj);
    const [found, missing] = await partitionCreatives(
        creatives,
        opts.lookUpAsset
    );

    logger.debug('Partitioned creatives', { found, missing });

    missing.forEach(async (creative) => {
        if (opts.onMissingAsset) {
            opts
                .onMissingAsset(creative)
                .then((response) => {
                    if (!response.ok) {
                        throw new Error(`Failed to submit job: ${response.statusText}`);
                    }
                    return response.json();
                })
                .then((data: any) => {
                    logger.info('Submitted transcode job', { jobId: data.id, creative });
                    if (opts.setupNotification) {
                        opts.setupNotification(creative);
                    }
                })
                .catch((error) => {
                    logger.error('Failed to handle missing asset', error);
                });
        }
    });

    const withBaseUrl = found.map((asset: VmapManifestAsset) => {
        return {
            creativeId: asset.creativeId,
            masterPlaylistUrl: PathUtils.join(
                opts.assetServerUrl,
                asset.masterPlaylistUrl
            )
        };
    });

    const builder = new XMLBuilder({ format: true, ignoreAttributes: false });
    const vmapXml = builder.build(vmapXmlObj);
    return { assets: withBaseUrl, vmapXml: vmapXml };
};

const getVmapXml = async (
    adServerUrl: string,
    path: string
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
                'Content-Type': 'application/xml'
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

export const getMediaFile = (vastAd: any): Record<string, string> => {
    const mediaFiles = vastAd.InLine.Creatives.Creative.Linear.MediaFiles.MediaFile;
    const mediaFileArray = Array.isArray(mediaFiles) ? mediaFiles : [mediaFiles];
    let highestBitrateMediaFile = mediaFileArray[0];
    for (const mediaFile of mediaFileArray) {
        const currentBitrate = parseInt(mediaFile['@_bitrate'] || '0');
        const highestBitrate = parseInt(highestBitrateMediaFile['@_bitrate'] || '0');
        if (currentBitrate > highestBitrate) {
            highestBitrateMediaFile = mediaFile;
        }
    }
    return highestBitrateMediaFile;
};

export const getCreatives = async (vmapXml: any): Promise<VmapManifestAsset[]> => {
    try {
        const creatives: VmapManifestAsset[] = [];
        if (vmapXml['vmap:VMAP']['vmap:AdBreak']) {
            for (const adBreak of vmapXml['vmap:VMAP']['vmap:AdBreak']) {
                if (adBreak['vmap:AdSource']?.['vast:VAST']?.Ad) {
                    const vastAds = Array.isArray(adBreak['vmap:AdSource']['vast:VAST'].Ad)
                        ? adBreak['vmap:AdSource']['vast:VAST'].Ad
                        : [adBreak['vmap:AdSource']['vast:VAST'].Ad];

                    for (const vastAd of vastAds) {
                        const adId = vastAd.InLine.Creatives.Creative.UniversalAdId['#text'].replace(
                            /[^a-zA-Z0-9]/g,
                            ''
                        );
                        const mediaFile: Record<string, string> = getMediaFile(vastAd);
                        const mediaFileUrl = mediaFile['#text'];
                        creatives.push({ creativeId: adId, masterPlaylistUrl: mediaFileUrl });
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

export const replaceMediaFiles = (vmapXml: string, assets: VmapManifestAsset[]): string => {
    try {
        const parser = new XMLParser({ ignoreAttributes: false, isArray: isArray });
        const parsedVMAP = parser.parse(vmapXml);
        if (parsedVMAP['vmap:VMAP']['vmap:AdBreak']) {
            for (const adBreak of parsedVMAP['vmap:VMAP']['vmap:AdBreak']) {
                if (adBreak['vmap:AdSource']?.['vast:VAST']?.Ad) {
                    const vastAds = Array.isArray(adBreak['vmap:AdSource']['vast:VAST'].Ad)
                        ? adBreak['vmap:AdSource']['vast:VAST'].Ad
                        : [adBreak['vmap:AdSource']['vast:VAST'].Ad];

                    adBreak['vmap:AdSource']['vast:VAST'].Ad = vastAds.reduce((acc: any[], vastAd: any) => {
                        const universalAdId = vastAd.InLine.Creatives.Creative.UniversalAdId;
                        const adId = (typeof universalAdId === 'string' ? universalAdId : universalAdId['#text']).replace(
                            /[^a-zA-Z0-9]/g,
                            ''
                        );
                        const asset = assets.find((a) => a.creativeId === adId);
                        if (asset) {
                            const mediaFile: Record<string, string> = getMediaFile(vastAd);
                            mediaFile['#text'] = asset.masterPlaylistUrl;
                            mediaFile['@_type'] = 'application/x-mpegURL';
                            vastAd.InLine.Creatives.Creative.Linear.MediaFiles.MediaFile = mediaFile;
                            acc.push(vastAd);
                        }
                        return acc;
                    }, []);
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

const parseVmap = (vmapXml: string): any => {
    try {
        const parser = new XMLParser({ ignoreAttributes: false, isArray: isArray });
        return parser.parse(vmapXml);
    } catch (error) {
        logger.error('Failed to parse VMAP XML', { error });
        return {};
    }
};
