package structure

import (
	"net/url"
	"path"
	"strings"
)

type ManifestFormat string

const (
	ManifestFormatHLS  ManifestFormat = "vnd.apple.mpegurl"
	ManifestFormatDASH ManifestFormat = "dash"
)

const dashManifestFileName = "manifest.mpd"

func (mf ManifestFormat) Extension() string {
	switch mf {
	case ManifestFormatDASH:
		return ".mpd"
	default:
		return ".m3u8"
	}
}

func (mf ManifestFormat) MediaType() string {
	switch mf {
	case ManifestFormatDASH:
		return "application/dash+xml"
	default:
		return "application/x-mpegURL"
	}
}

func (mf ManifestFormat) PreferValue() string {
	if mf == ManifestFormatDASH {
		return string(ManifestFormatDASH)
	}
	return string(ManifestFormatHLS)
}

func ManifestUrlForFormat(manifestUrl string, format ManifestFormat) string {
	if format == ManifestFormatDASH {
		return dashManifestUrl(manifestUrl)
	}
	parsedUrl, err := url.Parse(manifestUrl)
	if err != nil {
		return replaceManifestExtension(manifestUrl, format.Extension())
	}
	parsedUrl.Path = replaceManifestExtension(parsedUrl.Path, format.Extension())
	return parsedUrl.String()
}

func dashManifestUrl(manifestUrl string) string {
	parsedUrl, err := url.Parse(manifestUrl)
	if err != nil {
		return replaceManifestFileName(manifestUrl, dashManifestFileName)
	}
	parsedUrl.Path = replaceManifestFileName(parsedUrl.Path, dashManifestFileName)
	return parsedUrl.String()
}

func replaceManifestFileName(manifestPath string, fileName string) string {
	return path.Join(path.Dir(manifestPath), fileName)
}

func replaceManifestExtension(manifestPath string, extension string) string {
	currentExtension := path.Ext(manifestPath)
	if currentExtension == "" {
		return strings.TrimRight(manifestPath, "/") + extension
	}
	return strings.TrimSuffix(manifestPath, currentExtension) + extension
}
