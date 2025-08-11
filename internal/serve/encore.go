package serve

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
)

func (api *API) HandleEncoreCallback(w http.ResponseWriter, r *http.Request) {
	jobProgress := structure.EncoreJobProgress{}
	var requestBody []byte
	var err error
	defer r.Body.Close()
	if r.Header.Get("Content-Encoding") == "gzip" {
		requestBody, err = decompressGzip(r.Body)
		if err != nil {
			logger.Error("failed to decompress gzip request body", slog.String("error", err.Error()))
			http.Error(w, "Failed to decompress gzip request body", http.StatusInternalServerError)
			return
		}
	} else {
		requestBody, err = io.ReadAll(r.Body)
		if err != nil {
			logger.Error("failed to read request body", slog.String("error", err.Error()))
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
	}
	jsonDecoder := json.NewDecoder(bytes.NewBuffer(requestBody))
	err = jsonDecoder.Decode(&jobProgress)
	logger.Debug("Decoded Encore job progress",
		slog.String("jobId", jobProgress.JobId),
		slog.String("externalId", jobProgress.ExternalId),
		slog.String("status", jobProgress.Status),
	)
	if err != nil {
		logger.Error("failed to decode job progress", slog.String("error", err.Error()))
		http.Error(w, "Failed to decode job progress", http.StatusBadRequest)
		return
	}
	switch jobProgress.Status {
	case "SUCCESSFUL":
		err = api.handleTranscodeCompleted(&jobProgress)
	case "FAILED":
		err = api.handleTranscodeFailed(&jobProgress)
	case "IN_PROGRESS":
		err = api.handleTranscodeInProgress(&jobProgress)
	default:
		logger.Info("Job status does not match any known status", slog.String("status", jobProgress.Status))
		err = nil
	}
	if err != nil {
		logger.Error("failed to handle transcode job progress",
			slog.String("error", err.Error()),
			slog.String("jobId", jobProgress.JobId),
		)
		http.Error(w, "Failed to handle transcode job progress", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)

}

func (api *API) handleTranscodeInProgress(progress *structure.EncoreJobProgress) error {
	logger.Info("Transcoding progress updated",
		slog.String("creative ID", progress.ExternalId),
		slog.Int("progress", progress.Progress),
	)
	return nil
}

func (api *API) handleTranscodeFailed(progress *structure.EncoreJobProgress) error {
	err := api.valkeyStore.Delete(progress.ExternalId)
	return err
}

func (api *API) handleTranscodeCompleted(progress *structure.EncoreJobProgress) error {
	job, err := api.encoreHandler.GetEncoreJob(progress.JobId)
	if err != nil {
		logger.Error("failed to get encore job",
			slog.String("error", err.Error()),
			slog.String("jobId", progress.JobId),
		)
		return err
	}
	transcodeInfo, err := structure.TranscodeInfoFromEncoreJob(&job, api.jitPackage, api.assetServerUrl)
	if err != nil {
		logger.Error("failed to create transcode info from encore job",
			slog.String("error", err.Error()),
			slog.String("jobId", progress.JobId),
		)
		_ = api.valkeyStore.Delete(progress.ExternalId) // Something went wrong, remove the job from the store
		return nil
	}
	err = api.valkeyStore.Set(progress.ExternalId, transcodeInfo)
	if err != nil {
		logger.Error("failed to store transcode info",
			slog.String("error", err.Error()),
			slog.String("creativeId", progress.ExternalId),
		)
		_ = api.valkeyStore.Delete(progress.ExternalId) // Something went wrong, remove the job from the store
	}
	if !api.jitPackage {
		logger.Debug("JIT packaging is disabled, queueing packaing job", slog.String("creativeId", progress.ExternalId))
		packageInfo := structure.PackagingQueueMessage{
			JobId: progress.JobId,
			Url:   api.encoreUrl.JoinPath("encoreJobs", progress.JobId).String(),
		}
		err = api.valkeyStore.EnqueuePackagingJob(api.packageQueue, packageInfo)
	}
	return err
}
