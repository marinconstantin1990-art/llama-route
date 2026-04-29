package gpu

import (
	"context"
	"errors"
	"testing"
)

func TestParseNvidiaSMI(t *testing.T) {
	in := "0, NVIDIA GeForce RTX 4090, 24576, 23000\n1, NVIDIA GeForce RTX 4090, 24576, 100\n"
	got := parseNvidiaSMI(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 GPUs, got %d", len(got))
	}
	if got[0].ID != "nvidia:0" || got[0].VRAMTotalMB != 24576 || got[0].VRAMFreeMB != 23000 {
		t.Errorf("gpu 0 wrong: %+v", got[0])
	}
	if got[1].Index != 1 || got[1].Vendor != VendorNVIDIA {
		t.Errorf("gpu 1 wrong: %+v", got[1])
	}
}

func TestParseRocmSMI(t *testing.T) {
	in := []byte(`{
		"card0": {"Card series": "Radeon RX 7900 XTX", "VRAM Total Memory (B)": "25753026560", "VRAM Total Used Memory (B)": "1048576"},
		"card1": {"Card series": "Radeon RX 7900 XTX", "VRAM Total Memory (B)": "25753026560", "VRAM Total Used Memory (B)": "0"}
	}`)
	got := parseRocmSMI(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 GPUs, got %d", len(got))
	}
	for _, g := range got {
		if g.Vendor != VendorAMD {
			t.Errorf("wrong vendor: %+v", g)
		}
		if g.VRAMTotalMB == 0 {
			t.Errorf("zero VRAM total: %+v", g)
		}
	}
}

func TestParseAppleProfiler(t *testing.T) {
	in := []byte(`{"SPDisplaysDataType":[{"_name":"Apple M2 Max","sppci_model":"Apple M2 Max","spdisplays_vram":"32 GB"}]}`)
	got := parseAppleProfiler(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 GPU, got %d", len(got))
	}
	if got[0].VRAMTotalMB != 32*1024 {
		t.Errorf("expected 32GB VRAM, got %d MB", got[0].VRAMTotalMB)
	}
}

func TestEnvFor(t *testing.T) {
	cases := []struct {
		gpu  GPU
		want string
	}{
		{GPU{Vendor: VendorNVIDIA, Index: 1}, "CUDA_VISIBLE_DEVICES=1"},
		{GPU{Vendor: VendorAMD, Index: 0}, "HIP_VISIBLE_DEVICES=0"},
		{GPU{Vendor: VendorApple, Index: 0}, "GGML_METAL_DEVICE=0"},
		{GPU{Vendor: VendorIntel, Index: 2}, "ONEAPI_DEVICE_SELECTOR=level_zero:2"},
	}
	for _, c := range cases {
		got := EnvFor(c.gpu)
		if len(got) == 0 || got[0] != c.want {
			t.Errorf("EnvFor(%v) first=%v, want %q", c.gpu, got, c.want)
		}
	}
}

func TestDetectMergesAndSorts(t *testing.T) {
	probes := []probeFn{
		func(context.Context) []GPU {
			return []GPU{{ID: "nvidia:1", Vendor: VendorNVIDIA, Index: 1}}
		},
		func(context.Context) []GPU {
			return []GPU{{ID: "nvidia:0", Vendor: VendorNVIDIA, Index: 0}}
		},
		func(context.Context) []GPU { return nil },
	}
	got := detectWith(context.Background(), probes)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Index != 0 || got[1].Index != 1 {
		t.Errorf("not sorted by index: %+v", got)
	}
	for _, g := range got {
		if !g.Enabled {
			t.Errorf("Detect should mark Enabled=true: %+v", g)
		}
	}
}

func TestProbeNoBinary(t *testing.T) {
	orig := runCmd
	defer func() { runCmd = orig }()
	runCmd = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("not found")
	}
	if got := probeNVIDIA(context.Background()); got != nil {
		t.Errorf("expected nil when binary missing, got %+v", got)
	}
}
