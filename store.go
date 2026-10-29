package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cacheVersion must be bumped whenever PullRequest gains a field, so older caches are rescanned.
const cacheVersion = 3

type Cache struct {
	Version        int                 `json:"version"`
	Identity       Identity            `json:"identity"`
	ScannedThrough time.Time           `json:"scannedThrough"`
	PullRequests   map[int]PullRequest `json:"pullRequests"`
}

type Store struct {
	directory string
}

func NewStore(repository string) Store {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	return Store{directory: filepath.Join(stateHome, "review-queue", strings.ReplaceAll(repository, "/", "-"))}
}

func (store Store) read(name string, target any) {
	contents, readError := os.ReadFile(filepath.Join(store.directory, name))
	if readError == nil {
		_ = json.Unmarshal(contents, target)
	}
}

func (store Store) write(name string, value any) error {
	if makeError := os.MkdirAll(store.directory, 0o755); makeError != nil {
		return makeError
	}
	contents, marshalError := json.Marshal(value)
	if marshalError != nil {
		return marshalError
	}
	path := filepath.Join(store.directory, name)
	if writeError := os.WriteFile(path+".tmp", contents, 0o644); writeError != nil {
		return writeError
	}
	return os.Rename(path+".tmp", path)
}

func (store Store) LoadCache() Cache {
	cache := Cache{PullRequests: map[int]PullRequest{}}
	store.read("cache.json", &cache)
	if cache.Version != cacheVersion {
		return Cache{Version: cacheVersion, Identity: cache.Identity, PullRequests: map[int]PullRequest{}}
	}
	return cache
}

func (store Store) SaveCache(cache Cache) error {
	return store.write("cache.json", cache)
}

// LoadDismissals maps a pull request number to when it was marked seen locally.
func (store Store) LoadDismissals() map[int]time.Time {
	dismissals := map[int]time.Time{}
	store.read("dismissed.json", &dismissals)
	return dismissals
}

func (store Store) SaveDismissals(dismissals map[int]time.Time) error {
	return store.write("dismissed.json", dismissals)
}
