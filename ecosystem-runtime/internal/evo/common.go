package evo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

func Run() {
	started := time.Now().UTC()
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		write(Response{Verdict: "fail", Error: "read stdin: " + err.Error(), LatencyMs: latencyMs(started)})
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		write(Response{Verdict: "fail", Error: "decode input: " + err.Error(), LatencyMs: latencyMs(started)})
	}
	resp := Handle(req, started)
	write(resp)
}

func write(resp Response) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(sanitizeJSON(resp)); err != nil {
		os.Exit(1)
	}
	os.Exit(exitCode(resp.Verdict))
}

func exitCode(verdict string) int {
	switch verdict {
	case "pass", "warn":
		return 0
	case "n/a", "needs-human":
		return 2
	default:
		return 1
	}
}

func latencyMs(start time.Time) int64 { return time.Since(start).Milliseconds() }
func nowMs(req Request) int64 {
	if req.NowUnixMs > 0 {
		return req.NowUnixMs
	}
	return time.Now().UTC().UnixMilli()
}

func defaultStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func statePath(stateDir, ecosystemID string) string {
	if stateDir == "" {
		stateDir = ".evolution-state"
	}
	return filepath.Join(stateDir, ecosystemID+".json")
}

func loadState(stateDir, ecosystemID string) (*EcosystemState, error) {
	if ecosystemID == "" {
		ecosystemID = "default"
	}
	b, err := os.ReadFile(statePath(stateDir, ecosystemID))
	if err != nil {
		return nil, err
	}
	var st EcosystemState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	st.initMaps()
	return &st, nil
}

func saveState(stateDir string, st *EcosystemState) error {
	path := statePath(stateDir, st.EcosystemID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sanitizeJSON(st), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func shortID(prefix string, v any) string {
	h := sha256.Sum256(canonicalBytes(v))
	return prefix + ":" + hex.EncodeToString(h[:])[:16]
}

func sha256Ref(v any) string {
	h := sha256.Sum256(canonicalBytes(v))
	return "sha256:" + hex.EncodeToString(h[:])
}

func patternRef(kind string, v any) string {
	return "p-cfl:" + sha256Ref(map[string]any{"kind": kind, "payload": v})
}
func valueRef(v any) string    { return "v-cfl:" + sha256Ref(v) }
func relationRef(v any) string { return "r-cfl:" + sha256Ref(v) }
func stateRef(v any) string    { return "s-cfl:" + sha256Ref(v) }

func canonicalBytes(v any) []byte {
	b, _ := json.Marshal(sanitizeJSON(v))
	return b
}

func sanitizeJSON(v any) any { return sanitizeValue(reflect.ValueOf(v)) }

func sanitizeValue(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return sanitizeValue(v.Elem())
	}
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return f
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint()
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = sanitizeValue(v.Index(i))
		}
		return out
	case reflect.Map:
		out := map[string]any{}
		for _, k := range v.MapKeys() {
			out[fmt.Sprint(k.Interface())] = sanitizeValue(v.MapIndex(k))
		}
		return out
	case reflect.Struct:
		out := map[string]any{}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			name := f.Name
			if tag := f.Tag.Get("json"); tag != "" {
				name = strings.Split(tag, ",")[0]
				if name == "-" || name == "" {
					continue
				}
			}
			out[name] = sanitizeValue(v.Field(i))
		}
		return out
	default:
		return v.Interface()
	}
}

func round6(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	return math.Round(x*1_000_000) / 1_000_000
}

func clampNonNegative(x float64) float64 {
	if x < 0 {
		return 0
	}
	return x
}

func sortedSeedIDs(seeds map[string]Seed) []string {
	ids := make([]string, 0, len(seeds))
	for id := range seeds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedTaskIDs(tasks map[string]Task) []string {
	ids := make([]string, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func stringSet(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func mergeResources(dst map[string]float64, delta map[string]float64) {
	for k, v := range delta {
		dst[k] = round6(dst[k] + v)
	}
}

func subtractResources(dst map[string]float64, delta map[string]float64) {
	for k, v := range delta {
		dst[k] = round6(dst[k] - v)
	}
}
