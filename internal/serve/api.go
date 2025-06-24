package serve

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/encore"
	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/store"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/Eyevinn/ad-normalizer/internal/util"
)

const userAgentHeader = "X-Device-User-Agent"
const forwardedForHeader = "X-Forwarded-For"

type API struct {
	valkeyStore    store.Store
	adServerUrl    string
	encoreUrl      string
	assetServerUrl string
	keyField       string
	keyRegex       string
	EncoreHandler  *encore.EncoreHandler
	client         *http.Client
}

func NewAPI(valkeyStore store.Store, adServerUrl, encoreUrl, assetServerUrl string) *API {
	return &API{
		valkeyStore:    valkeyStore,
		adServerUrl:    adServerUrl,
		encoreUrl:      encoreUrl,
		assetServerUrl: assetServerUrl,
	}
}

func (api *API) HandleVmap(w http.ResponseWriter, r *http.Request) {
	// Implement the logic to handle the VMAP request
	// This will likely involve fetching data from valkeyStore and formatting it as needed
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("VMAP response"))
}

// TODO: Fix error handling
func (api *API) HandleVast(w http.ResponseWriter, r *http.Request) {
	// Implement the logic to handle the VAST request
	// This will likely involve fetching data from valkeyStore and formatting it as needed
	vastData := vmap.VAST{}
	vastReq, err := http.NewRequest("GET", api.adServerUrl, nil) // TODO: Handle the error properly
	if err != nil {
		logger.Error("failed to create VAST request", slog.String("error", err.Error()))
		return vastData, err
	}
	setupHeaders(r, vastReq)
	response, err := api.client.Do(vastReq)
	if err != nil {
		logger.Error("failed to fetch VAST data", slog.String("error", err.Error()))
		return vastData, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		logger.Error("failed to fetch VAST data", slog.Int("statusCode", response.StatusCode))
		return vastData, err
	}

	var responseBody []byte
	if response.Header.Get("Content-Encoding") == "gzip" {
		// Handle gzip decompression if necessary
		responseBody, err = decompressGzip(response.Body)
		if err != nil {
			logger.Error("failed to decompress gzip response", slog.String("error", err.Error()))
			return vastData, err
		}
	} else {
		responseBody, err = io.ReadAll(response.Body)
		if err != nil {
			logger.Error("failed to read response body", slog.String("error", err.Error()))
			return vastData, err
		}
	}
	vastData, err = vmap.DecodeVast(responseBody)
	if err != nil {
		logger.Error("failed to decode VAST data", slog.String("error", err.Error()))
		return vastData, err
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("VAST response"))
}

func (api *API) findMissingAndDispatchJobs(
	vast *vmap.VAST,
) {
	creatives := util.GetCreatives(vast, api.keyField, api.keyRegex)
	found, missing := api.partitionCreatives(creatives)
	logger.Debug("partitioned creatives", slog.Int("found", len(found)), slog.Int("missing", len(missing)))

	// No need to wait for the goroutines to finish
	// Since the creatives won't be used in this response anyway
	for _, creative := range missing {
		go func(creative *structure.ManifestAsset) {
			encoreJob, err := api.EncoreHandler.CreateJob(creative)
			if err != nil {
				logger.Error("failed to create encore job", slog.String("error", err.Error()), slog.String("creativeId", creative.CreativeId))
				return
			}
			logger.Debug("created encore job", slog.String("creativeId", creative.CreativeId), slog.String("jobId", encoreJob.Id))

		}(&creative)
	}
}

func (api *API) partitionCreatives(
	creatives map[string]structure.ManifestAsset,
) (map[string]structure.ManifestAsset, map[string]structure.ManifestAsset) {
	found := make(map[string]structure.ManifestAsset, len(creatives))
	missing := make(map[string]structure.ManifestAsset, len(creatives))

	for _, creative := range creatives {
		url, urlFound, err := api.valkeyStore.Get(creative.CreativeId)
		if err != nil {
			logger.Error("failed to get creative from store", slog.String("error", err.Error()), slog.String("creativeId", creative.CreativeId))
			continue
		}
		if urlFound {
			found[creative.CreativeId] = structure.ManifestAsset{
				CreativeId:        creative.CreativeId,
				MasterPlaylistUrl: url,
			}
		} else {
			missing[creative.CreativeId] = structure.ManifestAsset{
				CreativeId:        creative.CreativeId,
				MasterPlaylistUrl: creative.MasterPlaylistUrl,
			}
		}
	}
	return found, missing
}

func decompressGzip(body io.Reader) ([]byte, error) {
	zr, err := gzip.NewReader(body)
	defer zr.Close()
	if err != nil {
		return []byte{}, err
	}
	output, err := io.ReadAll(zr)
	if err != nil {
		return []byte{}, err
	}
	return output, nil
}

func setupHeaders(ir *http.Request, or *http.Request) {
	deviceUserAgent := ir.Header.Get(userAgentHeader)
	forwardedFor := ir.Header.Get(forwardedForHeader)
	or.Header.Add("User-Agent", "eyevinnn/ad-normalizer")
	if deviceUserAgent != "" {
		or.Header.Add(userAgentHeader, deviceUserAgent)
	}
	or.Header.Add(forwardedForHeader, forwardedFor)
	or.Header.Add("Accept", "application/xml")
	or.Header.Add("Accept-Encoding", "gzip")
}
