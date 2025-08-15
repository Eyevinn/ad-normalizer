package util

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/matryer/is"
)

func TestGetBestMediaFileFromVastAd(t *testing.T) {
	is := is.New(t)
	ad := defaultAd()
	res := GetBestMediaFileFromVastAd(&ad)
	is.Equal(res.Bitrate, 2000)
	is.Equal(res.Width, 1280)
	is.Equal(res.Height, 720)
}

func TestGetCreatives(t *testing.T) {
	vast := DefaultVast()
	is := is.New(t)
	cases := []struct {
		key         string
		regex       string
		expectedKey string
	}{
		{
			key:         "resolution",
			regex:       "",
			expectedKey: "1280x720",
		},
		{
			key:         "url",
			regex:       "[^a-zA-Z0-9]",
			expectedKey: "httpexamplecomvideo2mp4",
		},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			creatives := GetCreatives(vast, c.key, c.regex)
			is.Equal(len(creatives), 1)
			is.Equal(creatives[c.expectedKey].CreativeId, c.expectedKey)
			is.Equal(creatives[c.expectedKey].MasterPlaylistUrl, "http://example.com/video2.mp4")
		})
	}
}

func TestReplaceSubdomain(t *testing.T) {
	is := is.New(t)
	oldUrl, _ := url.Parse("http://old-subdomain.example.com/video1.mp4")
	newSubdomain := "new-subdomain"
	res := ReplaceSubdomain(*oldUrl, newSubdomain)
	is.Equal(res.Host, "new-subdomain.example.com")

}

func TestSubdomainLocalhost(t *testing.T) {
	is := is.New(t)
	oldUrl, _ := url.Parse("http://localhost:1337/video1.mp4")
	newSubdomain := "new-subdomain"
	res := ReplaceSubdomain(*oldUrl, newSubdomain)
	is.Equal(res.Host, "new-subdomain.localhost:1337")
}

func TestReplaceMediaFiles(t *testing.T) {
	vast := DefaultVast()
	is := is.New(t)
	vast.Ad = append(vast.Ad, vmap.Ad{
		InLine: &vmap.InLine{
			Creatives: []vmap.Creative{
				{
					Linear: &vmap.Linear{
						MediaFiles: []vmap.MediaFile{
							{Bitrate: 1000, Width: 640, Height: 360, Text: "http://example2.com/video1.mp4"},
							{Bitrate: 2000, Width: 1280, Height: 720, Text: "http://example2.com/video2.mp4"},
						},
					},
				},
			},
		},
	})
	assets := make(map[string]structure.ManifestAsset)
	assets["httpexamplecomvideo2mp4"] = structure.ManifestAsset{
		CreativeId:        "httpexamplecomvideo2mp4",
		MasterPlaylistUrl: "http://example.com/video2/index.m3u8",
	}
	err := ReplaceMediaFiles(vast, assets, "[^a-zA-Z0-9]", "url")
	is.NoErr(err)
	is.Equal(len(assets), 1)
}

func TestCreateOutputUrl(t *testing.T) {
	is := is.New(t)
	bucketUrl, _ := url.Parse("s3://example.com/transcoding-output/")
	folder := "test-creative-id"
	res := CreateOutputUrl(*bucketUrl, folder)
	is.True(strings.HasPrefix(res, "s3://example.com/transcoding-output/test-creative-id/"))
}

func DefaultVast() *vmap.VAST {
	return &vmap.VAST{
		Version: "4.0",
		Ad: []vmap.Ad{
			defaultAd(),
		},
	}
}

func defaultAd() vmap.Ad {
	return vmap.Ad{
		InLine: &vmap.InLine{
			Creatives: []vmap.Creative{
				{
					Linear: &vmap.Linear{
						MediaFiles: []vmap.MediaFile{
							{Bitrate: 1000, Width: 640, Height: 360, Text: "http://example.com/video1.mp4"},
							{Bitrate: 2000, Width: 1280, Height: 720, Text: "http://example.com/video2.mp4"},
						},
					},
				},
			},
		},
	}
}
