package encore

import (
	"net/http"
	"net/url"

	"github.com/Eyevinn/ad-normalizer/internal/structure"
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
	outputFolder, err := createOutputUrl(eh.outputBucket, creative.CreativeId) // TODO: Implement util function
	if err != nil {
		return structure.EncoreJob{}, err
	}
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

	return structure.EncoreJob{}, nil
}

func (eh *HttpEncoreHandler) submitJob(job structure.EncoreJob) (structure.EncoreJob, error) {
	// TODO: Set up the request, submit it, and handle the response
	return structure.EncoreJob{}, nil
}
