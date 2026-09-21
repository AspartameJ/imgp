package registry

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/util"
	"gitcode.com/DonaldTom/imgp/internal/version"
)

var userAgent = "imgp/" + version.Version

// Client handles registry communication with mirror fallback and auth.
type Client struct {
	cfg      *config.Config
	username string
	password string
	insecure bool
	retry    int
}

// NewClient creates a registry Client from the given config.
func NewClient(cfg *config.Config) *Client {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	r := 2
	if cfg.Retry >= 0 {
		r = cfg.Retry
	}
	return &Client{cfg: cfg, retry: r}
}

// WithAuth sets registry credentials.
func (c *Client) WithAuth(username, password string) *Client {
	clone := *c
	clone.username = username
	clone.password = password
	return &clone
}

// WithInsecure sets whether to skip TLS verification.
func (c *Client) WithInsecure(v bool) *Client {
	clone := *c
	clone.insecure = v
	return &clone
}

// WithRetry sets the max retry count for fetch operations.
func (c *Client) WithRetry(n int) *Client {
	clone := *c
	if n >= 0 {
		clone.retry = n
	}
	return &clone
}

var transportCache sync.Map // key: string(host+insecure), value: *http.Transport

func (c *Client) transport(reg name.Registry) http.RoundTripper {
	insecure := c.insecure
	regName := normalizeRegistry(reg.Name())
	for _, ir := range c.cfg.InsecureRegistries {
		if regName == ir || strings.HasSuffix(regName, "."+ir) {
			insecure = true
			break
		}
	}
	cacheKey := regName + ":" + strconv.FormatBool(insecure)
	if cached, ok := transportCache.Load(cacheKey); ok {
		if t, ok := cached.(*http.Transport); ok {
			return t
		}
	}

	dt, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	t := dt.Clone()
	t.MaxConnsPerHost = 100
	t.ResponseHeaderTimeout = 30 * time.Second
	t.TLSHandshakeTimeout = 10 * time.Second
	t.DialContext = (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	if insecure {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	transportCache.Store(cacheKey, t)
	return t
}

func (c *Client) authenticator(reg name.Registry) authn.Authenticator {
	if c.username != "" {
		return authn.FromConfig(authn.AuthConfig{
			Username: c.username,
			Password: c.password,
		})
	}
	regName := normalizeRegistry(reg.Name())
	if a := c.resolveAuth(regName); a != nil {
		return authn.FromConfig(*a)
	}
	if a := c.resolveAuth("*"); a != nil {
		return authn.FromConfig(*a)
	}
	return authn.Anonymous
}

func (c *Client) resolveAuth(regName string) *authn.AuthConfig {
	a, ok := c.cfg.Auths[regName]
	if !ok {
		return nil
	}
	password := a.Password
	if a.PasswordEnv != "" {
		if p, ok := os.LookupEnv(a.PasswordEnv); ok && p != "" {
			password = p
		} else {
			fmt.Fprintf(os.Stderr, "WARN: password environment variable %q is unset or empty\n", a.PasswordEnv)
		}
	}
	return &authn.AuthConfig{Username: a.Username, Password: password}
}

func parsePlatform(platform string) *v1.Platform {
	if platform == "" {
		return nil
	}
	p := &v1.Platform{}
	parts := strings.Split(platform, "/")
	switch len(parts) {
	case 2:
		p.OS = parts[0]
		p.Architecture = parts[1]
	case 3:
		p.OS = parts[0]
		p.Architecture = parts[1]
		p.Variant = parts[2]
	default:
		p.OS = "linux"
		p.Architecture = platform
	}
	return p
}

// NewLayerFetcher returns a function that opens a layer download stream
// with per-call context support, allowing per-layer timeouts and resumable
// downloads via HTTP Range requests. When offset > 0, the request asks the
// registry for the blob content starting at that byte offset.
func (c *Client) NewLayerFetcher(ref name.Reference) func(ctx context.Context, digestHex string, offset int64) (io.ReadCloser, error) {
	repo := ref.Context()
	return func(ctx context.Context, digestHex string, offset int64) (io.ReadCloser, error) {
		hex := strings.TrimPrefix(digestHex, "sha256:")
		reg := repo.Registry
		u := url.URL{
			Scheme: reg.Scheme(),
			Host:   reg.RegistryStr(),
			Path:   fmt.Sprintf("/v2/%s/blobs/sha256:%s", repo.RepositoryStr(), hex),
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		if offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		req.Header.Set("User-Agent", userAgent)

		httpClient := &http.Client{Transport: c.transport(reg)}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		switch resp.StatusCode {
		case http.StatusPartialContent:
			return resp.Body, nil
		case http.StatusOK:
			// Server ignored our Range header and returned the full body.
			if offset > 0 {
				// Skip the bytes we already have so the caller can append safely.
				if _, err := io.CopyN(io.Discard, resp.Body, offset); err != nil {
					resp.Body.Close()
					return nil, fmt.Errorf("fetch layer: discard prefix: %w", err)
				}
			}
			return resp.Body, nil
		default:
			resp.Body.Close()
			return nil, fmt.Errorf("fetch layer: unexpected status %d", resp.StatusCode)
		}
	}
}

func normalizeRegistry(reg string) string {
	reg = strings.TrimSuffix(reg, ":443")
	if reg == "index.docker.io" || reg == "docker.io" || reg == "registry-1.docker.io" {
		return "docker.io"
	}
	return reg
}

// FetchImage retrieves an image from the registry with mirror fallback and retry.
func (c *Client) FetchImage(ctx context.Context, image, platform string) (v1.Image, name.Reference, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, nil, fmt.Errorf("parse image reference: %w", err)
	}

	refsToTry := c.resolveRefs(ref)
	plat := parsePlatform(platform)
	origAuth := c.authenticator(ref.Context().Registry)

	skipRef := make(map[string]bool)
	var lastErr error
	for attempt := 0; attempt <= c.retry; attempt++ {
		if attempt > 0 {
			if err := util.Backoff(ctx, attempt); err != nil {
				return nil, nil, err
			}
		}

		var errs []string
		var anyRetryable bool
		for _, r := range refsToTry {
			if skipRef[r.String()] {
				continue
			}
			img, err := c.tryReference(ctx, r, origAuth, plat)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", r.String(), err))
				lastErr = err
				if util.IsRetryable(err) {
					anyRetryable = true
				} else {
					skipRef[r.String()] = true
				}
				continue
			}
			return img, r, nil
		}

		if !anyRetryable || attempt == c.retry {
			return nil, nil, fmt.Errorf("all registries failed:\n  %s", strings.Join(errs, "\n  "))
		}
	}
	return nil, nil, fmt.Errorf("all registries failed: %v", lastErr)
}

func (c *Client) resolveRefs(ref name.Reference) []name.Reference {
	seen := map[string]bool{ref.String(): true}
	var refs []name.Reference

	regStr := normalizeRegistry(ref.Context().RegistryStr())
	if mirrors, ok := c.cfg.MirrorMap[regStr]; ok {
		repoPath := ref.Context().RepositoryStr()

		switch r := ref.(type) {
		case name.Tag:
			tagStr := r.TagStr()
			for _, m := range mirrors {
				mirrorTag, err := name.NewTag(fmt.Sprintf("%s/%s:%s", m, repoPath, tagStr))
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARN: invalid mirror ref %q: %v\n", m, err)
					continue
				}
				s := mirrorTag.String()
				if !seen[s] {
					seen[s] = true
					refs = append(refs, mirrorTag)
				}
			}
		case name.Digest:
			digestStr := r.DigestStr()
			for _, m := range mirrors {
				mirrorDigest, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", m, repoPath, digestStr))
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARN: invalid mirror ref %q: %v\n", m, err)
					continue
				}
				s := mirrorDigest.String()
				if !seen[s] {
					seen[s] = true
					refs = append(refs, mirrorDigest)
				}
			}
		}
	}

	return append(refs, ref)
}

func (c *Client) tryReference(ctx context.Context, r name.Reference, origAuth authn.Authenticator, plat *v1.Platform) (v1.Image, error) {
	reg := r.Context().Registry
	auth := c.authenticator(reg)
	if c.username == "" && auth == authn.Anonymous && origAuth != authn.Anonymous {
		auth = origAuth
	}
	opts := []remote.Option{
		remote.WithAuth(auth),
		remote.WithTransport(c.transport(reg)),
		remote.WithContext(ctx),
		remote.WithUserAgent(userAgent),
	}
	if plat != nil {
		opts = append(opts, remote.WithPlatform(*plat))
	}
	return remote.Image(r, opts...)
}
