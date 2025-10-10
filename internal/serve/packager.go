package serve

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
)

func (api *API) HandlePackagingFailure(w http.ResponseWriter, r *http.Request) {
	body := structure.PackagingFailureBody{}
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "Failed to decode request body", http.StatusBadRequest)
		return
	}
	encoreJob, err := api.encoreHandler.GetEncoreJob(body.Message.JobId)
	if err != nil {
		http.Error(w, "Failed to get Encore job", http.StatusNotFound)
		return
	}
	if encoreJob.ExternalId == "" {
		http.Error(w, "Encore job does not have an external ID", http.StatusNotFound)
		return
	}
	if err := api.valkeyStore.Delete(encoreJob.ExternalId); err != nil {
		http.Error(w, "Failed to delete job from Valkey store", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	logger.Info("Packaging failure handled successfully", slog.String("creativeId", encoreJob.ExternalId))
}

func (api *API) HandlePackagingSuccess(w http.ResponseWriter, r *http.Request) {
	body := structure.PackagingSuccessBody{}
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "Failed to decode request body", http.StatusBadRequest)
		return
	}
	logger.Debug("getting Encore job for packaging success", slog.String("jobId", body.JobId))
	encoreJob, err := api.encoreHandler.GetEncoreJob(body.JobId)
	if err != nil {
		http.Error(w, "Failed to get Encore job", http.StatusNotFound)
		return
	}
	if encoreJob.ExternalId == "" {
		http.Error(w, "Encore job does not have an external ID", http.StatusNotFound)
		return
	}
	storeInfo, err := structure.TranscodeInfoFromEncoreJob(&encoreJob, api.jitPackage, api.assetServerUrl)
	if err != nil {
		logger.Error("Failed to create transcode info from Encore job",
			slog.String("error", err.Error()),
			slog.String("jobId", encoreJob.Id),
		)
		_ = api.valkeyStore.Delete(encoreJob.ExternalId) // Something went wrong, remove the job from the store
		http.Error(w, "Failed to create transcode info from Encore job", http.StatusInternalServerError)
		return
	}
	packageUrl := structure.CreatePackageUrl(api.assetServerUrl, body.OutputPath, "index")
	storeInfo.Url = packageUrl.String()
	storeInfo.Status = "COMPLETED"
	storeInfo.LastUpdate = time.Now().Unix()
	if err := api.valkeyStore.Set(encoreJob.ExternalId, storeInfo); err != nil {
		http.Error(w, "Failed to save job to Valkey store", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	logger.Info("Packaging success handled successfully",
		slog.String("creativeId", encoreJob.ExternalId),
		slog.String("packageUrl", packageUrl.String()),
	)
}
