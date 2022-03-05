package player

import (
	"fmt"
	"os"

	"github.com/OdyseeTeam/gody-cdn/cleanup"
	"github.com/OdyseeTeam/gody-cdn/configs"
	objectStore "github.com/OdyseeTeam/gody-cdn/store"
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
	cache *objectStore.CachingStore
	sf    *singleflight.Group
}
type decryptionData struct {
	key, iv []byte
}

func NewDecryptedCache(origin store.BlobStore, maxSizeInBytes int64) *DecryptedCache {
	stopper := stop.New()
	err := configs.Init("config.json")
	if err != nil {
		logrus.Fatalln(errors.FullTrace(err))
	}
	err = os.MkdirAll(configs.Configuration.DiskCache.Path, os.ModePerm)
	if err != nil {
		logrus.Fatal(errors.FullTrace(err))
	}
	ds := objectStore.NewDiskStore(configs.Configuration.DiskCache.Path, 2)
	localDB := configs.Configuration.LocalDB
	localDsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s", localDB.User, localDB.Password, localDB.Host, localDB.Database)
	dbs := objectStore.NewDBBackedStore(ds, localDsn)
	configs.Configuration.DiskCache.Size = fmt.Sprintf("%dB", maxSizeInBytes)
	go cleanup.SelfCleanup(dbs, dbs, stopper, configs.Configuration.DiskCache)

	baseFuncs := objectStore.BaseFuncs{
		GetFunc: func(hash string, extra interface{}) ([]byte, shared.BlobTrace, error) {
			//add miss metric logic here
			data, stack, err := origin.Get(hash)
			if extra != nil {
				dd := extra.(*decryptionData)
				chunk, err := stream.DecryptBlob(data, dd.key, dd.iv)
				return chunk, stack, err
			}

			return data, stack, err
		},
		HasFunc: origin.Has,
		PutFunc: func(hash string, object []byte) error {
			return errors.Err("not implemented")
		},
		DelFunc: origin.Delete,
	}
	finalStore := objectStore.NewCachingStoreV2("nvme-db-store", baseFuncs, dbs)
	//defer finalStore.Shutdown()
	//
	//interruptChan := make(chan os.Signal, 1)
	//signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)
	//<-interruptChan
	//// deferred shutdowns happen now
	//stopper.StopAndWait()

	h := &DecryptedCache{
		cache: finalStore,
		sf:    new(singleflight.Group),
	}

	return h
}

// GetSDBlob gets an sd blob. If it's not in the cache, it is fetched from the origin and cached.
// store.ErrBlobNotFound is returned if blob is not found.
func (h *DecryptedCache) GetSDBlob(hash string) (*stream.SDBlob, error) {
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
	item, _, err := h.cache.Get(hash, &dd)
	return item, err
}

func (h *DecryptedCache) IsCached(hash string) bool {
	has, _ := h.cache.Has(hash)
	return has
}
