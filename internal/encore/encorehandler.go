package encore

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/Eyevinn/ad-normalizer/internal/util"
)

type EncoreHandler interface {
	CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error)
}

type HttpEncoreHandler struct {
	Client             *http.Client
	encoreUrl          url.URL
	transcodingProfile string
	oscToken           string
	outputBucket       url.URL
	rootUrl            url.URL
}

func NewHttpEncoreHandler(
	client *http.Client,
	encoreUrl url.URL,
	transcodingProfile string,
	oscToken string,
) *HttpEncoreHandler {
	return &HttpEncoreHandler{
		Client:             client,
		encoreUrl:          encoreUrl,
		transcodingProfile: transcodingProfile,
		oscToken:           oscToken,
	}
}

func (eh *HttpEncoreHandler) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	outputFolder := util.CreateOutputUrl(
		eh.outputBucket,
		creative.CreativeId,
	)
	job := structure.EncoreJob{
		ExternalId:          creative.CreativeId,
		Profile:             eh.transcodingProfile,
		OutputFolder:        outputFolder,
		BaseName:            creative.CreativeId,
		ProgressCallbackUri: eh.rootUrl.String() + "/encoreCallback",
		Inputs: []structure.EncoreInput{
			{
				Uri:       creative.MasterPlaylistUrl,
				SeekTo:    0,
				CopyTs:    true,
				MediaType: "AudioVideo",
			},
		},
	}

	submitted, err := eh.submitJob(job)
	if err != nil {
		logger.Error("Failed to submit Encore job", slog.String("err", err.Error()))
	}
	return submitted, nil
}

func (eh *HttpEncoreHandler) submitJob(job structure.EncoreJob) (structure.EncoreJob, error) {
	// TODO: Set up the request, submit it, and handle the response
	jobRequest, err := http.NewRequest("POST", eh.encoreUrl.JoinPath("/encoreJobs").String(), nil)
	if err != nil {
		logger.Error("Failed to create Encore job request", slog.String("err", err.Error()))
		return structure.EncoreJob{}, err
	}
	jobRequest.Header.Set("Content-Type", "application/json")
	jobRequest.Header.Set("Accept", "application/hal+json")
	if eh.oscToken != "" {
		// TODO: Actually get a SAT
		jobRequest.Header.Set("x-jwt", eh.oscToken)
	}
	resp, err := eh.Client.Do(jobRequest)
	if err != nil {
		logger.Error("Failed to submit Encore job", slog.String("err", err.Error()))
		return structure.EncoreJob{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		logger.Error("Failed to submit Encore job", slog.Int("statusCode", resp.StatusCode))
		return structure.EncoreJob{}, fmt.Errorf("failed to submit Encore job, status code: %d", resp.StatusCode)
	}
	var newJob structure.EncoreJob
	jsonDecoder := json.NewDecoder(resp.Body)
	err = jsonDecoder.Decode(&newJob)
	if err != nil {
		logger.Error("Failed to decode Encore job response", slog.String("err", err.Error()))
		return structure.EncoreJob{}, fmt.Errorf("failed to decode Encore job response")
	}
	logger.Info("Successfully submitted Encore job", slog.String("jobId", newJob.ExternalId))
	return structure.EncoreJob{}, nil
}
