package comfy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Input struct {
	Node  string `json:"node"`
	Field string `json:"field"`
}

type Config struct {
	BaseURL        string           `json:"base_url"`
	TokenEnv       string           `json:"token_env,omitempty"`
	Model          string           `json:"model"`
	Name           string           `json:"name"`
	Workflow       string           `json:"workflow"`
	Inputs         map[string]Input `json:"inputs"`
	OutputNode     string           `json:"output_node"`
	TimeoutSeconds int              `json:"timeout_seconds,omitempty"`
}

type Node struct {
	ClassType string         `json:"class_type"`
	Inputs    map[string]any `json:"inputs"`
	Meta      map[string]any `json:"_meta,omitempty"`
}

type Graph map[string]Node

func decode(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("expected one JSON value")
	}
	return nil
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if len(b) > 4<<20 {
		return nil, errors.New("configuration or workflow exceeds 4 MiB")
	}
	return b, err
}

func Load(path string) (Config, Graph, error) {
	var c Config
	b, err := readFile(path)
	if err == nil {
		err = decode(b, &c)
	}
	if err != nil {
		return c, nil, fmt.Errorf("configuration: %w", err)
	}
	c.BaseURL, err = endpoint(c.BaseURL)
	if err != nil {
		return c, nil, err
	}
	// Magpie currently recognizes plugin image providers by their model IDs.
	if !strings.HasPrefix(c.Model, "sdxl") && !strings.HasPrefix(c.Model, "stable-diffusion") && !strings.HasPrefix(c.Model, "flux") && !strings.HasPrefix(c.Model, "qwen-image") {
		return c, nil, errors.New("model must start with sdxl, stable-diffusion, flux, or qwen-image for Magpie image discovery")
	}
	if strings.ContainsAny(c.Model, "/\\ \t\r\n") {
		return c, nil, errors.New("model must not contain a path or whitespace")
	}
	if c.Name == "" {
		c.Name = c.Model + " (ComfyUI)"
	}
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 900
	}
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 86400 {
		return c, nil, errors.New("timeout_seconds must be between 1 and 86400")
	}
	if c.Workflow == "" {
		return c, nil, errors.New("workflow must name an API-format JSON file")
	}
	if !filepath.IsAbs(c.Workflow) {
		c.Workflow = filepath.Join(filepath.Dir(path), c.Workflow)
	}
	b, err = readFile(c.Workflow)
	var graph Graph
	if err == nil {
		err = decode(b, &graph)
	}
	if err != nil {
		return c, nil, fmt.Errorf("API workflow: %w", err)
	}
	if len(graph) == 0 {
		return c, nil, errors.New("API workflow is empty")
	}
	for id, n := range graph {
		if n.ClassType == "" || n.Inputs == nil {
			return c, nil, fmt.Errorf("node %s needs class_type and inputs; export in API format", id)
		}
	}
	if graph[c.OutputNode].ClassType != "SaveImage" {
		return c, nil, errors.New("output_node must identify a SaveImage node")
	}
	seen := map[Input]bool{}
	for name, target := range c.Inputs {
		switch name {
		case "prompt", "negative_prompt", "width", "height", "seed", "batch_size":
		default:
			return c, nil, fmt.Errorf("unknown input mapping %q", name)
		}
		value, ok := graph[target.Node].Inputs[target.Field]
		if !ok || seen[target] {
			return c, nil, fmt.Errorf("input %s must map to an existing, distinct node field", name)
		}
		seen[target] = true
		if name == "prompt" || name == "negative_prompt" {
			if _, ok := value.(string); !ok {
				return c, nil, fmt.Errorf("input %s must map to a text literal", name)
			}
		} else if _, ok := value.(json.Number); !ok {
			return c, nil, fmt.Errorf("input %s must map to a number literal", name)
		}
	}
	if _, ok := c.Inputs["prompt"]; !ok {
		return c, nil, errors.New("a prompt input mapping is required")
	}
	return c, graph, nil
}

func endpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("base_url must be an HTTP(S) URL without credentials, query or fragment")
	}
	return strings.TrimRight(raw, "/"), nil
}

// Connection also accepts a server without a workflow, for discovery alone.
func Connection(server, tokenEnv, name, configFile string) (Config, Graph, error) {
	c := Config{TimeoutSeconds: 900}
	var graph Graph
	var err error
	if configFile != "" {
		c, graph, err = Load(configFile)
		if err != nil {
			return c, nil, err
		}
	}
	if server != "" {
		c.BaseURL = server
	}
	if c.BaseURL == "" {
		c.BaseURL = "http://127.0.0.1:8188"
	}
	c.BaseURL, err = endpoint(c.BaseURL)
	if err != nil {
		return c, nil, err
	}
	if tokenEnv != "" {
		c.TokenEnv = tokenEnv
	}
	if name != "" {
		c.Name = name
	}
	if c.Name == "" {
		u, _ := url.Parse(c.BaseURL)
		c.Name = "ComfyUI " + u.Host
	}
	return c, graph, nil
}
