package engine

import (
	"context"
	"github.com/c2fo/vfs/v6/backend/gs"
	"github.com/c2fo/vfs/v6/backend/os"
	"heph/config"
	log "heph/hlog"
	"heph/utils"
	"heph/utils/hash"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"time"
)

type orderCacheContainer struct {
	cache   CacheConfig
	latency time.Duration
}

func (c orderCacheContainer) calculateLatency(ctx context.Context) (time.Duration, error) {
	if c.cache.Location.FileSystem().Scheme() == os.Scheme {
		return -1, nil
	}

	if c.cache.Location.FileSystem().Scheme() == gs.Scheme {
		client := http.DefaultClient

		bucketName := c.cache.Location.Volume()
		url := "https://storage.googleapis.com/" + bucketName

		var mean time.Duration
		for i := 0; i < 10; i++ {
			start := time.Now()
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return -1, err
			}
			req = req.WithContext(ctx)

			res, err := client.Do(req)
			if err != nil {
				return -1, err
			}
			_, _ = io.Copy(ioutil.Discard, res.Body)
			_ = res.Body.Close()

			duration := time.Since(start)
			mean += duration
			time.Sleep(time.Millisecond)
		}

		mean = mean / 10

		return mean, nil
	}

	return -1, nil
}

func orderCaches(ctx context.Context, caches []CacheConfig) []CacheConfig {
	if len(caches) <= 1 {
		return caches
	}

	orderedCaches := make([]orderCacheContainer, 0, len(caches))
	for _, cache := range caches {
		oc := orderCacheContainer{
			cache: cache,
		}

		var err error
		oc.latency, err = oc.calculateLatency(ctx)
		if err != nil {
			log.Errorf("latency: %v: skipping: %v", cache.Name, err)
		}

		orderedCaches = append(orderedCaches, oc)
	}

	sort.SliceStable(orderedCaches, func(i, j int) bool {
		return orderedCaches[i].latency < orderedCaches[j].latency
	})

	return utils.Map(orderedCaches, func(c orderCacheContainer) CacheConfig {
		return c.cache
	})
}

func (e *Engine) OrderedCaches(ctx context.Context) ([]CacheConfig, error) {
	if len(e.Config.Caches) <= 1 || e.Config.CacheOrder != config.CacheOrderLatency {
		return e.Config.Caches, nil
	}

	if e.orderedCaches != nil {
		return e.orderedCaches, nil
	}

	err := e.orderedCachesLock.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer e.orderedCachesLock.Unlock()

	if e.orderedCaches != nil {
		return e.orderedCaches, nil
	}

	h := hash.NewHash()
	h.I64(1)
	h.String(e.Config.Version.String)
	hash.HashArray(h, e.Config.Caches, func(c CacheConfig) string {
		return c.Name + "|" + c.URI
	})

	cacheHash := h.Sum()
	cachePath := e.tmpRoot("caches_order").Abs()

	names, err := utils.HashCache(cachePath, cacheHash, func() ([]string, error) {
		log.Infof("Measuring caches latency...")
		ordered := orderCaches(ctx, e.Config.Caches)

		return utils.Map(ordered, func(c CacheConfig) string {
			return c.Name
		}), nil
	})

	cacheMap := map[string]CacheConfig{}
	for _, c := range e.Config.Caches {
		cacheMap[c.Name] = c
	}

	hasSecondary := false
	ordered := make([]CacheConfig, 0, len(names))
	for _, name := range names {
		c := cacheMap[name]
		if c.Secondary {
			if hasSecondary {
				continue
			}
			hasSecondary = true
		}
		ordered = append(ordered, c)
	}

	e.orderedCaches = ordered

	return ordered, nil
}
