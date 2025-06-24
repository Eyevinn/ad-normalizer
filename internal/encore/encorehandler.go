package encore

import (
	"net/http"

	"github.com/Eyevinn/ad-normalizer/internal/structure"
)

type EncoreHandler interface {
	CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error)
}

type HttpEncoreHandler struct {
	Client *http.Client
}

func (eh *HttpEncoreHandler) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	return structure.EncoreJob{}, nil
}
