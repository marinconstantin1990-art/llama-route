package gpu

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// runCmd is overridable from tests to inject canned probe output.
var runCmd = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(name); err != nil {
		return nil, errNoBinary
	}
	return exec.CommandContext(ctx, name, args...).Output()
}

var errNoBinary = errors.New("binary not on PATH")

func probeNVIDIA(ctx context.Context) []GPU {
	out, err := runCmd(ctx, "nvidia-smi",
		"--query-gpu=index,name,memory.total,memory.free",
		"--format=csv,noheader,nounits")
	if err != nil {
		return nil
	}
	return parseNvidiaSMI(string(out))
}

func parseNvidiaSMI(s string) []GPU {
	r := csv.NewReader(strings.NewReader(strings.TrimSpace(s)))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil
	}
	var out []GPU
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		total, _ := strconv.Atoi(strings.TrimSpace(row[2]))
		free, _ := strconv.Atoi(strings.TrimSpace(row[3]))
		out = append(out, GPU{
			ID:          MakeID(VendorNVIDIA, idx),
			Vendor:      VendorNVIDIA,
			Index:       idx,
			Name:        strings.TrimSpace(row[1]),
			VRAMTotalMB: total,
			VRAMFreeMB:  free,
		})
	}
	return out
}

func probeAMD(ctx context.Context) []GPU {
	out, err := runCmd(ctx, "rocm-smi",
		"--showid", "--showproductname", "--showmeminfo", "vram", "--json")
	if err != nil {
		return nil
	}
	return parseRocmSMI(out)
}

// parseRocmSMI handles the rocm-smi --json shape, which is a map keyed by
// "card0", "card1", ... with mixed-case fields that have varied across ROCm
// versions. We tolerate missing fields rather than failing the whole probe.
func parseRocmSMI(b []byte) []GPU {
	var raw map[string]map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}
	var out []GPU
	for k, fields := range raw {
		idx, ok := cardIndex(k)
		if !ok {
			continue
		}
		name := firstString(fields, "Card series", "Card model", "Card SKU", "Product Name")
		total := firstInt(fields, "VRAM Total Memory (B)", "vram total", "VRAM Total (B)") / (1024 * 1024)
		used := firstInt(fields, "VRAM Total Used Memory (B)", "vram used", "VRAM Used (B)") / (1024 * 1024)
		free := total - used
		if free < 0 {
			free = 0
		}
		out = append(out, GPU{
			ID:          MakeID(VendorAMD, idx),
			Vendor:      VendorAMD,
			Index:       idx,
			Name:        name,
			VRAMTotalMB: total,
			VRAMFreeMB:  free,
		})
	}
	return out
}

func cardIndex(key string) (int, bool) {
	const prefix = "card"
	if !strings.HasPrefix(key, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
	if err != nil {
		return 0, false
	}
	return n, true
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func firstInt(m map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case float64:
			return int(t)
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			if err == nil {
				return int(n)
			}
		}
	}
	return 0
}

func probeApple(ctx context.Context) []GPU {
	if runtime.GOOS != "darwin" {
		return nil
	}
	out, err := runCmd(ctx, "system_profiler", "-json", "SPDisplaysDataType")
	if err != nil {
		return nil
	}
	return parseAppleProfiler(out)
}

func parseAppleProfiler(b []byte) []GPU {
	var doc struct {
		SPDisplaysDataType []struct {
			Name        string `json:"_name"`
			ChipsetType string `json:"sppci_model"`
			Cores       string `json:"sppci_cores"`
			VRAM        string `json:"spdisplays_vram"`
		} `json:"SPDisplaysDataType"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil
	}
	var out []GPU
	for i, d := range doc.SPDisplaysDataType {
		name := d.ChipsetType
		if name == "" {
			name = d.Name
		}
		out = append(out, GPU{
			ID:          MakeID(VendorApple, i),
			Vendor:      VendorApple,
			Index:       i,
			Name:        name,
			VRAMTotalMB: parseAppleVRAM(d.VRAM),
		})
	}
	return out
}

// parseAppleVRAM accepts strings like "8 GB" or "16384 MB" and returns MB.
// Apple Silicon shares RAM, so VRAM total is reported as the unified-memory
// figure; free is unknown from system_profiler.
func parseAppleVRAM(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Fields(s)
	if len(parts) < 1 {
		return 0
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	if len(parts) >= 2 && strings.EqualFold(parts[1], "GB") {
		return n * 1024
	}
	return n
}

func probeIntel(ctx context.Context) []GPU {
	out, err := runCmd(ctx, "xpu-smi", "discovery", "-j")
	if err != nil {
		return nil
	}
	return parseXpuSMI(out)
}

func parseXpuSMI(b []byte) []GPU {
	var doc struct {
		DeviceList []struct {
			DeviceID   int    `json:"device_id"`
			DeviceName string `json:"device_name"`
		} `json:"device_list"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil
	}
	var out []GPU
	for _, d := range doc.DeviceList {
		out = append(out, GPU{
			ID:     MakeID(VendorIntel, d.DeviceID),
			Vendor: VendorIntel,
			Index:  d.DeviceID,
			Name:   d.DeviceName,
		})
	}
	return out
}
