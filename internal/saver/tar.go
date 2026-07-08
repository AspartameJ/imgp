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

// OpenCachedLayer validates a cached layer file and returns a CompressedLayer.
// verifyGzip opens cacheFile, checks gzip magic bytes, and performs full CRC decompression check.
func verifyGzip(cacheFile string) error {
	f, err := os.Open(cacheFile)
	if err != nil {
		return fmt.Errorf("open cached layer: %w", err)
	}
	defer f.Close()

	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil || magic[0] != 0x1f || magic[1] != 0x8b {
		return fmt.Errorf("cached layer corrupted (bad gzip header): %s", cacheFile)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek cached layer: %w", err)
	}
	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("cached layer corrupted (invalid gzip): %s: %w", cacheFile, err)
	}
	defer gr.Close()
	if _, err := io.Copy(io.Discard, gr); err != nil {
		if rmErr := os.Remove(cacheFile); rmErr != nil {
			fmt.Fprintf(os.Stderr, "remove corrupted cache: %s: %v\n", cacheFile, rmErr)
		}
		return fmt.Errorf("cached layer corrupted (gzip CRC mismatch): %s: %w", cacheFile, err)
	}
	return nil
}

func OpenCachedLayer(v1Layer v1.Layer, cacheFile string) (partial.CompressedLayer, error) {
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
	if err := verifyGzip(cacheFile); err != nil {
		return nil, err
	}

	return &fileLayer{
		digest:    digest,
		mediaType: mediaType,
		size:      size,
		filepath:  cacheFile,
	}, nil
}

type cancelWriter struct {
	f   *os.File
	ctx context.Context
}

func (cw *cancelWriter) Write(p []byte) (int, error) {
	if err := cw.ctx.Err(); err != nil {
		return 0, err
	}
	return cw.f.Write(p)
}

// buildCachedImage reconstructs a v1.Image from cached layer files.
func buildCachedImage(img v1.Image, cachePathFn func(digest string) string) (v1.Image, error) {
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}

	configRaw, err := img.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}

	rawManifest, err := img.RawManifest()
	if err != nil {
		return nil, fmt.Errorf("get raw manifest: %w", err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	cachedLayers := make(map[v1.Hash]partial.CompressedLayer)
	for _, l := range layers {
		digest, err := l.Digest()
		if err != nil {
			return nil, fmt.Errorf("get layer digest: %w", err)
		}
		cl, err := OpenCachedLayer(l, cachePathFn(digest.Hex))
		if err != nil {
			return nil, err
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
		return nil, fmt.Errorf("rebuild image from cache: %w", err)
	}
	return v1Img, nil
}

// runProgress reports progress from a channel to a progress callback with panic recovery.
func runProgress(progressCh <-chan v1.Update, progressFn func(completed, total int64)) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for update := range progressCh {
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Fprintf(os.Stderr, "PANIC in progressFn: %v\n", r)
					}
				}()
				progressFn(update.Complete, update.Total)
			}()
		}
	}()
	return done
}

// cleanupTmp removes a temp file, ignoring not-exist errors.
func cleanupTmp(tmpPath string) {
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "remove temp file %s: %v\n", tmpPath, err)
	}
}

// writeTarBall writes the image as a tar (optionally gzipped) to a temp file and renames it.
func writeTarBall(ctx context.Context, ref name.Reference, v1Img v1.Image, outputPath string, gzipEnabled bool, progressFn func(completed, total int64)) (err error) {
	tmpPath := outputPath + ".tmp"
	succeeded := false
	defer func() {
		if !succeeded {
			cleanupTmp(tmpPath)
		}
	}()

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	f, createErr := os.Create(tmpPath)
	if createErr != nil {
		return fmt.Errorf("create output file: %w", createErr)
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	progressCh := make(chan v1.Update, 100)
	progressDone := runProgress(progressCh, progressFn)

	cw := &cancelWriter{f: f, ctx: ctx}
	var w io.Writer = cw
	var gw *gzip.Writer
	if gzipEnabled {
		gw = gzip.NewWriter(cw)
		w = gw
	}
	writeErr := func() error {
		defer close(progressCh)
		return tarball.Write(ref, v1Img, w, tarball.WithProgress(progressCh))
	}()

	<-progressDone

	if writeErr != nil {
		if gw != nil {
			gw.Close()
		}
		if ctx.Err() != nil {
			return fmt.Errorf("write tar: %w (context: %v)", writeErr, ctx.Err())
		}
		return fmt.Errorf("write tar: %w", writeErr)
	}

	if gw != nil {
		if gwErr := gw.Close(); gwErr != nil {
			return fmt.Errorf("close gzip: %w", gwErr)
		}
	}

	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("close tar: %w", cerr)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing output: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return fmt.Errorf("rename output: %w", err)
	}
	succeeded = true
	return nil
}

// Export writes a Docker-compatible tar archive from the given image.
func Export(
	ctx context.Context,
	ref name.Reference,
	img v1.Image,
	outputPath string,
	cachePathFn func(digest string) string,
	gzipEnabled bool,
	progressFn func(completed, total int64),
) error {
	if progressFn == nil {
		progressFn = func(int64, int64) {}
	}
	if fi, err := os.Stat(outputPath); err == nil && fi.IsDir() {
		return fmt.Errorf("output path is a directory: %s", outputPath)
	}

	v1Img, err := buildCachedImage(img, cachePathFn)
	if err != nil {
		return err
	}

	return writeTarBall(ctx, ref, v1Img, outputPath, gzipEnabled, progressFn)
}
