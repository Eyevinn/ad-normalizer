package encore

import (
	"net/http"

	"github.com/Eyevinn/ad-normalizer/internal/structure"
)

type EncoreHandler struct {
	Client *http.Client
}

func (eh *EncoreHandler) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	return structure.EncoreJob{}, nil
}
