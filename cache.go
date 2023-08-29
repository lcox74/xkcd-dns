package main

import (
	"sync"
	"time"
)

const CACHE_EXPIRY = 5 * time.Minute

type cacheItem struct {
	value    Comic
	lastUsed time.Time
}

var cachePool = sync.Pool{
	New: func() interface{} {
		return &cacheItem{}
	},
}

type Cache struct {
	cache map[int]*cacheItem
	mu    sync.RWMutex
}

func NewCache[T any]() *Cache {
	c := &Cache{
		cache: make(map[int]*cacheItem),
		mu:    sync.RWMutex{},
	}

	go c.cleanExpiredEntries()

	return c
}

func (c *Cache) cleanExpiredEntries() {
	for {
		for k, v := range c.cache {
			if time.Since(v.lastUsed) > CACHE_EXPIRY {
				c.mu.Lock()

				// Delete the entry from the map and return it to the pool
				delete(c.cache, k)
				cachePool.Put(v)

				c.mu.Unlock()
			}
		}

		time.Sleep(1 * time.Minute)
	}
}

func (c *Cache) Get(key int) (Comic, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.cache[key]
	if ok {
		v.lastUsed = time.Now()
		c.cache[key] = v
		return v.value, ok
	}

	return Comic{}, ok
}

func (c *Cache) Set(key int, value Comic) {
	c.mu.Lock()
	defer c.mu.Unlock()

	newRecord := cachePool.Get().(*cacheItem)
	newRecord.value = value
	newRecord.lastUsed = time.Now()
	c.cache[key] = newRecord
}
