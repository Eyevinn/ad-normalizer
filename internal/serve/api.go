package serve

import (
	"net/http"

	"github.com/Eyevinn/ad-normalizer/internal/store"
)

type API struct {
	valkeyStore    store.Store
	adServerUrl    string
	encoreUrl      string
	assetServerUrl string
}

func NewAPI(valkeyStore store.Store, adServerUrl, encoreUrl, assetServerUrl string) *API {
	return &API{
		valkeyStore:    valkeyStore,
		adServerUrl:    adServerUrl,
		encoreUrl:      encoreUrl,
		assetServerUrl: assetServerUrl,
	}
}

func (api *API) HandleVmap(w http.ResponseWriter, r *http.Request) {
	// Implement the logic to handle the VMAP request
	// This will likely involve fetching data from valkeyStore and formatting it as needed
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("VMAP response"))
}

func (api *API) HandleVast(w http.ResponseWriter, r *http.Request) {
	// Implement the logic to handle the VAST request
	// This will likely involve fetching data from valkeyStore and formatting it as needed
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("VAST response"))
}
