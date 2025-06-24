package util

import (
	"testing"

	"github.com/Eyevinn/VMAP/vmap"
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
