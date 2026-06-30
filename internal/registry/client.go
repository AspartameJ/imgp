package registry

import (
	"context"
	"crypto/tls"
	"errors"
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
)

type Client struct {
	cfg      *config.Config
	username string
	password string
	insecure bool
	retry    int
}

func NewClient(cfg *config.Config) *Client {
	return &Client{cfg: cfg, retry: 2}
}

func (c *Client) WithAuth(username, password string) *Client {
	c.username = username
	c.password = password
	return c
}

func (c *Client) WithInsecure(v bool) *Client {
	c.insecure = v
	return c
}

func (c *Client) WithRetry(n int) *Client {
	if n >= 0 {
		c.retry = n
	}
	return c
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

func isRetryableFetch(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "unexpected status code 4") {
		return false
	}
	retryable := []string{"unexpected EOF", "connection reset", "connection refused",
		"TLS handshake", "broken pipe"}
	for _, s := range retryable {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return strings.Contains(msg, "unexpected status code 5")
}

func (c *Client) FetchImage(ctx context.Context, image, platform string) (v1.Image, name.Reference, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, nil, fmt.Errorf("parse image reference: %w", err)
	}

	var refsToTry []name.Reference

	if tag, ok := ref.(name.Tag); ok {
		regStr := normalizeRegistry(tag.Context().RegistryStr())

		// Check MirrorMap first
		if mirrors, ok := c.cfg.MirrorMap[regStr]; ok {
			repoPath := tag.Context().RepositoryStr()
			tagStr := tag.TagStr()
			for _, m := range mirrors {
				mirrorTag, err := name.NewTag(fmt.Sprintf("%s/%s:%s", m, repoPath, tagStr))
				if err != nil {
					continue
				}
				refsToTry = append(refsToTry, mirrorTag)
			}
		}

	}

	refsToTry = append(refsToTry, ref)

	plat := parsePlatform(platform)

	origAuth := c.authenticator(ref.Context().Registry)

	var lastErr error
	for attempt := 0; attempt <= c.retry; attempt++ {
		if attempt > 0 {
			if !isRetryableFetch(lastErr) {
				break
			}
			shift := attempt - 1
			const maxShift = 30
			if shift > maxShift {
				shift = maxShift
			}
			backoff := time.Duration(1<<uint(shift)) * time.Second
			const maxBackoff = 30 * time.Second
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, nil, ctx.Err()
			case <-timer.C:
			}
		}

		var errs []string
		for _, r := range refsToTry {
			reg := r.Context().Registry
			auth := c.authenticator(reg)
			if c.username == "" && auth == authn.Anonymous && origAuth != authn.Anonymous {
				auth = origAuth
			}
			opts := []remote.Option{
				remote.WithAuth(auth),
				remote.WithTransport(c.transport(reg)),
				remote.WithContext(ctx),
			}
			if plat != nil {
				opts = append(opts, remote.WithPlatform(*plat))
			}

			img, err := remote.Image(r, opts...)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", r.String(), err))
				lastErr = err
				continue
			}
			return img, r, nil
		}

		if attempt == c.retry || !isRetryableFetch(lastErr) {
			return nil, nil, fmt.Errorf("all registries failed:\n  %s", strings.Join(errs, "\n  "))
		}
	}
	// Keep compiler happy: loop always returns, but Go can't prove c.retry >= 0
	return nil, nil, fmt.Errorf("all registries failed: %v", lastErr)
}
