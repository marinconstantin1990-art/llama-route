// Package gpu detects locally available GPUs across vendors (NVIDIA, AMD, Apple
// Metal, Intel) and exposes the env-vars needed to pin a child process to a
// specific device.
//
// Detection is best-effort: if a vendor's CLI tool is not installed or fails,
// that vendor's probe returns nil and the others still run.
package gpu

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Vendor identifies a GPU's vendor family.
type Vendor string

const (
	VendorNVIDIA Vendor = "nvidia"
	VendorAMD    Vendor = "amd"
	VendorApple  Vendor = "apple"
	VendorIntel  Vendor = "intel"
)

// GPU describes a single detected device.
type GPU struct {
	// ID is "<vendor>:<index>" and is stable across detections so configs that
	// reference a specific GPU keep working when probes run again.
	ID          string `json:"id"`
	Vendor      Vendor `json:"vendor"`
	Index       int    `json:"index"`
	Name        string `json:"name"`
	VRAMTotalMB int    `json:"vramTotalMB"`
	VRAMFreeMB  int    `json:"vramFreeMB"`
	// Enabled mirrors the user's preference from config; detection itself
	// always returns Enabled=true and the proxy layer overlays disabled IDs.
	Enabled bool `json:"enabled"`
}

// probeFn runs a single vendor probe within ctx and returns whatever GPUs it
// found. Probes must never panic; they return nil on any error.
type probeFn func(ctx context.Context) []GPU

// defaultProbes is the registry consulted by Detect. Tests replace it.
var defaultProbes = []probeFn{
	probeNVIDIA,
	probeAMD,
	probeApple,
	probeIntel,
}

// Detect runs all vendor probes in parallel with a 2s budget each and returns
// the merged list, sorted by (vendor, index) for stable output.
func Detect(ctx context.Context) []GPU {
	return detectWith(ctx, defaultProbes)
}

func detectWith(ctx context.Context, probes []probeFn) []GPU {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []GPU
	)
	for _, p := range probes {
		wg.Add(1)
		go func(probe probeFn) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			found := probe(pctx)
			if len(found) == 0 {
				return
			}
			mu.Lock()
			results = append(results, found...)
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		if results[i].Vendor != results[j].Vendor {
			return results[i].Vendor < results[j].Vendor
		}
		return results[i].Index < results[j].Index
	})
	for i := range results {
		results[i].Enabled = true
	}
	return results
}

// MakeID composes the canonical ID for a GPU.
func MakeID(v Vendor, index int) string {
	return fmt.Sprintf("%s:%d", v, index)
}

// EnvFor returns the environment variables needed to make a child llama-server
// see only the given GPU. Returns nil for unknown vendors.
func EnvFor(g GPU) []string {
	switch g.Vendor {
	case VendorNVIDIA:
		return []string{fmt.Sprintf("CUDA_VISIBLE_DEVICES=%d", g.Index)}
	case VendorAMD:
		// HIP_VISIBLE_DEVICES is preferred by recent ROCm; ROCR_VISIBLE_DEVICES
		// is still honored by older runtimes. Set both to be safe.
		return []string{
			fmt.Sprintf("HIP_VISIBLE_DEVICES=%d", g.Index),
			fmt.Sprintf("ROCR_VISIBLE_DEVICES=%d", g.Index),
		}
	case VendorApple:
		// Apple Silicon exposes a single GPU; index is informational only.
		return []string{fmt.Sprintf("GGML_METAL_DEVICE=%d", g.Index)}
	case VendorIntel:
		return []string{fmt.Sprintf("ONEAPI_DEVICE_SELECTOR=level_zero:%d", g.Index)}
	default:
		return nil
	}
}
