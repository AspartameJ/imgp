package saver

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

func (l *fileLayer) Digest() (v1.Hash, error)            { return l.digest, nil }
func (l *fileLayer) MediaType() (types.MediaType, error) { return l.mediaType, nil }
func (l *fileLayer) Size() (int64, error)                { return l.size, nil }
func (l *fileLayer) Compressed() (io.ReadCloser, error) {
	return os.Open(l.filepath)
}

type compressedImage struct {
	configFile  []byte
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

	fi, err := os.Stat(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cached layer not found: %s", cacheFile)
		}
		return nil, fmt.Errorf("access cached layer: %s: %w", cacheFile, err)
	}
	if fi.Size() != size {
		return nil, fmt.Errorf("cached layer incomplete: %s: expected %d bytes, got %d", cacheFile, size, fi.Size())
	}
	f, err := os.Open(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("open cached layer: %w", err)
	}
	var magic [2]byte
	if _, err := f.Read(magic[:]); err != nil || magic[0] != 0x1f || magic[1] != 0x8b {
		f.Close()
		return nil, fmt.Errorf("cached layer corrupted (bad gzip header): %s", cacheFile)
	}
	f.Close()

	// Full gzip integrity check: decompress and verify CRC
	gf, err := os.Open(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("open cached layer: %w", err)
	}
	gr, err := gzip.NewReader(gf)
	if err != nil {
		gf.Close()
		return nil, fmt.Errorf("cached layer corrupted (invalid gzip): %s: %w", cacheFile, err)
	}
	if _, err := io.Copy(io.Discard, gr); err != nil {
		gr.Close()
		gf.Close()
		os.Remove(cacheFile)
		return nil, fmt.Errorf("cached layer corrupted (gzip CRC mismatch): %s: %w", cacheFile, err)
	}
	gr.Close()
	gf.Close()

	return &fileLayer{
		digest:    digest,
		mediaType: mediaType,
		size:      size,
		filepath:  cacheFile,
	}, nil
}

func Export(
	ctx context.Context,
	ref name.Reference,
	img v1.Image,
	outputPath string,
	cachePathFn func(digest string) string,
	progressFn func(completed, total int64),
) error {
	if fi, err := os.Stat(outputPath); err == nil && fi.IsDir() {
		return fmt.Errorf("output path is a directory: %s", outputPath)
	}
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
		digest, err := l.Digest()
		if err != nil {
			return fmt.Errorf("get layer digest: %w", err)
		}
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

	tmpPath := outputPath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}

	progressCh := make(chan v1.Update, 100)
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		for update := range progressCh {
			func() {
				defer func() { recover() }()
				progressFn(update.Complete, update.Total)
			}()
		}
	}()

	writeErr := tarball.Write(ref, v1Img, f, tarball.WithProgress(progressCh))
	close(progressCh)
	<-progressDone
	closeErr := f.Close()

	if writeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write tar: %w", writeErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close tar: %w", closeErr)
	}

	if ctx.Err() != nil {
		os.Remove(tmpPath)
		return ctx.Err()
	}

	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		os.Remove(tmpPath)
		return fmt.Errorf("remove existing output: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename output: %w", err)
	}

	return nil
}
