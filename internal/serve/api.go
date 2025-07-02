package serve

import (
	"compress/gzip"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"

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
	jitPackage     bool
	packageQueue   string
	encoreUrl      url.URL
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
		jitPackage:     config.JitPackage,
		packageQueue:   config.PackagingQueueName,
		encoreUrl:      config.EncoreUrl,
	}
}

func (api *API) HandleVmap(w http.ResponseWriter, r *http.Request) {
	// Implement the logic to handle the VMAP request
	// This will likely involve fetching data from valkeyStore and formatting it as needed
	vmapData := vmap.VMAP{}
	logger.Debug("Handling VMAP request", slog.String("path", r.URL.Path))
	byteResponse, err := api.makeAdServerRequest(r)
	if err != nil {
		logger.Error("failed to fetch VMAP data", slog.String("error", err.Error()))
		var adServerErr structure.AdServerError
		if errors.As(err, &adServerErr) {
			logger.Error("ad server error", slog.Int("statusCode", adServerErr.StatusCode), slog.String("message", adServerErr.Message))
			http.Error(w, adServerErr.Message, adServerErr.StatusCode)
			return
		} else {
			logger.Error("error fetching VMAP data", slog.String("error", err.Error()))

			http.Error(w, "Failed to fetch VMAP data", http.StatusInternalServerError)
			return
		}
	}

	vmapData, err = vmap.DecodeVmap(byteResponse)
	if err != nil {
		logger.Error("failed to decode VMAP data", slog.String("error", err.Error()))
		http.Error(w, "Failed to decode VMAP data", http.StatusInternalServerError)
		return
	}
	if err := api.processVmap(&vmapData); err != nil {
		logger.Error("failed to process VMAP data", slog.String("error", err.Error()))
		http.Error(w, "Failed to process VMAP data", http.StatusInternalServerError)
		return
	}
	serializedVmap, err := xml.Marshal(vmapData)
	if err != nil {
		logger.Error("failed to marshal VMAP data", slog.String("error", err.Error()))
		http.Error(w, "Failed to marshal VMAP data", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(serializedVmap)
}

func (api *API) HandleVast(w http.ResponseWriter, r *http.Request) {
	vastData := vmap.VAST{}
	logger.Debug("Handling VAST request", slog.String("path", r.URL.Path))
	responseBody, err := api.makeAdServerRequest(r)
	if err != nil {
		logger.Error("failed to fetch VAST data", slog.String("error", err.Error()))
		http.Error(w, "Failed to fetch VAST data", http.StatusInternalServerError)
		return
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

// Makes a request to the ad server and returns the response body.
// In the form of a byte slice. It's up to the caller to decode it as needed.
// If the response is gzipped, it will decompress it.
func (api *API) makeAdServerRequest(r *http.Request) ([]byte, error) {
	newUrl := api.adServerUrl
	adServerReq, err := http.NewRequest(
		"GET",
		newUrl.String(),
		nil,
	)
	if err != nil {
		logger.Error("failed to create ad server request", slog.String("error", err.Error()))
		return nil, err
	}
	setupHeaders(r, adServerReq)
	logger.Debug("Making ad server request", slog.String("url", adServerReq.URL.String()))
	response, err := api.client.Do(adServerReq)
	if err != nil {
		logger.Error("failed to fetch ad server data", slog.String("error", err.Error()))
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		logger.Error("failed to fetch ad server data", slog.Int("statusCode", response.StatusCode))
		return nil, structure.AdServerError{
			StatusCode: response.StatusCode,
			Message:    "Failed to fetch ad server data",
		}
	}
	var responseBody []byte
	if response.Header.Get("Content-Encoding") == "gzip" {
		// Handle gzip decompression if necessary
		responseBody, err = decompressGzip(response.Body)
		if err != nil {
			logger.Error("failed to decompress gzip response", slog.String("error", err.Error()))
			return nil, err
		}
	} else {
		responseBody, err = io.ReadAll(response.Body)
		if err != nil {
			logger.Error("failed to read response body", slog.String("error", err.Error()))
			return nil, err
		}
	}
	return responseBody, nil
}

func (api *API) processVmap(
	vmapData *vmap.VMAP,
) error {
	breakWg := &sync.WaitGroup{}
	for _, adBreak := range vmapData.AdBreaks {
		logger.Debug("Processing ad break", slog.String("breakId", adBreak.Id))
		breakWg.Add(1)
		go func(vastData *vmap.VAST) {
			defer breakWg.Done()
			api.findMissingAndDispatchJobs(vastData)
		}(adBreak.AdSource.VASTData.VAST)
	}
	breakWg.Wait()
	return nil
}

func (api *API) findMissingAndDispatchJobs(
	vast *vmap.VAST,
) {
	logger.Debug("Finding missing creatives in VAST", slog.Int("adCount", len(vast.Ad)))
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
	logger.Debug("partioning creatives", slog.Int("totalCreatives", len(creatives)))
	for _, creative := range creatives {
		transcodeInfo, urlFound, err := api.valkeyStore.Get(creative.CreativeId)
		if err != nil {
			logger.Error("failed to get creative from store", slog.String("error", err.Error()), slog.String("creativeId", creative.CreativeId))
			continue
		}
		if urlFound && transcodeInfo.Status == "COMPLETED" {
			found[creative.CreativeId] = structure.ManifestAsset{
				CreativeId:        creative.CreativeId,
				MasterPlaylistUrl: transcodeInfo.Url,
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
	defer func() { _ = zr.Close() }()
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
	or.Header.Add("User-Agent", "eyevinn/ad-normalizer")
	if deviceUserAgent != "" {
		or.Header.Add(userAgentHeader, deviceUserAgent)
	}
	or.Header.Add(forwardedForHeader, forwardedFor)
	or.Header.Add("Accept", "application/xml")
	or.Header.Add("Accept-Encoding", "gzip")
	// Copy query parameters from the incoming request to the outgoing request
	query := or.URL.Query()
	for k, v := range ir.URL.Query() {
		if strings.ToLower(k) == "subdomain" {
			logger.Debug("Replacing subdomain in URL", slog.String("subdomain", v[0]), slog.String("originalUrl", or.URL.String()))
			newUrl := util.ReplaceSubdomain(*or.URL, v[0])
			or.URL = &newUrl
			logger.Debug("New URL after subdomain replacement", slog.String("newUrl", or.URL.String()))
			continue
		}
		for _, val := range v {
			query.Add(k, val)
		}
	}
	or.URL.RawQuery = query.Encode()
}
