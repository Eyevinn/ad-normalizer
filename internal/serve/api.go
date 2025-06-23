package serve

import "github.com/Eyevinn/ad-normalizer/internal/valkey"

type API struct {
	valkeyStore *valkey.ValkeyStore
}
