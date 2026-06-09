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
)

type Client struct {
	cfg      *config.Config
	username string
	password string
	insecure bool
}

func NewClient(cfg *config.Config) *Client {
	return &Client{cfg: cfg}
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

func (c *Client) transport(reg name.Registry) http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
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
	for _, ir := range c.cfg.InsecureRegistries {
		if reg.Name() == ir || strings.HasSuffix(reg.Name(), ir) {
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
	if a, ok := c.cfg.Auths[reg.Name()]; ok {
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
		parts := strings.SplitN(platform, "/", 3)
		p.OS = parts[0]
		if len(parts) > 1 {
			p.Architecture = parts[1]
		}
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
		digestRef := repo.Digest("sha256:" + digestHex)
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

	var errs []string
	for _, r := range refsToTry {
		reg := r.Context().Registry
		opts := []remote.Option{
			remote.WithAuth(c.authenticator(reg)),
			remote.WithTransport(c.transport(reg)),
			remote.WithContext(ctx),
		}
		if plat != nil {
			opts = append(opts, remote.WithPlatform(*plat))
		}

		img, err := remote.Image(r, opts...)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.String(), err))
			continue
		}
		return img, r, nil
	}

	return nil, nil, fmt.Errorf("all registries failed:\n  %s", strings.Join(errs, "\n  "))
}
