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
	creatives := GetCreatives(vast, "resolution", "")
	is.Equal(len(creatives), 1)
	is.Equal(creatives[0].CreativeId, "1280x720")
	is.Equal(creatives[0].MasterPlaylistUrl, "http://example.com/video2.mp4")
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
