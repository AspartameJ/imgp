package registry

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
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
	return &Client{cfg: cfg, retry: 2}
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

func (c *Client) transport(reg name.Registry) http.RoundTripper {
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
	if c.insecure {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		return t
	}
	regName := normalizeRegistry(reg.Name())
	for _, ir := range c.cfg.InsecureRegistries {
		if regName == ir || strings.HasSuffix(regName, "."+ir) {
			if t.TLSClientConfig == nil {
				t.TLSClientConfig = &tls.Config{}
			}
			t.TLSClientConfig.InsecureSkipVerify = true
			break
		}
	}
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
	if a, ok := c.cfg.Auths[regName]; ok {
		password := a.Password
		if a.PasswordEnv != "" {
			if p, ok := os.LookupEnv(a.PasswordEnv); ok {
				password = p
			}
		}
		return authn.FromConfig(authn.AuthConfig{
			Username: a.Username,
			Password: password,
		})
	}
	if a, ok := c.cfg.Auths["*"]; ok {
		password := a.Password
		if a.PasswordEnv != "" {
			if p, ok := os.LookupEnv(a.PasswordEnv); ok {
				password = p
			}
		}
		return authn.FromConfig(authn.AuthConfig{
			Username: a.Username,
			Password: password,
		})
	}
	return authn.Anonymous
}

func parsePlatform(platform string) *v1.Platform {
	if platform == "" {
		return nil
	}
	p := &v1.Platform{}
	if strings.Contains(platform, "/") {
		parts := strings.Split(platform, "/")
		for _, part := range parts {
			if part == "" {
				return nil
			}
		}
		if len(parts) < 2 || len(parts) > 3 {
			return nil
		}
		p.OS = parts[0]
		p.Architecture = parts[1]
		if len(parts) > 2 {
			p.Variant = parts[2]
		}
	} else {
		p.OS = "linux"
		p.Architecture = platform
	}
	return p
}

// NewLayerFetcher returns a function that opens a layer download stream
// with per-call context support, allowing per-layer timeouts.
func (c *Client) NewLayerFetcher(ref name.Reference) func(ctx context.Context, digestHex string) (io.ReadCloser, error) {
	repo := ref.Context()
	return func(ctx context.Context, digestHex string) (io.ReadCloser, error) {
		hex := strings.TrimPrefix(digestHex, "sha256:")
		digestRef := repo.Digest("sha256:" + hex)
		reg := digestRef.Context().Registry
		l, err := remote.Layer(digestRef,
			remote.WithAuth(c.authenticator(reg)),
			remote.WithTransport(c.transport(reg)),
			remote.WithContext(ctx),
			remote.WithUserAgent(userAgent),
		)
		if err != nil {
			return nil, err
		}
		return l.Compressed()
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
					continue
				}
				refs = append(refs, mirrorTag)
			}
		case name.Digest:
			digestStr := r.DigestStr()
			for _, m := range mirrors {
				mirrorDigest, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", m, repoPath, digestStr))
				if err != nil {
					continue
				}
				refs = append(refs, mirrorDigest)
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
