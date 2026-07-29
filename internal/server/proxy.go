package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/charmbracelet/log"

	"arca/internal/blob"
	"arca/internal/format"
	"arca/internal/maven"
	"arca/internal/proxy"
	"arca/internal/store"
	"arca/internal/store/models"
)

const (
	accessTouchInterval = time.Hour
	maxRewriteBytes     = 64 << 20
)

var errRemoteMissing = errors.New("server: the upstream does not have this path")

func remoteFor(repository *models.Repository) proxy.Remote {
	return proxy.Remote{
		BaseURL:  repository.RemoteURL,
		Username: repository.RemoteUsername,
		Password: repository.RemotePassword,
	}
}

func cacheExpiry(layout format.Layout, repository *models.Repository, path string) int64 {
	if !layout.Mutable(path) {
		return 0
	}
	return store.NowMillis() + repository.MetadataTTLSeconds*1000
}

// proxyFetch names both ends of a cached upstream file. They are the same path
// for Maven, whose storage layout is the wire layout, and differ for formats
// like npm that address the registry over one URL shape and store components
// under another.
type proxyFetch struct {
	CachePath  string
	RemotePath string
	Accept     string
	// Rewrite adapts an upstream document before it is cached. Unlike the npm
	// packument rewrite this one does not depend on the request, so it runs once
	// on the way in and the stored digests describe what is actually served.
	Rewrite func(document []byte) ([]byte, error)
}

func pathFetch(path string) proxyFetch {
	return proxyFetch{CachePath: path, RemotePath: path}
}

func (s *Server) serveProxyRead(writer http.ResponseWriter, request *http.Request, repository *models.Repository, path string) {
	s.serveProxyFetch(writer, request, repository, pathFetch(path))
}

func (s *Server) serveProxyFetch(writer http.ResponseWriter, request *http.Request, repository *models.Repository, fetch proxyFetch) {
	// Two passes: a cached row whose file has gone missing is dropped while
	// serving it, so the second pass refetches rather than answering 404 from a
	// record that no longer has any bytes behind it.
	for attempt := 0; attempt < 2; attempt++ {
		asset, err := s.proxyAsset(repository, fetch)
		switch {
		case errors.Is(err, errRemoteMissing):
			s.serveProxyMiss(writer, repository, fetch.CachePath)
			return
		case err != nil:
			log.Errorf("proxying %s/%s failed: %v", repository.Name, fetch.CachePath, err)
			plain(writer, http.StatusBadGateway, "Upstream fetch failed")
			return
		}

		if s.serveCachedAsset(writer, request, repository, asset) {
			return
		}
	}

	plain(writer, http.StatusBadGateway, "Cached file went missing")
}

// proxyAsset resolves the cache row a proxy repository should answer with,
// asking the upstream when the cache is cold or stale and falling back to a
// stale copy when that fails.
func (s *Server) proxyAsset(repository *models.Repository, fetch proxyFetch) (*models.Asset, error) {
	if !format.IsSafe(fetch.CachePath) {
		return nil, errRemoteMissing
	}

	cached, err := s.store.FindAsset(repository.ID, fetch.CachePath)
	if err != nil {
		cached = nil
	}

	if cached != nil && !cached.IsStale(store.NowMillis()) {
		return cached, nil
	}

	if cached == nil {
		if _, err := s.store.FindRemoteMiss(repository.ID, fetch.CachePath); err == nil {
			return nil, errRemoteMissing
		}
	}

	refreshed, err := s.fetchThroughProxy(repository, fetch, cached)
	switch {
	case err == nil:
		return refreshed, nil
	case cached != nil:
		log.Warnf("serving a stale copy of %s/%s: %v", repository.Name, fetch.CachePath, err)
		return cached, nil
	default:
		return nil, err
	}
}

// serveCachedAsset reports whether the cached copy could be served. A row whose
// file has gone missing is dropped rather than left to answer every future
// request with a 404: eviction removes rows before files, so a fetch that lands
// in that window can outlive its own blob.
func (s *Server) serveCachedAsset(writer http.ResponseWriter, request *http.Request, repository *models.Repository, asset *models.Asset) bool {
	s.recordCacheHit(request, repository, asset)

	if s.writeAsset(writer, request, asset) {
		return true
	}

	s.dropDanglingAsset(asset)
	return false
}

// recordCacheHit keeps a served file out of the eviction sweep and visible in
// traffic insights, whether it was streamed straight through or transformed on
// the way out.
func (s *Server) recordCacheHit(request *http.Request, repository *models.Repository, asset *models.Asset) {
	if request.Method == http.MethodGet {
		s.recordAssetTraffic(request, models.TrafficDownload, repository, asset)
	}
	if store.NowMillis()-asset.LastAccessedAt >= accessTouchInterval.Milliseconds() {
		if err := s.store.MarkAssetAccessed(asset.ID); err != nil {
			log.Errorf("recording a cache access failed: %v", err)
		}
	}
}

func (s *Server) dropDanglingAsset(asset *models.Asset) {
	log.Warnf("cached file %s is missing from storage, dropping the record", asset.Path)
	if _, err := s.store.DeleteAsset(asset.RepositoryID, asset.Path); err != nil {
		log.Errorf("dropping a dangling cache record failed: %v", err)
	}
}

// serveProxyMiss answers a path the upstream does not have. A checksum whose
// artifact is already cached is answered from that copy, which keeps clients
// that insist on one working against upstreams that publish only some of them.
func (s *Server) serveProxyMiss(writer http.ResponseWriter, repository *models.Repository, path string) {
	base, checksum := maven.SplitChecksum(path)
	if checksum != "" {
		if parent, err := s.store.FindAsset(repository.ID, base); err == nil {
			writer.Header().Set("Content-Type", "text/plain")
			writer.Header().Set("Cache-Control", "no-cache")
			fmt.Fprint(writer, parent.Checksum(checksum))
			return
		}
	}

	plain(writer, http.StatusNotFound, "Not Found")
}

func (s *Server) fetchThroughProxy(repository *models.Repository, fetch proxyFetch, cached *models.Asset) (*models.Asset, error) {
	err, _ := s.flight.Do(repository.ID+"/"+fetch.CachePath, func() error {
		return s.refreshFromRemote(repository, fetch, cached)
	})
	if err != nil {
		return nil, err
	}

	// Followers of a shared fetch read the result the leader wrote rather than
	// the row they looked up before waiting.
	return s.store.FindAsset(repository.ID, fetch.CachePath)
}

func (s *Server) refreshFromRemote(repository *models.Repository, fetch proxyFetch, cached *models.Asset) error {
	// The fetch is detached from the request so a client that walks away does
	// not cancel a download other callers are already waiting on.
	ctx, cancel := context.WithTimeout(context.Background(), s.config.ProxyTimeout)
	defer cancel()

	var conditional proxy.Conditional
	if cached != nil {
		conditional = proxy.Conditional{ETag: cached.RemoteETag, LastModified: cached.RemoteLastModified}
	}

	remote := remoteFor(repository)
	remote.Accept = fetch.Accept

	response, err := s.remote.Fetch(ctx, remote, fetch.RemotePath, conditional)
	if err != nil {
		return err
	}
	defer response.Close()

	switch {
	case response.Status == http.StatusOK:
		return s.storeRemoteAsset(repository, fetch, response)

	case response.Status == http.StatusNotModified && cached != nil:
		return s.store.MarkAssetRefreshed(
			cached.ID,
			cacheExpiry(layoutFor(repository), repository, fetch.CachePath),
			orKeep(response.ETag, cached.RemoteETag),
			orKeep(response.LastModified, cached.RemoteLastModified),
		)

	case response.Status == http.StatusNotFound || response.Status == http.StatusGone:
		expiresAt := store.NowMillis() + repository.NegativeTTLSeconds*1000
		if err := s.store.RecordRemoteMiss(repository.ID, fetch.CachePath, response.Status, expiresAt); err != nil {
			log.Errorf("recording an upstream miss failed: %v", err)
		}
		return errRemoteMissing

	default:
		return fmt.Errorf("upstream answered %d", response.Status)
	}
}

func (s *Server) storeRemoteAsset(repository *models.Repository, fetch proxyFetch, response *proxy.Response) error {
	now := store.NowMillis()

	body := io.Reader(response.Body)
	if fetch.Rewrite != nil {
		rewritten, err := rewriteRemoteDocument(fetch, response.Body)
		if err != nil {
			return err
		}
		body = rewritten
	}

	_, err := s.storeUpload(repository, fetch.CachePath, body, uploadDetails{
		FetchedAt:          now,
		ExpiresAt:          cacheExpiry(layoutFor(repository), repository, fetch.CachePath),
		LastAccessedAt:     now,
		RemoteETag:         response.ETag,
		RemoteLastModified: response.LastModified,
	})
	if errors.Is(err, blob.ErrTooLarge) {
		return fmt.Errorf("upstream artifact exceeds the %d byte limit", s.config.MaxUploadBytes)
	}
	if err != nil {
		return err
	}

	return s.store.ClearRemoteMiss(repository.ID, fetch.CachePath)
}

// rewriteRemoteDocument buffers the upstream body so it can be transformed.
// Only documents small enough to hold are eligible: a rewrite that cannot run
// fails the fetch rather than caching bytes nobody inspected.
func rewriteRemoteDocument(fetch proxyFetch, body io.Reader) (io.Reader, error) {
	document, err := io.ReadAll(io.LimitReader(body, maxRewriteBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(document)) > maxRewriteBytes {
		return nil, fmt.Errorf("upstream document exceeds the %d byte rewrite limit", maxRewriteBytes)
	}

	rewritten, err := fetch.Rewrite(document)
	if err != nil {
		return nil, fmt.Errorf("rewriting %s: %w", fetch.CachePath, err)
	}
	return bytes.NewReader(rewritten), nil
}

// purgeIdleCaches drops cached files that no build has asked for within their
// repository's retention window. Only proxies are swept: a hosted artifact is
// the only copy there is, however long it sits unread.
func (s *Server) purgeIdleCaches(now time.Time) {
	repositories, err := s.store.EvictableRepositories()
	if err != nil {
		log.Errorf("listing proxy repositories failed: %v", err)
		return
	}

	for _, repository := range repositories {
		cutoff := now.AddDate(0, 0, -int(repository.CacheRetentionDays)).UnixMilli()

		keys, err := s.store.DeleteIdleAssets(repository.ID, cutoff)
		if err != nil {
			log.Errorf("evicting the %s cache failed: %v", repository.Name, err)
			continue
		}
		if len(keys) == 0 {
			continue
		}
		if err := s.blobs.Delete(keys...); err != nil {
			log.Errorf("removing evicted files of %s failed: %v", repository.Name, err)
		}

		log.Infof("evicted %d files from the %s cache (retention %d days)", len(keys), repository.Name, repository.CacheRetentionDays)
	}
}

func orKeep(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
