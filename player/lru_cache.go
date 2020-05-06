package player

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/lbryio/lbry.go/v2/stream"
)

// LRUCacheOpts contains options for a cache. Size is max size in bytes.
type LRUCacheOpts struct {
	Path          string
	Size          uint64
	SweepInterval time.Duration
}

type lruCache struct {
	storage  *fsStorage
	lru      *lru.Cache
	resError chan error
}

// InitLRUCache initializes a LRU cache for chunks.
func InitLRUCache(opts *LRUCacheOpts) (ChunkCache, error) {
	storage, err := initFSStorage(opts.Path)
	if err != nil {
		return nil, err
	}

	if opts.Size == 0 {
		opts.Size = defaultMaxCacheSize
	}

	if opts.SweepInterval == 0 {
		opts.SweepInterval = time.Second * 120
	}

	onEvicted := func(key interface{}, value interface{}) {
		storage.remove(value)
	}

	lru, err := lru.NewWithEvict(int(opts.Size/ChunkSize), onEvicted)
	if err != nil {
		return nil, err
	}

	c := &lruCache{storage, lru, make(chan error, 1)}

	Logger.Infof("LRU cache of %vGB initialized at %v", opts.Size/1024/1024/1024, opts.Path)

	sweepTicker := time.NewTicker(opts.SweepInterval)
	metricsTicker := time.NewTicker(1 * time.Second)
	go func() {
		for {
			<-sweepTicker.C
			c.sweepChunks()
		}
	}()
	go func() {
		for {
			<-metricsTicker.C
			MtrCacheSize.Set(float64(c.Size()))
		}
	}()
	go func() {
		Logger.Infoln("restoring cache in memory...")
		err := c.reloadExistingChunks()
		if err != nil {
			Logger.Errorf("failed to restore cache in memory: %s", err.Error())
		} else {
			Logger.Infoln("done restoring cache in memory")
		}
		c.resError <- err
	}()

	return c, nil
}

func (c *lruCache) sweepChunks() {
	var removed int
	err := filepath.Walk(c.storage.path, func(path string, info os.FileInfo, err error) error {
		if c.storage.path == path {
			return nil
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !c.Has(info.Name()) {
			err := os.Remove(path)
			if err == nil {
				removed++
			} else {
				return err
			}
		}
		return nil
	})
	if err != nil {
		Logger.Errorf("error sweeping cache folder: %v", err)
	} else {
		Logger.Infof("swept cache folder, %v chunks removed", removed)
	}
}

func (c *lruCache) reloadExistingChunks() error {
	err := filepath.Walk(c.storage.path, func(path string, info os.FileInfo, err error) error {
		if c.storage.path == path {
			return nil
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			if len(info.Name()) != 1 {
				return fmt.Errorf("subfolder %v found inside cache folder", path)
			}
			return nil
		}
		if len(info.Name()) != stream.BlobHashHexLength {
			return fmt.Errorf("non-cache file found at path %v", path)
		}
		c.lru.Add(info.Name(), info.Name())
		return nil
	})
	return err
}

func (c *lruCache) Set(hash string, body []byte) (ReadableChunk, error) {
	var numWritten int
	Logger.Debugf("attempting to cache chunk %v", hash)
	chunkPath := c.storage.getPath(hash)
	err := os.MkdirAll(strings.Replace(chunkPath, hash, "", -1), 0700)
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(chunkPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		MtrCacheErrorCount.Inc()
		Logger.Debugf("chunk %v already exists on the local filesystem, not overwriting", hash)
	} else {
		numWritten, err = f.Write(body)
		defer f.Close()
		if err != nil {
			MtrCacheErrorCount.Inc()
			Logger.Errorf("error saving cache file %v: %v", chunkPath, err)
			return nil, err
		}

		err = f.Close()
		if err != nil {
			MtrCacheErrorCount.Inc()
			Logger.Errorf("error closing cache file %v: %v", chunkPath, err)
			return nil, err
		}
	}

	evicted := c.lru.Add(hash, hash)
	Logger.Debugf("cached chunk %v, retrieved: %vB, evicted: %v", hash, numWritten, evicted)

	return &cachedChunk{reflectedChunk{body}}, nil
}

func (c *lruCache) Has(hash string) bool {
	return c.lru.Contains(hash)
}

func (c *lruCache) Get(hash string) (ReadableChunk, bool) {
	if value, ok := c.lru.Get(hash); ok {
		f, err := c.storage.open(value)
		if err != nil {
			MtrCacheErrorCount.Inc()
			Logger.Errorf("chunk %v found in cache but couldn't open the file: %v", hash, err)
			c.lru.Remove(value)
			return nil, false
		}
		cb, err := initCachedChunk(f)
		if err != nil {
			Logger.Errorf("chunk %v found in cache but couldn't read the file: %v", hash, err)
			return nil, false
		}
		defer f.Close()
		return cb, true
	}

	Logger.Debugf("cache miss for chunk %v", hash)
	return nil, false
}

func (c *lruCache) Remove(hash string) {
	c.storage.remove(hash)
	c.lru.Remove(hash)
}

func (c *lruCache) Size() uint64 {
	return uint64(c.lru.Len()) * ChunkSize
}

// WaitForRestore blocks execution until cache restore is complete and returns the resulting error (if any).
func (c *lruCache) WaitForRestore() error {
	err := <-c.resError
	close(c.resError)
	return err
}
