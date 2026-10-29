package comfy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxImage = 16 << 20

type Request struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body"`
}

type Reply struct {
	Status int `json:"status"`
	Body   any `json:"body"`
}

type ImageRequest struct {
	Model          string  `json:"model"`
	Prompt         string  `json:"prompt"`
	NegativePrompt *string `json:"negative_prompt,omitempty"`
	Size           string  `json:"size,omitempty"`
	N              *int    `json:"n,omitempty"`
	Seed           *uint64 `json:"seed,omitempty"`
	ResponseFormat string  `json:"response_format,omitempty"`
	OutputFormat   string  `json:"output_format,omitempty"`
	Quality        string  `json:"quality,omitempty"`
	Background     string  `json:"background,omitempty"`
	User           string  `json:"user,omitempty"`
}

func failure(status int, err error) Reply {
	kind := "comfyui_error"
	if status < 500 {
		kind = "invalid_request_error"
	}
	return Reply{status, map[string]any{"error": map[string]string{"message": err.Error(), "type": kind}}}
}

func prepare(c Config, graph Graph, body string) (int, error) {
	var r ImageRequest
	if err := decode([]byte(body), &r); err != nil {
		return 0, err
	}
	if r.Model != c.Model || strings.TrimSpace(r.Prompt) == "" || len(r.Prompt) > 64<<10 {
		return 0, errors.New("model must match the configuration and prompt must contain 1-65536 bytes of text")
	}
	if (r.ResponseFormat != "" && r.ResponseFormat != "b64_json") ||
		(r.OutputFormat != "" && r.OutputFormat != "png") ||
		(r.Quality != "" && r.Quality != "auto") ||
		(r.Background != "" && r.Background != "auto" && r.Background != "opaque") {
		return 0, errors.New("this workflow returns PNG b64_json; quality/background must use workflow defaults")
	}
	n := 1
	if r.N != nil {
		n = *r.N
	}
	if n < 1 || n > 4 {
		return 0, errors.New("n must be between 1 and 4")
	}
	set := func(name string, value any, required bool) error {
		t, ok := c.Inputs[name]
		if !ok {
			if required {
				return fmt.Errorf("workflow has no %s input mapping", name)
			}
			return nil
		}
		graph[t.Node].Inputs[t.Field] = value
		return nil
	}
	if err := set("prompt", r.Prompt, true); err != nil {
		return 0, err
	}
	if r.NegativePrompt != nil {
		if err := set("negative_prompt", *r.NegativePrompt, true); err != nil {
			return 0, err
		}
	}
	if err := set("batch_size", n, n > 1); err != nil {
		return 0, err
	}
	if r.Size != "" && r.Size != "auto" {
		parts := strings.Split(r.Size, "x")
		if len(parts) != 2 {
			return 0, errors.New("size must be WIDTHxHEIGHT or auto")
		}
		for i, name := range []string{"width", "height"} {
			v, err := strconv.Atoi(parts[i])
			if err != nil || v < 1 || v > 16384 {
				return 0, errors.New("size dimensions must be between 1 and 16384")
			}
			if err := set(name, v, true); err != nil {
				return 0, err
			}
		}
	}
	if r.Seed != nil {
		if err := set("seed", *r.Seed, true); err != nil {
			return 0, err
		}
	} else if _, ok := c.Inputs["seed"]; ok {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		// Stay exactly representable by JavaScript-based ComfyUI clients.
		set("seed", binary.LittleEndian.Uint64(b[:])&((1<<53)-1), false)
	}
	return n, nil
}

type client struct {
	config Config
	http   *http.Client
	token  string
}

func newClient(c Config) (client, error) {
	token := ""
	if c.TokenEnv != "" {
		token = os.Getenv(c.TokenEnv)
		if token == "" {
			return client{}, errors.New("configured token_env is not set in the Magpie process")
		}
	}
	return client{c, &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("ComfyUI redirects are refused; configure the final base_url")
	}}, token}, nil
}

func (c client) call(ctx context.Context, method, path string, body any, limit int64) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.config.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s response exceeds %d bytes", path, limit)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d: %s", path, res.StatusCode, string(b[:min(len(b), 2048)]))
	}
	return b, nil
}

type history struct {
	Status struct {
		Status    string            `json:"status_str"`
		Completed bool              `json:"completed"`
		Messages  []json.RawMessage `json:"messages"`
	} `json:"status"`
	Outputs map[string]struct {
		Images []struct {
			Filename  string `json:"filename"`
			Subfolder string `json:"subfolder"`
			Type      string `json:"type"`
		} `json:"images"`
	} `json:"outputs"`
}

func (c client) wait(ctx context.Context, id string, poll time.Duration) (history, error) {
	var empty history
	for {
		b, err := c.call(ctx, "GET", "/history/"+url.PathEscape(id), nil, 8<<20)
		if err != nil {
			return empty, err
		}
		var results map[string]history
		if err := json.Unmarshal(b, &results); err != nil {
			return empty, fmt.Errorf("history: %w", err)
		}
		if h, ok := results[id]; ok {
			if h.Status.Status == "error" {
				return empty, fmt.Errorf("workflow failed: %s", h.Status.Messages)
			}
			if h.Status.Completed {
				return h, nil
			}
		}
		select {
		case <-ctx.Done():
			return empty, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func Handle(ctx context.Context, c Config, graph Graph, req Request, log io.Writer) Reply {
	return handle(ctx, c, graph, req, log, time.Second)
}

func handle(ctx context.Context, c Config, graph Graph, req Request, log io.Writer, poll time.Duration) Reply {
	if req.Path != "/v1/images/generations" || req.Method != "POST" {
		return failure(400, errors.New("only POST /v1/images/generations is supported; image edits and chat are not implemented"))
	}
	if len(graph) == 0 {
		// Magpie can try an account whose empty model list was lost in its cache.
		return failure(404, errors.New("model not available on this connection; connect a workflow configuration to generate images"))
	}
	n, err := prepare(c, graph, req.Body)
	if err != nil {
		return failure(400, err)
	}
	api, err := newClient(c)
	if err != nil {
		return failure(400, err)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	b, err := api.call(ctx, "POST", "/prompt", map[string]any{"prompt": graph}, 4<<20)
	if err != nil {
		return failure(502, fmt.Errorf("submission failed (not retried; check ComfyUI queue before retrying): %w", err))
	}
	var receipt struct {
		ID    string          `json:"prompt_id"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(b, &receipt); err != nil || receipt.ID == "" || len(receipt.Error) > 0 && string(receipt.Error) != "null" {
		return failure(502, errors.New("ComfyUI returned no valid prompt_id; inspect its queue before retrying"))
	}
	fmt.Fprintf(log, "ComfyUI prompt_id=%s\n", receipt.ID)
	h, err := api.wait(ctx, receipt.ID, poll)
	if err != nil {
		// Never interrupt a shared GPU or resubmit a job after losing its result.
		return failure(502, fmt.Errorf("prompt_id=%s: %w; inspect /history/%s before retrying; the job may still run", receipt.ID, err, receipt.ID))
	}
	pictures := h.Outputs[c.OutputNode].Images
	if len(pictures) != n {
		return failure(502, fmt.Errorf("prompt_id=%s: output node returned %d images, wanted %d", receipt.ID, len(pictures), n))
	}
	data := make([]map[string]string, 0, n)
	for _, pic := range pictures {
		if pic.Filename == "" || pic.Type != "output" {
			return failure(502, fmt.Errorf("prompt_id=%s: output is not a saved image", receipt.ID))
		}
		q := url.Values{"filename": {pic.Filename}, "subfolder": {pic.Subfolder}, "type": {pic.Type}}
		b, err := api.call(ctx, "GET", "/view?"+q.Encode(), nil, maxImage)
		if err != nil {
			return failure(502, fmt.Errorf("prompt_id=%s: %w", receipt.ID, err))
		}
		_, format, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil || format != "png" {
			return failure(502, fmt.Errorf("prompt_id=%s: output is not a valid PNG", receipt.ID))
		}
		data = append(data, map[string]string{"b64_json": base64.StdEncoding.EncodeToString(b)})
	}
	return Reply{200, map[string]any{"created": time.Now().Unix(), "data": data, "prompt_id": receipt.ID}}
}
