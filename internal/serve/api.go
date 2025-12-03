package serve

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/encore"
	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/normalizerMetrics"
	"github.com/Eyevinn/ad-normalizer/internal/store"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/Eyevinn/ad-normalizer/internal/util"
	"go.opentelemetry.io/otel"
)

const userAgentHeader = "X-Device-User-Agent"
const forwardedForHeader = "X-Forwarded-For"
const jobPath = "/jobs"
const blacklistPath = "/blacklist"

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
	reportKpi      func(normalizerMetrics.AdsHandledEventArguments)
}

func NewAPI(
	valkeyStore store.Store,
	config config.AdNormalizerConfig,
	encoreHandler encore.EncoreHandler,
	client *http.Client,
	kpiReportFunc func(normalizerMetrics.AdsHandledEventArguments),
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
		reportKpi:      kpiReportFunc,
	}
}

type statusResponse struct {
	Jobs        []structure.TranscodeInfo `json:"jobs"`
	Page        int                       `json:"page"`
	Size        int                       `json:"size"`
	Next        string                    `json:"next,omitempty"`
	Prev        string                    `json:"prev,omitempty"`
	TotalAmount int64                     `json:"totalAmount"`
}

func (api *API) HandleJobList(w http.ResponseWriter, r *http.Request) {
	_, span := otel.Tracer("api").Start(r.Context(), "HandleStatus")
	defer span.End()
	var err error
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	page := 0
	size := 10
	if p := query.Get("page"); p != "" {
		page, err = strconv.Atoi(p)
		if err != nil || page < 0 {
			http.Error(w, "Invalid page parameter", http.StatusBadRequest)
			return
		}
	}
	if s := query.Get("size"); s != "" {
		size, err = strconv.Atoi(s)
		if err != nil || size <= 0 || size > 100 {
			http.Error(w, "Invalid size parameter", http.StatusBadRequest)
			return
		}
	}

	var prev, next string
	if page > 0 {
		prev = jobPath + "?page=" + strconv.Itoa(page-1) + "&size=" + strconv.Itoa(size)
	}

	results, cardinality, err := api.valkeyStore.List(page, size)
	if err != nil {
		logger.Error("failed to list jobs", slog.String("error", err.Error()))
		http.Error(w, "Failed to list jobs", http.StatusInternalServerError)
		return
	}

	if len(results) == size {
		next = jobPath + "?page=" + strconv.Itoa(page+1) + "&size=" + strconv.Itoa(size)
	}
	resp := statusResponse{
		Jobs:        results,
		Page:        page,
		Size:        len(results),
		Next:        next,
		Prev:        prev,
		TotalAmount: cardinality,
	}
	// Marshal to JSON and write response
	ret, err := json.Marshal(resp)
	if err != nil {
		logger.Error("failed to marshal jobs", slog.String("error", err.Error()))
		http.Error(w, "Failed to marshal jobs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(ret)
}

type blacklistRequest struct {
	MediaUrl string `json:"mediaUrl"`
}

type blacklistResponse struct {
	MediaUrls  []string `json:"mediaUrls"`
	Page       int      `json:"page"`
	Size       int      `json:"size"`
	Next       string   `json:"next,omitempty"`
	Prev       string   `json:"prev,omitempty"`
	TotalCount int64    `json:"totalCount"`
}

func readBlacklistRequest(r *http.Request) (blacklistRequest, error) {
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		return blacklistRequest{}, err
	}
	var blRequest blacklistRequest
	if err := json.Unmarshal(bytes, &blRequest); err != nil {
		return blRequest, err
	}

	return blRequest, nil
}

func (api *API) HandleBlackList(w http.ResponseWriter, r *http.Request) {
	_, span := otel.Tracer("api").Start(r.Context(), "HandleBlackList")
	defer span.End()
	if r.Method != http.MethodPost && r.Method != http.MethodDelete && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch r.Method {
	case http.MethodPost:
		blRequest, err := readBlacklistRequest(r)
		if err != nil {
			logger.Error("failed to read blacklist request", slog.String("error", err.Error()))
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		err = api.valkeyStore.BlackList(blRequest.MediaUrl)
		if err != nil {
			logger.Error("failed to blacklist media URL",
				slog.String("mediaUrl", blRequest.MediaUrl),
				slog.String("error", err.Error()),
			)
			http.Error(w, "Failed to blacklist media URL", http.StatusInternalServerError)
			return
		}
		logger.Info("blacklisted media URL", slog.String("mediaUrl", blRequest.MediaUrl))
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		blRequest, err := readBlacklistRequest(r)
		if err != nil {
			logger.Error("failed to read blacklist request", slog.String("error", err.Error()))
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		err = api.valkeyStore.RemoveFromBlackList(blRequest.MediaUrl)
		if err != nil {
			logger.Error("failed to unblacklist media URL",
				slog.String("mediaUrl", blRequest.MediaUrl),
				slog.String("error", err.Error()),
			)
			http.Error(w, "Failed to unblacklist media URL", http.StatusInternalServerError)
			return
		}
		logger.Info("unblacklisted media URL", slog.String("mediaUrl", blRequest.MediaUrl))
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		query := r.URL.Query()
		page := 0
		size := 10
		if p := query.Get("page"); p != "" {
			page, err := strconv.Atoi(p)
			if err != nil || page < 0 {
				http.Error(w, "Invalid page parameter", http.StatusBadRequest)
				return
			}
		}
		if s := query.Get("size"); s != "" {
			size, err := strconv.Atoi(s)
			if err != nil || size <= 0 || size > 100 {
				http.Error(w, "Invalid size parameter", http.StatusBadRequest)
				return
			}
		}
		var prev, next string
		if page > 0 {
			prev = blacklistPath + "?page=" + strconv.Itoa(page-1) + "&size=" + strconv.Itoa(size)
		}

		results, cardinality, err := api.valkeyStore.GetBlackList(page, size)
		if err != nil {
			logger.Error("failed to list jobs", slog.String("error", err.Error()))
			http.Error(w, "Failed to list jobs", http.StatusInternalServerError)
			return
		}

		if len(results) == size {
			next = blacklistPath + "?page=" + strconv.Itoa(page+1) + "&size=" + strconv.Itoa(size)
		}
		resp := blacklistResponse{
			MediaUrls:  results,
			Page:       page,
			Size:       len(results),
			Next:       next,
			Prev:       prev,
			TotalCount: cardinality,
		}
		ret, err := json.Marshal(resp)
		if err != nil {
			logger.Error("failed to marshal blacklist", slog.String("error", err.Error()))
			http.Error(w, "Failed to marshal blacklist", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(ret)
	}
}

func (api *API) HandleVmap(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("api").Start(r.Context(), "HandleVmap")
	vmapData := vmap.VMAP{}
	logger.Debug("Handling VMAP request", slog.String("path", r.URL.Path))
	byteResponse, subdomain, err := api.makeAdServerRequest(r, ctx)
	if err != nil {
		logger.Error("failed to fetch VMAP data", slog.String("error", err.Error()))
		var adServerErr structure.AdServerError
		if errors.As(err, &adServerErr) {
			logger.Error("ad server error",
				slog.Int("statusCode", adServerErr.StatusCode),
				slog.String("error", adServerErr.Message),
			)
			http.Error(w, adServerErr.Message, adServerErr.StatusCode)
			return
		} else {
			logger.Error("error fetching VMAP data", slog.String("error", err.Error()))

			http.Error(w, "Failed to fetch VMAP data", http.StatusInternalServerError)
			return
		}
	}

	vmapData, err = vmap.DecodeVmap(byteResponse)
	span.AddEvent("Decoded VMAP data")
	if err != nil {
		logger.Error("failed to decode VMAP data", slog.String("error", err.Error()))
		http.Error(w, "Failed to decode VMAP data", http.StatusInternalServerError)
		return
	}
	if err := api.processVmap(&vmapData, subdomain); err != nil {
		logger.Error("failed to process VMAP data", slog.String("error", err.Error()))
		http.Error(w, "Failed to process VMAP data", http.StatusInternalServerError)
		return
	}
	span.AddEvent("Processed VMAP data")
	serializedVmap, err := xml.Marshal(vmapData)
	if err != nil {
		logger.Error("failed to marshal VMAP data", slog.String("error", err.Error()))
		http.Error(w, "Failed to marshal VMAP data", http.StatusInternalServerError)
		return
	}
	span.AddEvent("Serialized VMAP data")
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(serializedVmap)
	span.End()
}

func (api *API) HandleVast(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("api").Start(r.Context(), "HandleVast")

	vastData := vmap.VAST{}
	logger.Debug("Handling VAST request", slog.String("path", r.URL.Path))
	qp := r.URL.Query()
	fillerUrl := qp.Get("filler")
	responseBody, subdomain, err := api.makeAdServerRequest(r, ctx)
	if err != nil {
		logger.Error("failed to fetch VAST data", slog.String("error", err.Error()))
		http.Error(w, "Failed to fetch VAST data", http.StatusInternalServerError)
		return
	}
	vastData, err = vmap.DecodeVast(responseBody)
	span.AddEvent("Decoded VAST data")
	if err != nil {
		logger.Error("failed to decode VAST data",
			slog.String("error", err.Error()),
			slog.String("responseBody", string(responseBody)),
		)
		http.Error(w, "Failed to decode VAST data", http.StatusInternalServerError)
		return
	}
	logger.Debug("Decoded VAST data", slog.Int("adCount", len(vastData.Ad)))
	if fillerUrl != "" {
		logger.Debug("Adding filler to the end of the VAST",
			slog.String("fillerUrl", fillerUrl),
		)
		vastData.Ad = append(vastData.Ad, util.CreateFillerAd(fillerUrl, len(vastData.Ad)+1))
	}
	api.findMissingAndDispatchJobs(&vastData, subdomain)
	var serializedVast []byte
	requestedContentType := r.Header.Get("Accept")
	if requestedContentType == "application/json" {
		span.AddEvent("Processing VAST data for JSON response")
		assetDescriptors := util.ConvertToAssetDescriptionSlice(&vastData)
		span.AddEvent("Converted VAST data to asset descriptions")
		serializedVast, err = json.Marshal(assetDescriptors)
		if err != nil {
			logger.Error("failed to marshal VAST data to JSON", slog.String("error", err.Error()))
			http.Error(w, "Failed to marshal VAST data to JSON", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
	} else {
		span.AddEvent("Processed VAST data")
		serializedVast, err = xml.Marshal(vastData)
		span.AddEvent("Serialized VAST data")
		if err != nil {
			logger.Error("failed to marshal VAST data", slog.String("error", err.Error()))
			http.Error(w, "Failed to marshal VAST data", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(serializedVast)
	span.End()
}

// Makes a request to the ad server and returns the response body.
// In the form of a byte slice. It's up to the caller to decode it as needed.
// If the response is gzipped, it will decompress it.
func (api *API) makeAdServerRequest(r *http.Request, ctx context.Context) ([]byte, string, error) {
	_, span := otel.Tracer("api").Start(ctx, "makeAdServerRequest")
	defer span.End()
	newUrl := api.adServerUrl
	subdomain := ""
	for k := range r.URL.Query() {
		if strings.ToLower(k) == "subdomain" {
			subdomain = r.URL.Query().Get(k)
		}
	}
	if subdomain != "" {
		logger.Debug("Replacing subdomain in URL",
			slog.String("subdomain", subdomain),
			slog.String("originalUrl", newUrl.String()),
		)
		newUrl = util.ReplaceSubdomain(newUrl, subdomain)
		logger.Debug("New URL after subdomain replacement", slog.String("newUrl", newUrl.String()))
	}
	adServerReq, err := http.NewRequest(
		"GET",
		newUrl.String(),
		nil,
	)
	if err != nil {
		logger.Error("failed to create ad server request",
			slog.String("error", err.Error()),
		)
		return nil, subdomain, err
	}
	span.AddEvent("Created ad server request")
	setupHeaders(r, adServerReq)
	span.AddEvent("Done setting up headers and query parameters")
	logger.Debug("Making ad server request", slog.String("url", adServerReq.URL.String()))
	response, err := api.client.Do(adServerReq)
	if err != nil {
		logger.Error("failed to fetch ad server data", slog.String("error", err.Error()))
		return nil, subdomain, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		logger.Error("failed to fetch ad server data", slog.Int("statusCode", response.StatusCode))
		return nil, subdomain, structure.AdServerError{
			StatusCode: response.StatusCode,
			Message:    "Failed to fetch ad server data",
		}
	}
	span.AddEvent("Received response from ad server")
	var responseBody []byte
	if response.Header.Get("Content-Encoding") == "gzip" {
		// Handle gzip decompression if necessary
		responseBody, err = decompressGzip(response.Body)
		if err != nil {
			logger.Error("failed to decompress gzip response", slog.String("error", err.Error()))
			return nil, subdomain, err
		}
		span.AddEvent("Decompressed gzip response")
	} else {
		responseBody, err = io.ReadAll(response.Body)
		if err != nil {
			logger.Error("failed to read response body", slog.String("error", err.Error()))
			return nil, subdomain, err
		}
	}
	return responseBody, subdomain, nil
}

func (api *API) processVmap(
	vmapData *vmap.VMAP,
	subdomain string,
) error {
	breakWg := &sync.WaitGroup{}
	for _, adBreak := range vmapData.AdBreaks {
		logger.Debug("Processing ad break", slog.String("breakId", adBreak.Id))
		if adBreak.AdSource.VASTData.VAST != nil {
			breakWg.Add(1)
			go func(vastData *vmap.VAST, subdomain string) {
				defer breakWg.Done()
				api.findMissingAndDispatchJobs(vastData, subdomain)
			}(adBreak.AdSource.VASTData.VAST, subdomain)
		}
	}
	breakWg.Wait()
	return nil
}

func (api *API) dispatchJobs(missingCreatives map[string]structure.ManifestAsset) {
	// No need to wait for the goroutines to finish
	// Since the creatives won't be used in this response anyway
	for _, creative := range missingCreatives {
		go func(creative *structure.ManifestAsset) {
			encoreJob, err := api.encoreHandler.CreateJob(creative)
			if err != nil {
				logger.Error("failed to create encore job",
					slog.String("error", err.Error()),
					slog.String("creativeId", creative.CreativeId),
				)
				return
			}
			logger.Debug("created encore job",
				slog.String("creativeId", creative.CreativeId),
				slog.String("jobId", encoreJob.Id),
			)
			_ = api.valkeyStore.Set(creative.CreativeId, structure.TranscodeInfo{
				Url:        creative.MasterPlaylistUrl,
				Status:     "QUEUED",
				Source:     creative.MasterPlaylistUrl,
				LastUpdate: time.Now().Unix(),
			})
		}(&creative)
	}
}

func (api *API) findMissingAndDispatchJobs(
	vast *vmap.VAST,
	subdomain string,
) {
	logger.Debug("Finding missing creatives in VAST", slog.Int("adCount", len(vast.Ad)))
	creatives := util.GetCreatives(vast, api.keyField, api.keyRegex)
	found, missing, filteredOut := api.partitionCreatives(creatives)
	logger.Debug("partitioned creatives", slog.Int("found", len(found)), slog.Int("missing", len(missing)))

	api.dispatchJobs(missing)

	api.reportKpi(normalizerMetrics.AdsHandledEventArguments{
		Subdomain:   subdomain,
		BrokenAds:   filteredOut,
		IngestedAds: len(missing),
		ServedAds:   len(found),
	})

	// TODO: Error handling
	_ = util.ReplaceMediaFiles(
		vast,
		found,
		api.keyRegex,
		api.keyField,
	)

}

// Same as findMissingAndDispatchJobs but for JSON requests, since the original is built around VAST
// Returns an int representing the number of missing creatives that jobs will be created for
func (api *API) findMissingAndDispatchJobsJson(request *preIngestCreativeRequest) int {
	logger.Debug("Finding missing creatives in pre-ingest request", slog.Int("mediaUrlCount", len(request.MediaUrls)))
	// convert to ManifestAsset
	creatives := util.MakeCreatives(request.MediaUrls, api.keyRegex)
	found, missing, _ := api.partitionCreatives(creatives)
	logger.Debug("partitioned creatives", slog.Int("found", len(found)), slog.Int("missing", len(missing)))
	api.dispatchJobs(missing)
	return len(missing)
}

// TODO: Return amt blacklisted as well
func (api *API) partitionCreatives(
	creatives map[string]structure.ManifestAsset,
) (map[string]structure.ManifestAsset, map[string]structure.ManifestAsset, int) {
	found := make(map[string]structure.ManifestAsset, len(creatives))
	missing := make(map[string]structure.ManifestAsset, len(creatives))
	logger.Debug("partioning creatives", slog.Int("totalCreatives", len(creatives)))
	filteredOut := 0
	for _, creative := range creatives {
		transcodeInfo, urlFound, err := api.valkeyStore.Get(creative.CreativeId)
		if err != nil {
			logger.Error("failed to get creative from store",
				slog.String("error", err.Error()),
				slog.String("creativeId", creative.CreativeId),
			)
			continue
		}
		if blacklisted, _ := api.valkeyStore.InBlackList(creative.MasterPlaylistUrl); blacklisted {
			logger.Debug("creative is in blacklist, skipping",
				slog.String("creativeId", creative.CreativeId),
				slog.String("masterPlaylistUrl", creative.MasterPlaylistUrl),
			)
			filteredOut++
			continue
		}
		if urlFound {
			if transcodeInfo.Status == "COMPLETED" {
				found[creative.CreativeId] = structure.ManifestAsset{
					CreativeId:        creative.CreativeId,
					MasterPlaylistUrl: transcodeInfo.Url,
					Source:            transcodeInfo.Source,
				}
			}
		} else {
			missing[creative.CreativeId] = structure.ManifestAsset{
				CreativeId:        creative.CreativeId,
				MasterPlaylistUrl: creative.MasterPlaylistUrl,
				Source:            transcodeInfo.Source,
			}
		}
	}
	return found, missing, filteredOut
}

type preIngestCreativeRequest struct {
	MediaUrls []string `json:"mediaUrls"`
}

type preIngestCreativeResponse struct {
	NotYetProcessed int `json:"notYetProcessed"`
}

func (api *API) HandlePreIngestCreatives(w http.ResponseWriter, r *http.Request) {
	_, span := otel.Tracer("api").Start(r.Context(), "PreIngestCreatives")
	defer span.End()
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	piRequest, err := readPreIngestRequest(r)
	if err != nil {
		logger.Error("failed to read request body", slog.String("error", err.Error()))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	amtMissing := api.findMissingAndDispatchJobsJson(&piRequest)
	resp := preIngestCreativeResponse{
		NotYetProcessed: amtMissing,
	}
	ret, err := json.Marshal(resp)
	if err != nil {
		logger.Error("failed to marshal response", slog.String("error", err.Error()))
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(ret)
	span.End()
}

func readPreIngestRequest(r *http.Request) (preIngestCreativeRequest, error) {
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		return preIngestCreativeRequest{}, err
	}
	var piRequest preIngestCreativeRequest
	if err := json.Unmarshal(bytes, &piRequest); err != nil {
		return piRequest, err
	}

	return piRequest, nil
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
			continue
		}
		for _, val := range v {
			query.Add(k, val)
		}
	}
	or.URL.RawQuery = query.Encode()
}
