package encore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/Eyevinn/ad-normalizer/internal/util"
	osaasclient "github.com/EyevinnOSC/client-go"
)

type EncoreHandler interface {
	CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error)
	GetEncoreJob(jobId string) (structure.EncoreJob, error)
}

type HttpEncoreHandler struct {
	Client             *http.Client
	encoreUrl          url.URL
	transcodingProfile string
	oscContext         *osaasclient.Context
	outputBucket       url.URL
	rootUrl            url.URL
}

func NewHttpEncoreHandler(
	client *http.Client,
	encoreUrl url.URL,
	transcodingProfile string,
	oscContext *osaasclient.Context,
	outputBucket url.URL,
	rootUrl url.URL,
) *HttpEncoreHandler {
	return &HttpEncoreHandler{
		Client:             client,
		encoreUrl:          encoreUrl,
		transcodingProfile: transcodingProfile,
		oscContext:         oscContext,
		outputBucket:       outputBucket,
		rootUrl:            rootUrl,
	}
}

func (eh *HttpEncoreHandler) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	outputFolder := util.CreateOutputUrl(
		eh.outputBucket,
		creative.CreativeId,
	)
	callbackUrl := eh.rootUrl.JoinPath("/encoreCallback").String()
	job := structure.EncoreJob{
		ExternalId:          creative.CreativeId,
		Profile:             eh.transcodingProfile,
		OutputFolder:        outputFolder,
		BaseName:            creative.CreativeId,
		ProgressCallbackUri: callbackUrl,
		Inputs: []structure.EncoreInput{
			{
				Uri:       creative.MasterPlaylistUrl,
				SeekTo:    0.0,
				CopyTs:    true,
				MediaType: "AudioVideo",
			},
		},
	}

	submitted, err := eh.submitJob(job)
	if err != nil {
		logger.Error("Failed to submit Encore job", slog.String("error", err.Error()))
	}
	return submitted, nil
}

func (eh *HttpEncoreHandler) GetEncoreJob(jobId string) (structure.EncoreJob, error) {
	logger.Debug("Getting Encore job", slog.String("jobId", jobId))
	job := structure.EncoreJob{} // init zero value
	jobRequest, err := http.NewRequest("GET", eh.encoreUrl.JoinPath("/encoreJobs", jobId).String(), nil)
	logger.Debug("Created Encore job request", slog.String("url", jobRequest.URL.String()))
	if err != nil {
		logger.Error("Failed to create Encore job request", slog.String("error", err.Error()))
		return job, err
	}
	jobRequest.Header.Set("Accept", "application/hal+json")
	jobRequest.Header.Set("Content-Type", "application/json")
	if eh.oscContext != nil && eh.oscContext.PersonalAccessToken != "" {
		sat, err := eh.oscContext.GetServiceAccessToken("encore")
		if err != nil {
			logger.Error("Failed to get Service Access Token for Encore", slog.String("error", err.Error()))
			return job, fmt.Errorf("failed to get Service Access Token for Encore: %w", err)
		}
		jobRequest.Header.Set("x-jwt", "Bearer "+sat)
	}
	res, err := eh.Client.Do(jobRequest)
	if err != nil {
		logger.Error("Failed to get Encore job", slog.String("error", err.Error()))
		return job, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		logger.Error("Failed to get Encore job", slog.Int("statusCode", res.StatusCode))
		return job, fmt.Errorf("failed to get Encore job, status code: %d", res.StatusCode)
	}
	jsonDecoder := json.NewDecoder(res.Body)
	err = jsonDecoder.Decode(&job)
	if err != nil {
		logger.Error("Failed to decode Encore job response", slog.String("error", err.Error()))
		return job, fmt.Errorf("failed to decode Encore job response")
	}
	return job, nil
}

func (eh *HttpEncoreHandler) submitJob(job structure.EncoreJob) (structure.EncoreJob, error) {
	serialized, err := json.Marshal(job)
	if err != nil {
		logger.Error("Failed to serialize Encore job", slog.String("error", err.Error()))
		return structure.EncoreJob{}, err
	}
	jobRequest, err := http.NewRequest("POST",
		eh.encoreUrl.JoinPath("/encoreJobs").String(),
		bytes.NewBuffer(serialized),
	)

	if err != nil {
		logger.Error("Failed to create Encore job request", slog.String("error", err.Error()))
		return structure.EncoreJob{}, err
	}

	jobRequest.Header.Set("Content-Type", "application/json")
	jobRequest.Header.Set("Accept", "application/hal+json")
	if eh.oscContext != nil && eh.oscContext.PersonalAccessToken != "" {
		sat, err := eh.oscContext.GetServiceAccessToken("encore")
		if err != nil {
			logger.Error("Failed to get Service Access Token for Encore", slog.String("error", err.Error()))
			return job, fmt.Errorf("failed to get Service Access Token for Encore: %w", err)
		}
		jobRequest.Header.Set("x-jwt", "Bearer "+sat)
	}
	resp, err := eh.Client.Do(jobRequest)
	if err != nil {
		logger.Error("Failed to submit Encore job", slog.String("error", err.Error()))
		return structure.EncoreJob{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respStr, _ := io.ReadAll(resp.Body)
		logger.Error("Failed to submit Encore job",
			slog.Int("statusCode", resp.StatusCode),
			slog.String("error", string(respStr)),
		)
		return structure.EncoreJob{}, fmt.Errorf("failed to submit Encore job, status code: %d", resp.StatusCode)
	}
	newJob := structure.EncoreJob{}
	jsonDecoder := json.NewDecoder(resp.Body)
	err = jsonDecoder.Decode(&newJob)
	if err != nil {
		logger.Error("Failed to decode Encore job response", slog.String("error", err.Error()))
		return structure.EncoreJob{}, fmt.Errorf("failed to decode Encore job response")
	}
	logger.Info("Successfully submitted Encore job", slog.String("jobId", newJob.ExternalId))
	return newJob, nil
}
