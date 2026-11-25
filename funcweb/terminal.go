package funcweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// handleTerminalRun executes a shell command in RootDir and streams stdout+stderr
// back as Server-Sent Events (SSE).
//
// Request:  POST /api/terminal/run   body: {"cmd": "ls -la"}
// Response: text/event-stream
//
//	data: {"type":"output","text":"...\n"}   — incremental output chunks
//	data: {"type":"exit","code":0}           — final event
//
// Security: execution is confined to cfg.RootDir; timeout is 30 s.
func (s *Server) handleTerminalRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.RootDir == "" {
		http.Error(w, "terminal not configured (no RootDir)", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Cmd string `json:"cmd"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil || json.Unmarshal(body, &req) != nil || strings.TrimSpace(req.Cmd) == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// SSE headers — disable buffering so chunks reach the browser immediately.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	sendEvent := func(typ, text string) {
		data := struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
			Code int    `json:"code,omitempty"`
		}{Type: typ, Text: text}
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", req.Cmd)
	cmd.Dir = s.cfg.RootDir

	// Merge stdout and stderr into one pipe for simplicity.
	cmd.Stdout = &sseWriter{send: func(p []byte) { sendEvent("output", string(p)) }}
	cmd.Stderr = cmd.Stdout

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok2 := err.(*exec.ExitError); ok2 {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			sendEvent("output", "\r\n[timeout after 30 s]\r\n")
			exitCode = -1
		}
	}

	// Send a synthetic exit event so the frontend knows the command finished.
	b, _ := json.Marshal(struct {
		Type string `json:"type"`
		Code int    `json:"code"`
	}{"exit", exitCode})
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}

// sseWriter calls send for every write, forwarding output chunks in real time.
type sseWriter struct {
	send func([]byte)
}

func (s *sseWriter) Write(p []byte) (int, error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	s.send(cp)
	return len(p), nil
}
