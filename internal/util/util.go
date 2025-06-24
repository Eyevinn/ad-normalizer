package util

import (
	"regexp"
	"strconv"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
)

func GetBestMediaFileFromVastAd(ad *vmap.Ad) *vmap.MediaFile {
	bestMediaFile := &vmap.MediaFile{}
	for _, c := range ad.InLine.Creatives {
		for _, m := range c.Linear.MediaFiles {
			if m.Bitrate > bestMediaFile.Bitrate {
				bestMediaFile = &m
			}
		}
	}
	return bestMediaFile
}

func GetCreatives(
	vast *vmap.VAST,
	keyField string,
	keyRegex string,
) map[string]structure.ManifestAsset {
	creatives := make(map[string]structure.ManifestAsset, len(vast.Ad))
	for _, ad := range vast.Ad {
		mediaFile := GetBestMediaFileFromVastAd(&ad)
		adId := getKey(keyField, keyRegex, &ad, mediaFile)
		creatives[adId] = structure.ManifestAsset{
			CreativeId:        getKey(keyField, keyRegex, &ad, mediaFile),
			MasterPlaylistUrl: mediaFile.Text,
		}
	}

	return creatives
}

func getKey(keyField, keyRegex string, ad *vmap.Ad, mediaFile *vmap.MediaFile) string {
	var res string
	switch keyField {
	case "resolution":
		res = strconv.Itoa(mediaFile.Width) + "x" + strconv.Itoa(mediaFile.Height)
	case "url":
		re := regexp.MustCompile(keyRegex)
		res = re.ReplaceAllString(mediaFile.Text, "")
	default:
		re := regexp.MustCompile(keyRegex)
		res = re.ReplaceAllString(ad.InLine.Creatives[0].UniversalAdId.Id, "")
	}
	return res
}

func ReplaceMediaFiles(
	vast *vmap.VAST,
	assets map[string]structure.ManifestAsset,
	keyRegex string,
	keyField string,
) error {
	newAds := make([]vmap.Ad, 0, len(vast.Ad))
	for _, ad := range vast.Ad {
		mediaFile := GetBestMediaFileFromVastAd(&ad)
		adId := getKey(keyField, keyRegex, &ad, mediaFile)
		if asset, found := assets[adId]; found {
			newAd := ad
			newMediaFile := *mediaFile // Copy to overwrite
			newMediaFile.Text = asset.MasterPlaylistUrl
			newMediaFile.MediaType = "application/x-mpegURL"
			newAd.InLine.Creatives[0].Linear.MediaFiles = []vmap.MediaFile{
				newMediaFile,
			}
		}
	}
	vast.Ad = newAds
	return nil
}
