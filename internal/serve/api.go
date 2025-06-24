package serve

import (
	"compress/gzip"
	"encoding/xml"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/config"
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
	adServerUrl    url.URL
	assetServerUrl url.URL
	keyField       string
	keyRegex       string
	encoreHandler  encore.EncoreHandler
	client         *http.Client
}

func NewAPI(
	valkeyStore store.Store,
	config config.AdNormalizerConfig,
	encoreHandler encore.EncoreHandler,
	client *http.Client,
) *API {
	return &API{
		valkeyStore:    valkeyStore,
		adServerUrl:    config.AdServerUrl,
		assetServerUrl: config.AssetServerUrl,
		keyField:       config.KeyField,
		keyRegex:       config.KeyRegex,
		encoreHandler:  encoreHandler,
		client:         client,
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

	newUrl := api.adServerUrl
	newUrl.Path = path.Join(api.adServerUrl.Path, r.URL.Path)
	vastReq, err := http.NewRequest(
		"GET",
		newUrl.String(),
		nil,
	) // TODO: Handle the error properly
	if err != nil {
		logger.Error("failed to create VAST request", slog.String("error", err.Error()))
		http.Error(w, "Failed to create VAST request", http.StatusInternalServerError)
		return
	}
	setupHeaders(r, vastReq)
	response, err := api.client.Do(vastReq)
	if err != nil {
		logger.Error("failed to fetch VAST data", slog.String("error", err.Error()))
		http.Error(w, "Failed to fetch VAST data", http.StatusInternalServerError)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		logger.Error("failed to fetch VAST data", slog.Int("statusCode", response.StatusCode))
		http.Error(w, "Failed to fetch VAST data", response.StatusCode)
		return
	}

	var responseBody []byte
	if response.Header.Get("Content-Encoding") == "gzip" {
		// Handle gzip decompression if necessary
		responseBody, err = decompressGzip(response.Body)
		if err != nil {
			logger.Error("failed to decompress gzip response", slog.String("error", err.Error()))
			http.Error(w, "Failed to decompress gzip response", http.StatusInternalServerError)
			return
		}
	} else {
		responseBody, err = io.ReadAll(response.Body)
		if err != nil {
			logger.Error("failed to read response body", slog.String("error", err.Error()))
			http.Error(w, "Failed to read response body", http.StatusInternalServerError)
			return
		}
	}
	vastData, err = vmap.DecodeVast(responseBody)
	if err != nil {
		logger.Error("failed to decode VAST data", slog.String("error", err.Error()))
		http.Error(w, "Failed to decode VAST data", http.StatusInternalServerError)
		return
	}
	api.findMissingAndDispatchJobs(&vastData)
	serializedVast, err := xml.Marshal(vastData)
	if err != nil {
		logger.Error("failed to marshal VAST data", slog.String("error", err.Error()))
		http.Error(w, "Failed to marshal VAST data", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(serializedVast)
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
			encoreJob, err := api.encoreHandler.CreateJob(creative)
			if err != nil {
				logger.Error("failed to create encore job", slog.String("error", err.Error()), slog.String("creativeId", creative.CreativeId))
				return
			}
			logger.Debug("created encore job", slog.String("creativeId", creative.CreativeId), slog.String("jobId", encoreJob.Id))

		}(&creative)
	}
	// TODO: Error handling
	_ = util.ReplaceMediaFiles(
		vast,
		found,
		api.keyRegex,
		api.keyField,
	)
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
