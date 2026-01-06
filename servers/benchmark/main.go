// main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

type BenchRequest struct {
	// REQUIRED
	Model string `json:"model"`

	// Optional overrides (defaults match your example)
	NGL       *int `json:"ngl,omitempty"`     // -ngl (default 99)
	FlashAttn *int `json:"fa,omitempty"`      // -fa  (default 1)
	NoHost    *int `json:"no_host,omitempty"` // --no-host (default 1)
	Threads   *int `json:"threads,omitempty"` // -t (default 8)
	UBatch    *int `json:"ubatch,omitempty"`  // -ub (default 512)
}

type ErrResp struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error"`
	ExitCode  int    `json:"exit_code,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
	StdErr    string `json:"stderr,omitempty"`
}

type Server struct {
	mu  sync.Mutex
	cfg *Config
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	s := &Server{cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /benchmark", s.handleBench)

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("benchmark config: host=%s port=%d type=%s api_base=%s models_path=%s", cfg.Host, cfg.Port, cfg.Type, cfg.APIBase, cfg.ModelsPath)
	log.Printf("benchmark timeout: %ds", cfg.TimeoutSeconds)
	log.Printf("listening on %s (GOOS=%s)", srv.Addr, runtime.GOOS)
	log.Fatal(srv.ListenAndServe())
}

func (s *Server) handleBench(w http.ResponseWriter, r *http.Request) {
	// One benchmark at a time (simple + protects the machine)
	log.Printf("benchmark request from %s", r.RemoteAddr)
	s.mu.Lock()
	defer s.mu.Unlock()

	var req BenchRequest
	if err := decodeJSON(w, r, &req, 1<<20 /* 1 MiB */); err != nil {
		log.Printf("benchmark decode error: %v", err)
		writeJSON(w, http.StatusBadRequest, ErrResp{OK: false, Error: "invalid JSON: " + err.Error()})
		return
	}

	if req.Model == "" {
		log.Printf("benchmark validation error: model is required")
		writeJSON(w, http.StatusBadRequest, ErrResp{OK: false, Error: "model is required"})
		return
	}

	modelPath := req.Model
	switch strings.ToLower(strings.TrimSpace(s.cfg.Type)) {
	case "llama.cpp":
		// Ensure model exists on server (strict, avoids arbitrary file reads)
		if !filepath.IsAbs(modelPath) {
			if strings.TrimSpace(s.cfg.ModelsPath) == "" {
				log.Printf("benchmark models_path not configured for llama.cpp")
				writeJSON(w, http.StatusInternalServerError, ErrResp{OK: false, Error: "models_path is required for llama.cpp"})
				return
			}
			modelPath = filepath.Join(s.cfg.ModelsPath, modelPath)
		}
		if _, err := os.Stat(modelPath); err != nil {
			log.Printf("benchmark model not found (llama.cpp): %s", modelPath)
			writeJSON(w, http.StatusBadRequest, ErrResp{OK: false, Error: "model file not found: " + modelPath})
			return
		}
	default:
		log.Printf("benchmark unsupported backend type: %s", s.cfg.Type)
		writeJSON(w, http.StatusInternalServerError, ErrResp{OK: false, Error: "unsupported backend type: " + s.cfg.Type})
		return
	}

	bin, err := resolveBenchBinary()
	if err != nil {
		log.Printf("benchmark resolve binary error: %v", err)
		writeJSON(w, http.StatusInternalServerError, ErrResp{OK: false, Error: err.Error()})
		return
	}

	req.Model = modelPath
	args, err := buildArgs(req)
	if err != nil {
		log.Printf("benchmark build args error: %v", err)
		writeJSON(w, http.StatusBadRequest, ErrResp{OK: false, Error: err.Error()})
		return
	}

	// Hard timeout for the benchmark run
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	start := time.Now()
	log.Printf("benchmark start: bin=%s args=%v", bin, args)
	stdout, stderr, code, runErr := runCommand(ctx, bin, args, 25<<20 /*25 MiB*/, 5<<20 /*5 MiB*/)
	elapsed := time.Since(start).Milliseconds()

	if runErr != nil {
		log.Printf("benchmark run error: %v (exit=%d elapsed_ms=%d)", runErr, code, elapsed)
		writeJSON(w, http.StatusInternalServerError, ErrResp{
			OK:        false,
			Error:     runErr.Error(),
			ExitCode:  code,
			ElapsedMS: elapsed,
			StdErr:    stderr,
		})
		return
	}

	if !json.Valid([]byte(stdout)) {
		log.Printf("benchmark output invalid JSON (exit=%d elapsed_ms=%d)", code, elapsed)
		writeJSON(w, http.StatusInternalServerError, ErrResp{
			OK:        false,
			Error:     "benchmark did not emit valid JSON on stdout",
			ExitCode:  code,
			ElapsedMS: elapsed,
			StdErr:    stderr,
		})
		return
	}

	// Pass-through the JSON array exactly as llama-bench produced it.
	log.Printf("benchmark complete (exit=%d elapsed_ms=%d)", code, elapsed)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(stdout))
}

func resolveBenchBinary() (string, error) {
	// Fixed layout relative to where the server is run from.
	// If you want to make it relative to the executable instead, say so and I’ll switch to os.Executable().
	var rel string
	switch runtime.GOOS {
	case "windows":
		rel = "./llama.cpp-windows/llama-bench.exe"
	case "linux":
		rel = "./llama.cpp-linux/llama-bench"
	case "darwin":
		rel = "./llama.cpp-darwin/llama-bench"
	default:
		return "", errors.New("unsupported OS: " + runtime.GOOS)
	}

	abs := rel
	if !filepath.IsAbs(rel) {
		if wd, err := os.Getwd(); err == nil {
			abs = filepath.Join(wd, rel)
		}
	}

	if _, err := os.Stat(abs); err != nil {
		return "", errors.New("llama-bench binary not found at: " + abs)
	}
	return abs, nil
}

type Config struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	Type           string `yaml:"type"`
	APIBase        string `yaml:"api_base"`
	ModelsPath     string `yaml:"models_path"`
	TimeoutSeconds int    `yaml:"timeout"`
}

var (
	configOnce sync.Once
	configVal  *Config
	configErr  error
)

func loadConfig() (*Config, error) {
	configOnce.Do(func() {
		path := filepath.Join("servers", "benchmark", "benchmark.yml")
		data, err := os.ReadFile(path)
		if err != nil {
			configErr = err
			return
		}

		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			configErr = err
			return
		}

		switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
		case "llama.cpp":
		default:
			configErr = fmt.Errorf("invalid type %q (expected \"llama.cpp\")", cfg.Type)
			return
		}

		if cfg.TimeoutSeconds <= 0 {
			cfg.TimeoutSeconds = 3600
		}

		configVal = &cfg
	})

	return configVal, configErr
}

func buildArgs(req BenchRequest) ([]string, error) {
	// Defaults match your example invocation
	ngl := 99
	fa := 1
	noHost := 1
	threads := 8
	ub := 512

	if req.NGL != nil {
		ngl = *req.NGL
	}
	if req.FlashAttn != nil {
		fa = *req.FlashAttn
	}
	if req.NoHost != nil {
		noHost = *req.NoHost
	}
	if req.Threads != nil {
		threads = *req.Threads
	}
	if req.UBatch != nil {
		ub = *req.UBatch
	}

	// Sanity bounds
	if ngl < 0 || ngl > 999 {
		return nil, errors.New("ngl out of range (0..999)")
	}
	if fa != 0 && fa != 1 {
		return nil, errors.New("fa must be 0 or 1")
	}
	if noHost != 0 && noHost != 1 {
		return nil, errors.New("no_host must be 0 or 1")
	}
	if threads < 1 || threads > 1024 {
		return nil, errors.New("threads out of range (1..1024)")
	}
	if ub < 1 || ub > 131072 {
		return nil, errors.New("ubatch out of range (1..131072)")
	}

	// Your exact invocation:
	// llama-bench -m <model> -ngl 99 -fa 1 --no-host 1 -t 8 -ub 512 -o json
	args := []string{
		"-m", req.Model,
		"-ngl", itoa(ngl),
		"-fa", itoa(fa),
		"--no-host", itoa(noHost),
		"-t", itoa(threads),
		"-ub", itoa(ub),
		"-o", "json",
	}

	return args, nil
}

func runCommand(ctx context.Context, bin string, args []string, maxStdout, maxStderr int64) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, bin, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", 127, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", "", 127, err
	}

	if err := cmd.Start(); err != nil {
		return "", "", 127, err
	}

	var outBuf, errBuf bytes.Buffer
	outDone := make(chan error, 1)
	errDone := make(chan error, 1)

	go func() {
		_, e := io.Copy(&outBuf, io.LimitReader(stdoutPipe, maxStdout))
		outDone <- e
	}()
	go func() {
		_, e := io.Copy(&errBuf, io.LimitReader(stderrPipe, maxStderr))
		errDone <- e
	}()

	waitErr := cmd.Wait()
	_ = <-outDone
	_ = <-errDone

	stdout = outBuf.String()
	stderr = errBuf.String()

	exitCode = 0
	if waitErr != nil {
		exitCode = exitStatus(waitErr)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return stdout, stderr, exitCode, errors.New("benchmark timed out")
		}
		return stdout, stderr, exitCode, waitErr
	}

	return stdout, stderr, 0, nil
}

func exitStatus(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 1
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [32]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + (i % 10))
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
