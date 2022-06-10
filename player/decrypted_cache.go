package player

import (
	"fmt"
	"os"

	"github.com/OdyseeTeam/gody-cdn/cleanup"
	"github.com/OdyseeTeam/gody-cdn/configs"
	objectStore "github.com/OdyseeTeam/gody-cdn/store"
	"github.com/OdyseeTeam/player-server/internal/metrics"
	"github.com/lbryio/lbry.go/v2/extras/errors"
	"github.com/lbryio/lbry.go/v2/extras/stop"
	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/sirupsen/logrus"

	"github.com/lbryio/reflector.go/shared"
	"github.com/lbryio/reflector.go/store"

	"golang.org/x/sync/singleflight"
)

// DecryptedCache Stores and retrieves unencrypted blobs on disk.
type DecryptedCache struct {
	cache   *objectStore.CachingStore
	sf      *singleflight.Group
	stopper *stop.Group
}

type decryptionData struct {
	key, iv []byte
}

func NewDecryptedCache(origin store.BlobStore) *DecryptedCache {
	stopper := stop.New()
	err := configs.Init("config.json")
	if err != nil {
		logrus.Fatalln(errors.FullTrace(err))
	}
	err = os.MkdirAll(configs.Configuration.DiskCache.Path, os.ModePerm)
	if err != nil {
		logrus.Fatal(errors.FullTrace(err))
	}
	ds, err := objectStore.NewDiskStore(configs.Configuration.DiskCache.Path, 2)
	if err != nil {
		logrus.Fatal(errors.FullTrace(err))
	}
	localDB := configs.Configuration.LocalDB
	localDsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s", localDB.User, localDB.Password, localDB.Host, localDB.Database)
	dbs := objectStore.NewDBBackedStore(ds, localDsn)
	go cleanup.SelfCleanup(dbs, dbs, stopper, configs.Configuration.DiskCache)

	baseFuncs := objectStore.BaseFuncs{
		GetFunc: func(hash string, extra interface{}) ([]byte, shared.BlobTrace, error) {
			//add miss metric logic here
			metrics.DecryptedCacheRequestCount.WithLabelValues("object", "miss").Inc()
			data, stack, err := origin.Get(hash)
			if extra != nil {
				dd := extra.(*decryptionData)
				chunk, err := stream.DecryptBlob(data, dd.key, dd.iv)
				return chunk, stack, err
			}

			return data, stack, err
		},
		HasFunc: func(hash string, extra interface{}) (bool, error) {
			return origin.Has(hash)
		},
		PutFunc: func(hash string, object []byte, extra interface{}) error {
			return errors.Err("not implemented")
		},
		DelFunc: func(hash string, extra interface{}) error {
			return origin.Delete(hash)
		},
	}
	finalStore := objectStore.NewCachingStoreV2("nvme-db-store", baseFuncs, dbs)

	h := &DecryptedCache{
		cache:   finalStore,
		sf:      new(singleflight.Group),
		stopper: stopper,
	}

	return h
}

// GetSDBlob gets an sd blob. If it's not in the cache, it is fetched from the origin and cached.
// store.ErrBlobNotFound is returned if blob is not found.
func (h *DecryptedCache) GetSDBlob(hash string) (*stream.SDBlob, error) {
	metrics.DecryptedCacheRequestCount.WithLabelValues("sdblob", "total").Inc()
	cached, _, err := h.cache.Get(hash, nil)
	if err != nil {
		return nil, err
	}

	var sd stream.SDBlob
	err = sd.FromBlob(cached)
	return &sd, errors.Err(err)
}

// GetChunk gets a decrypted stream chunk. If chunk is not cached, it is fetched from origin
// and decrypted.
func (h *DecryptedCache) GetChunk(hash string, key, iv []byte) (ReadableChunk, error) {
	dd := decryptionData{
		key: key,
		iv:  iv,
	}
	metrics.DecryptedCacheRequestCount.WithLabelValues("blob", "total").Inc()
	item, _, err := h.cache.Get(hash, &dd)
	return item, err
}

func (h *DecryptedCache) IsCached(hash string) bool {
	has, _ := h.cache.Has(hash, nil)
	return has
}

func (h *DecryptedCache) Shutdown() {
	h.cache.Shutdown()
	h.stopper.StopAndWait()
}
