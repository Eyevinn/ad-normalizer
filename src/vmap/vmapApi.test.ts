import {
  ManifestAsset,
  getBestMediaFileFromVastAd,
  VastAd
} from '../vast/vastApi';
import { XMLParser, XMLBuilder } from 'fast-xml-parser';

jest.mock('../util/logger');

describe('VMAP API', () => {
  const { replaceMediaFiles, getCreatives } = jest.requireActual('./vmapApi');

  describe('replaceMediaFiles', () => {
    it('should replace media files in VMAP XML with transcoded assets', () => {
      const vmapXml = `<?xml version="1.0" encoding="UTF-8"?>
        <vmap:VMAP xmlns:vmap="http://www.iab.net/vmap-1.0" version="1.0">
          <vmap:AdBreak timeOffset="start" breakType="linear">
            <vmap:AdSource>
              <vmap:VASTAdData xmlns:vast="http://www.iab.net/VAST" version="4.0">
                <VAST version="4.0">  
                  <Ad>
                    <InLine>
                      <Creatives>
                        <Creative>
                          <UniversalAdId idRegistry="test-registry">ad123</UniversalAdId>
                          <Linear>
                            <MediaFiles>
                              <MediaFile type="video/mp4" bitrate="2000">
                                http://example.com/original.mp4
                              </MediaFile>
                            </MediaFiles>
                          </Linear>
                        </Creative>
                      </Creatives>
                    </InLine>
                  </Ad>
                </VAST>
              </vmap:VASTAdData>
            </vmap:AdSource>
          </vmap:AdBreak>
        </vmap:VMAP>`;

      const assets: ManifestAsset[] = [
        {
          creativeId: 'ad123',
          masterPlaylistUrl: 'https://example.com/transcoded/index.m3u8'
        }
      ];

      const result = replaceMediaFiles(
        vmapXml,
        assets,
        /[^a-zA-Z0-9]g/,
        'universaladid'
      );

      expect(result).toContain('https://example.com/transcoded/index.m3u8');
      expect(result).toContain('application/x-mpegURL');
      expect(result).toContain('<vmap:VMAP');
      expect(result).toContain('</vmap:VMAP>');
    });

    it('should handle multiple ads in a single ad break', () => {
      const vmapXml = `<?xml version="1.0" encoding="UTF-8"?>
        <vmap:VMAP xmlns:vmap="http://www.iab.net/vmap-1.0" version="1.0">
          <vmap:AdBreak timeOffset="start" breakType="linear">
            <vmap:AdSource>
              <vmap:VASTAdData xmlns:vast="http://www.iab.net/VAST" version="4.0">
              <VAST version="4.0">
                <Ad>
                  <InLine>
                    <Creatives>
                      <Creative>
                        <UniversalAdId idRegistry="test-registry">ad123</UniversalAdId>
                        <Linear>
                          <MediaFiles>
                            <MediaFile type="video/mp4" bitrate="2000">
                              http://example.com/original1.mp4
                            </MediaFile>
                          </MediaFiles>
                        </Linear>
                      </Creative>
                    </Creatives>
                  </InLine>
                </Ad>
                <Ad>
                  <InLine>
                    <Creatives>
                      <Creative>
                        <UniversalAdId idRegistry="test-registry">ad456</UniversalAdId>
                        <Linear>
                          <MediaFiles>
                            <MediaFile type="video/mp4" bitrate="2000">
                              http://example.com/original2.mp4
                            </MediaFile>
                          </MediaFiles>
                        </Linear>
                      </Creative>
                    </Creatives>
                  </InLine>
                </Ad>
                </VAST>
              </vmap:VASTAdData>
            </vmap:AdSource>
          </vmap:AdBreak>
        </vmap:VMAP>`;

      const assets: ManifestAsset[] = [
        {
          creativeId: 'ad123',
          masterPlaylistUrl: 'https://example.com/transcoded1/index.m3u8'
        },
        {
          creativeId: 'ad456',
          masterPlaylistUrl: 'https://example.com/transcoded2/index.m3u8'
        }
      ];

      const result = replaceMediaFiles(
        vmapXml,
        assets,
        /[^a-zA-Z0-9]g/,
        'universaladid'
      );
      const mediaTypeCount = (result.match(/application\/x-mpegURL/g) || [])
        .length;

      expect(result).toContain('https://example.com/transcoded1/index.m3u8');
      expect(result).toContain('https://example.com/transcoded2/index.m3u8');
      expect(mediaTypeCount).toBe(2);
    });

    it('should select highest bitrate media file when multiple are present', () => {
      const vmapXml = `<?xml version="1.0" encoding="UTF-8"?>
        <vmap:VMAP xmlns:vmap="http://www.iab.net/vmap-1.0" version="1.0">
          <vmap:AdBreak timeOffset="start" breakType="linear">
            <vmap:AdSource>
              <vmap:VASTAdData xmlns:vast="http://www.iab.net/VAST" version="4.0">
                <VAST version="4.0">
                    <Ad>
                      <InLine>
                        <Creatives>
                          <Creative>
                            <UniversalAdId idRegistry="test-registry">ad123</UniversalAdId>
                            <Linear>
                              <MediaFiles>
                                <MediaFile type="video/mp4" bitrate="1000">
                                  http://example.com/low.mp4
                                </MediaFile>
                                <MediaFile type="video/mp4" bitrate="2000">
                                  http://example.com/medium.mp4
                                </MediaFile>
                                <MediaFile type="video/mp4" bitrate="3000">
                                  http://example.com/high.mp4
                                </MediaFile>
                              </MediaFiles>
                            </Linear>
                          </Creative>
                        </Creatives>
                      </InLine>
                    </Ad>
                  </VAST>
              </vmap:VASTAdData>
            </vmap:AdSource>
          </vmap:AdBreak>
        </vmap:VMAP>`;

      const assets: ManifestAsset[] = [
        {
          creativeId: 'ad123',
          masterPlaylistUrl: 'https://example.com/transcoded/index.m3u8'
        }
      ];

      const result = replaceMediaFiles(
        vmapXml,
        assets,
        /[^a-zA-Z0-9]g/,
        'universaladid'
      );
      const mediaFileCount = (result.match(/<\/MediaFile>/g) || []).length;

      expect(result).toContain('https://example.com/transcoded/index.m3u8');
      expect(mediaFileCount).toBe(1);
    });

    it('should remove ads without transcoded assets', () => {
      const vmapXml = `<?xml version="1.0" encoding="UTF-8"?>
        <vmap:VMAP xmlns:vmap="http://www.iab.net/vmap-1.0" version="1.0">
          <vmap:AdBreak timeOffset="start" breakType="linear">
            <vmap:AdSource>
              <vmap:VASTAdData>
              <VAST xmlns:vast="http://www.iab.net/VAST" version="4.0">
                <Ad>
                  <InLine>
                    <Creatives>
                      <Creative>
                        <UniversalAdId idRegistry="test-registry">ad123</UniversalAdId>
                        <Linear>
                          <MediaFiles>
                            <MediaFile type="video/mp4" bitrate="2000">
                              http://example.com/original.mp4
                            </MediaFile>
                          </MediaFiles>
                        </Linear>
                      </Creative>
                    </Creatives>
                  </InLine>
                </Ad>
                </VAST> 
              </vmap:VASTAdData>
            </vmap:AdSource>
          </vmap:AdBreak>
        </vmap:VMAP>`;

      const expectedXml = `<?xml version="1.0" encoding="UTF-8"?>
<vmap:VMAP xmlns:vmap="http://www.iab.net/vmap-1.0" version="1.0">
  <vmap:AdBreak timeOffset="start" breakType="linear">
    <vmap:AdSource>
      <vmap:VASTAdData>
        <VAST xmlns:vast="http://www.iab.net/VAST" version="4.0"></VAST>
      </vmap:VASTAdData>
    </vmap:AdSource>
  </vmap:AdBreak>
</vmap:VMAP>
`;

      const assets: ManifestAsset[] = [
        {
          creativeId: 'different-ad',
          masterPlaylistUrl: 'https://example.com/transcoded/index.m3u8'
        }
      ];

      const result = replaceMediaFiles(
        vmapXml,
        assets,
        /[^a-zA-Z0-9]g/,
        'universaladid'
      );
      expect(result).toContain(
        '<VAST xmlns:vast="http://www.iab.net/VAST" version="4.0"></VAST>'
      );
      expect(result).toBe(expectedXml);
    });
  });

  describe('getBestMediaFileFromVastAd', () => {
    it('should return the highest bitrate media file', () => {
      const vastAd: VastAd = {
        InLine: {
          Creatives: {
            Creative: {
              UniversalAdId: {
                '#text': 'test-ad-id',
                '@_idRegistry': 'test-id-registry'
              },
              Linear: {
                MediaFiles: {
                  MediaFile: [
                    {
                      '#text': 'http://example.com/low.mp4',
                      '@_bitrate': '1000',
                      '@_type': 'video/mp4',
                      '@_delivery': 'progressive',
                      '@_width': '1920',
                      '@_height': '1080'
                    },
                    {
                      '#text': 'http://example.com/high.mp4',
                      '@_bitrate': '3000',
                      '@_type': 'video/mp4',
                      '@_delivery': 'progressive',
                      '@_width': '1920',
                      '@_height': '1080'
                    },
                    {
                      '#text': 'http://example.com/medium.mp4',
                      '@_bitrate': '2000',
                      '@_type': 'video/mp4',
                      '@_delivery': 'progressive',
                      '@_width': '1920',
                      '@_height': '1080'
                    }
                  ]
                },
                Duration: '00:00:30'
              }
            }
          }
        }
      };

      const result = getBestMediaFileFromVastAd(vastAd);
      expect(result['#text']).toBe('http://example.com/high.mp4');
      expect(result['@_bitrate']).toBe('3000');
    });

    it('should handle single media file', () => {
      const vastAd: VastAd = {
        InLine: {
          Creatives: {
            Creative: {
              UniversalAdId: {
                '#text': 'test-ad-id',
                '@_idRegistry': 'test-id-registry'
              },
              Linear: {
                MediaFiles: {
                  MediaFile: {
                    '#text': 'http://example.com/video.mp4',
                    '@_bitrate': '2000',
                    '@_type': 'video/mp4',
                    '@_delivery': 'progressive',
                    '@_width': '1920',
                    '@_height': '1080'
                  }
                },
                Duration: '00:00:30'
              }
            }
          }
        }
      };

      const result = getBestMediaFileFromVastAd(vastAd);
      expect(result['#text']).toBe('http://example.com/video.mp4');
      expect(result['@_bitrate']).toBe('2000');
    });

    it('should handle media files without bitrate', () => {
      const vastAd: VastAd = {
        InLine: {
          Creatives: {
            Creative: {
              UniversalAdId: {
                '#text': 'test-ad-id',
                '@_idRegistry': 'test-id-registry'
              },
              Linear: {
                MediaFiles: {
                  MediaFile: [
                    {
                      '#text': 'http://example.com/video1.mp4',
                      '@_type': 'video/mp4',
                      '@_height': '1080',
                      '@_width': '1920',
                      '@_delivery': 'progressive'
                    },
                    {
                      '#text': 'http://example.com/video2.mp4',
                      '@_bitrate': '2000',
                      '@_type': 'video/mp4',
                      '@_height': '1080',
                      '@_width': '1920',
                      '@_delivery': 'progressive'
                    }
                  ]
                },
                Duration: '00:00:30'
              }
            }
          }
        }
      };

      const result = getBestMediaFileFromVastAd(vastAd);
      expect(result['#text']).toBe('http://example.com/video2.mp4');
      expect(result['@_bitrate']).toBe('2000');
    });

    it('should return first media file when none have bitrate', () => {
      const vastAd: VastAd = {
        InLine: {
          Creatives: {
            Creative: {
              UniversalAdId: {
                '#text': 'test-ad-id',
                '@_idRegistry': 'test-id-registry'
              },
              Linear: {
                MediaFiles: {
                  MediaFile: [
                    {
                      '#text': 'http://example.com/video1.mp4',
                      '@_type': 'video/mp4',
                      '@_delivery': 'progressive',
                      '@_width': '1920',
                      '@_height': '1080'
                    },
                    {
                      '#text': 'http://example.com/video2.mp4',
                      '@_type': 'video/mp4',
                      '@_delivery': 'progressive',
                      '@_width': '1920',
                      '@_height': '1080'
                    }
                  ]
                },
                Duration: '00:00:30'
              }
            }
          }
        }
      };

      const result = getBestMediaFileFromVastAd(vastAd);
      expect(result['#text']).toBe('http://example.com/video1.mp4');
      expect(result['@_type']).toBe('video/mp4');
    });
  });

  describe('getCreatives', () => {
    it('should extract creatives from VMAP XML', async () => {
      const vmapXml = {
        'vmap:VMAP': {
          'vmap:AdBreak': [
            {
              'vmap:AdSource': {
                'vmap:VASTAdData': {
                  VAST: {
                    Ad: [
                      {
                        InLine: {
                          Creatives: {
                            Creative: {
                              UniversalAdId: {
                                '#text': 'ad123',
                                '@_idRegistry': 'test-id-registry'
                              },
                              Linear: {
                                MediaFiles: {
                                  MediaFile: {
                                    '#text': 'http://example.com/video.mp4',
                                    '@_bitrate': '2000',
                                    '@_type': 'video/mp4'
                                  }
                                }
                              }
                            }
                          }
                        }
                      }
                    ]
                  }
                }
              }
            }
          ]
        }
      };

      const result = await getCreatives(vmapXml);
      expect(result).toHaveLength(1);
      expect(result[0]).toEqual({
        creativeId: 'ad123',
        masterPlaylistUrl: 'http://example.com/video.mp4'
      });
    });

    it('should handle multiple ad breaks', async () => {
      const vmapXml = {
        'vmap:VMAP': {
          'vmap:AdBreak': [
            {
              'vmap:AdSource': {
                'vmap:VASTAdData': {
                  VAST: {
                    Ad: {
                      InLine: {
                        Creatives: {
                          Creative: {
                            UniversalAdId: {
                              '#text': 'ad123',
                              '@_idRegistry': 'test-id-registry'
                            },
                            Linear: {
                              MediaFiles: {
                                MediaFile: {
                                  '#text': 'http://example.com/video1.mp4',
                                  '@_bitrate': '2000',
                                  '@_type': 'video/mp4'
                                }
                              }
                            }
                          }
                        }
                      }
                    }
                  }
                }
              }
            },
            {
              'vmap:AdSource': {
                'vmap:VASTAdData': {
                  VAST: {
                    Ad: {
                      InLine: {
                        Creatives: {
                          Creative: {
                            UniversalAdId: {
                              '#text': 'ad456',
                              '@_idRegistry': 'test-id-registry'
                            },
                            Linear: {
                              MediaFiles: {
                                MediaFile: {
                                  '#text': 'http://example.com/video2.mp4',
                                  '@_bitrate': '2000',
                                  '@_type': 'video/mp4'
                                }
                              }
                            }
                          }
                        }
                      }
                    }
                  }
                }
              }
            }
          ]
        }
      };

      const result = await getCreatives(vmapXml);
      expect(result).toHaveLength(2);
      expect(result[0]).toEqual({
        creativeId: 'ad123',
        masterPlaylistUrl: 'http://example.com/video1.mp4'
      });
      expect(result[1]).toEqual({
        creativeId: 'ad456',
        masterPlaylistUrl: 'http://example.com/video2.mp4'
      });
    });

    it('should handle multiple ads in a single ad break', async () => {
      const vmapXml = {
        'vmap:VMAP': {
          'vmap:AdBreak': [
            {
              'vmap:AdSource': {
                'vmap:VASTAdData': {
                  VAST: {
                    Ad: [
                      {
                        InLine: {
                          Creatives: {
                            Creative: {
                              UniversalAdId: {
                                '#text': 'ad123',
                                '@_idRegistry': 'test-id-registry'
                              },
                              Linear: {
                                MediaFiles: {
                                  MediaFile: {
                                    '#text': 'http://example.com/video1.mp4',
                                    '@_bitrate': '2000',
                                    '@_type': 'video/mp4'
                                  }
                                }
                              }
                            }
                          }
                        }
                      },
                      {
                        InLine: {
                          Creatives: {
                            Creative: {
                              UniversalAdId: {
                                '#text': 'ad456',
                                '@_idRegistry': 'test-id-registry'
                              },
                              Linear: {
                                MediaFiles: {
                                  MediaFile: {
                                    '#text': 'http://example.com/video2.mp4',
                                    '@_bitrate': '2000',
                                    '@_type': 'video/mp4'
                                  }
                                }
                              }
                            }
                          }
                        }
                      }
                    ]
                  }
                }
              }
            }
          ]
        }
      };

      const result = await getCreatives(vmapXml);
      expect(result).toHaveLength(2);
      expect(result[0]).toEqual({
        creativeId: 'ad123',
        masterPlaylistUrl: 'http://example.com/video1.mp4'
      });
      expect(result[1]).toEqual({
        creativeId: 'ad456',
        masterPlaylistUrl: 'http://example.com/video2.mp4'
      });
    });

    it('should handle empty VMAP', async () => {
      const vmapXml = {
        'vmap:VMAP': {}
      };

      const result = await getCreatives(vmapXml);
      expect(result).toHaveLength(0);
    });

    it('should handle VMAP without ads', async () => {
      const vmapXml = {
        'vmap:VMAP': {
          'vmap:AdBreak': [
            {
              'vmap:AdSource': {
                'vast:VAST': {}
              }
            }
          ]
        }
      };

      const result = await getCreatives(vmapXml);
      expect(result).toHaveLength(0);
    });

    it('should handle malformed VMAP and return empty array', async () => {
      const vmapXml = {
        'vmap:VMAP': {
          'vmap:AdBreak': [
            {
              'vmap:AdSource': {
                'vast:VAST': {
                  Ad: {
                    // Missing required InLine structure
                    UniversalAdId: {
                      '#text': 'ad123',
                      '@_idRegistry': 'test-id-registry'
                    }
                  }
                }
              }
            }
          ]
        }
      };

      const result = await getCreatives(vmapXml);
      expect(result).toHaveLength(0);
    });
  });
});
