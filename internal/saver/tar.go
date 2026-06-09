package saver

import (
	"fmt"
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

type fileLayer struct {
	digest    v1.Hash
	mediaType types.MediaType
	size      int64
	filepath  string
}

func (l *fileLayer) Digest() (v1.Hash, error)      { return l.digest, nil }
func (l *fileLayer) MediaType() (types.MediaType, error) { return l.mediaType, nil }
func (l *fileLayer) Size() (int64, error)           { return l.size, nil }
func (l *fileLayer) Compressed() (io.ReadCloser, error) {
	return os.Open(l.filepath)
}

type compressedImage struct {
	configFile []byte
	rawManifest []byte
	mediaType   types.MediaType
	layers      map[v1.Hash]partial.CompressedLayer
}

func (c *compressedImage) RawConfigFile() ([]byte, error) {
	return c.configFile, nil
}

func (c *compressedImage) RawManifest() ([]byte, error) {
	return c.rawManifest, nil
}

func (c *compressedImage) MediaType() (types.MediaType, error) {
	return c.mediaType, nil
}

func (c *compressedImage) LayerByDigest(h v1.Hash) (partial.CompressedLayer, error) {
	l, ok := c.layers[h]
	if !ok {
		return nil, fmt.Errorf("layer %s not found in cache", h)
	}
	return l, nil
}

func BuildLayer(v1Layer v1.Layer, cacheFile string) (partial.CompressedLayer, error) {
	digest, err := v1Layer.Digest()
	if err != nil {
		return nil, err
	}
	mediaType, err := v1Layer.MediaType()
	if err != nil {
		return nil, err
	}
	size, err := v1Layer.Size()
	if err != nil {
		return nil, err
	}

	if fi, err := os.Stat(cacheFile); err != nil || fi.Size() != size {
		return nil, fmt.Errorf("cached layer not found or incomplete: %s", cacheFile)
	}

	return &fileLayer{
		digest:    digest,
		mediaType: mediaType,
		size:      size,
		filepath:  cacheFile,
	}, nil
}

func Export(
	ref name.Reference,
	img v1.Image,
	outputPath string,
	cachePathFn func(digest string) string,
	progressFn func(completed, total int64),
) error {
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("get layers: %w", err)
	}

	configRaw, err := img.RawConfigFile()
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}

	rawManifest, err := img.RawManifest()
	if err != nil {
		return fmt.Errorf("get raw manifest: %w", err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		return fmt.Errorf("get manifest: %w", err)
	}

	cachedLayers := make(map[v1.Hash]partial.CompressedLayer)

	for _, l := range layers {
		digest, _ := l.Digest()
		cacheFile := cachePathFn(digest.Hex)

		cl, err := BuildLayer(l, cacheFile)
		if err != nil {
			return err
		}
		cachedLayers[digest] = cl
	}

	ci := &compressedImage{
		configFile:  configRaw,
		rawManifest: rawManifest,
		mediaType:   manifest.MediaType,
		layers:      cachedLayers,
	}

	v1Img, err := partial.CompressedToImage(ci)
	if err != nil {
		return fmt.Errorf("rebuild image from cache: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	progressCh := make(chan v1.Update, 100)
	go func() {
		for update := range progressCh {
			progressFn(update.Complete, update.Total)
		}
	}()
	defer close(progressCh)

	return tarball.Write(ref, v1Img, f, tarball.WithProgress(progressCh))
}
