package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"
)

func executeCFL(ref CFLRef, input map[string]any) (map[string]any, error) {
	if ref.ID == "" || ref.Layer == "" {
		return nil, errors.New("invalid CFL ref")
	}
	body, err := json.Marshal(sanitizeJSON(input))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch {
	case ref.Command != "":
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", ref.Command)
	case ref.Module != "":
		dir, arg := goRunTarget(ref.Module)
		cmd = exec.CommandContext(ctx, "go", "run", arg)
		cmd.Dir = dir
	default:
		return nil, fmt.Errorf("cfl %s has no command or module", ref.ID)
	}
	if cmd.Dir == "" {
		cmd.Dir = moduleRoot()
	}
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("cfl %s timed out", ref.ID)
	}
	var raw map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &raw); err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("cfl %s exec failed: %v stderr=%s stdout=%s", ref.ID, runErr, strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()))
		}
		return nil, fmt.Errorf("cfl %s returned non-json stdout=%s", ref.ID, strings.TrimSpace(stdout.String()))
	}
	if raw["cfl_id"] == nil {
		raw["cfl_id"] = ref.ID
	}
	return raw, nil
}

func moduleRoot() string {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func goRunTarget(module string) (string, string) {
	root := moduleRoot()
	target := module
	if !filepath.IsAbs(target) {
		target = filepath.Clean(filepath.Join(root, target))
	}
	if modRoot := nearestGoModDir(target); modRoot != "" {
		if rel, err := filepath.Rel(modRoot, target); err == nil {
			return modRoot, "./" + filepath.ToSlash(rel)
		}
	}
	return root, module
}

func nearestGoModDir(path string) string {
	dir := path
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			return ""
		}
		dir = next
	}
}
