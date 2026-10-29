package comfy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sample(t *testing.T) (Config, Graph) {
	t.Helper()
	c, g, err := Load("../../examples/config.json")
	if err != nil {
		t.Fatal(err)
	}
	return c, g
}

func TestParameters(t *testing.T) {
	c, g := sample(t)
	n, err := prepare(c, g, `{"model":"sdxl","prompt":"line 1\n\"quoted\"","negative_prompt":"","n":2,"size":"768x512","seed":18446744073709551615}`)
	if err != nil || n != 2 {
		t.Fatalf("%d: %v", n, err)
	}
	if g["2"].Inputs["text"] != "line 1\n\"quoted\"" || g["3"].Inputs["text"] != "" ||
		g["4"].Inputs["width"] != 768 || g["4"].Inputs["height"] != 512 || g["4"].Inputs["batch_size"] != 2 ||
		g["5"].Inputs["seed"] != ^uint64(0) {
		t.Fatalf("wrong mapping: %#v", g)
	}
	if g["5"].Inputs["steps"] != json.Number("20") || g["1"].Inputs["ckpt_name"] != "sd_xl_base_1.0.safetensors" {
		t.Fatal("changed workflow defaults")
	}
}

func TestRejectUnsupportedInputs(t *testing.T) {
	for _, fields := range []string{
		`"n":0`, `"n":5`, `"size":"512xno"`, `"size":"0x1"`,
		`"quality":"high"`, `"background":"transparent"`, `"output_format":"webp"`,
		`"response_format":"url"`, `"images":["test"]`, `"seed":-1`, `"unknown":true`,
	} {
		t.Run(fields, func(t *testing.T) {
			c, g := sample(t)
			if _, err := prepare(c, g, `{"model":"sdxl","prompt":"hello",`+fields+`}`); err == nil {
				t.Fatal("accepted unsupported input")
			}
		})
	}
	c, g := sample(t)
	delete(c.Inputs, "width")
	if _, err := prepare(c, g, `{"model":"sdxl","prompt":"hi","size":"512x512"}`); err == nil {
		t.Fatal("silently ignored size")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.BaseURL = "https://user@host/" },
		func(c *Config) { c.BaseURL = "file:///tmp/comfy" },
		func(c *Config) { c.Model = "generic-image" },
		func(c *Config) { c.OutputNode = "4" },
		func(c *Config) { c.Inputs["prompt"] = Input{"1", "absent"} },
		func(c *Config) { c.Inputs["seed"] = c.Inputs["prompt"] },
		func(c *Config) { c.Inputs["seed"] = Input{"5", "model"} },
		func(c *Config) { delete(c.Inputs, "prompt") },
		func(c *Config) { c.TimeoutSeconds = -1 },
	} {
		c, _ := sample(t)
		c.Workflow, _ = filepath.Abs(c.Workflow)
		change(&c)
		b, _ := json.Marshal(c)
		p := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Load(p); err == nil {
			t.Fatalf("accepted invalid config: %s", b)
		}
	}
}

func TestGeneration(t *testing.T) {
	var posts, polls atomic.Int32
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authentication")
		}
		switch r.URL.Path {
		case "/prefix/prompt":
			posts.Add(1)
			var body struct{ Prompt Graph }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Prompt["2"].Inputs["text"] != "a bird" {
				t.Errorf("wrong workflow: %+v, %v", body, err)
			}
			io.WriteString(w, `{"prompt_id":"job-1"}`)
		case "/prefix/history/job-1":
			if polls.Add(1) == 1 {
				io.WriteString(w, `{}`)
				return
			}
			io.WriteString(w, `{"job-1":{"status":{"completed":true,"status_str":"success"},"outputs":{"7":{"images":[{"filename":"one + two.png","subfolder":"folder/x","type":"output"}]},"9":{"images":[{"filename":"ignore.png","type":"temp"}]}}}}`)
		case "/prefix/view":
			if r.URL.Query().Get("filename") != "one + two.png" || r.URL.Query().Get("subfolder") != "folder/x" {
				t.Error("incorrect image query encoding")
			}
			w.Write(pngData.Bytes())
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	t.Setenv("COMFY_TEST_TOKEN", "test-token")
	c, g := sample(t)
	c.BaseURL, c.TokenEnv = up.URL+"/prefix", "COMFY_TEST_TOKEN"
	var log bytes.Buffer
	reply := handle(context.Background(), c, g, Request{"POST", "/v1/images/generations", `{"model":"sdxl","prompt":"a bird"}`}, &log, time.Millisecond)
	if reply.Status != 200 {
		t.Fatalf("%+v", reply)
	}
	body := reply.Body.(map[string]any)
	img := body["data"].([]map[string]string)[0]["b64_json"]
	if img != base64.StdEncoding.EncodeToString(pngData.Bytes()) || posts.Load() != 1 || polls.Load() != 2 || !strings.Contains(log.String(), "job-1") {
		t.Fatal("wrong result or duplicated submission")
	}
}

func TestFailureAndCancellationNeverResubmit(t *testing.T) {
	for _, mode := range []string{"rejected", "failed", "missing-output", "bad-image", "timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var posts atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/prompt":
					posts.Add(1)
					if mode == "rejected" {
						w.WriteHeader(400)
						io.WriteString(w, `{"error":"missing checkpoint"}`)
						return
					}
					io.WriteString(w, `{"prompt_id":"job-fail"}`)
				case "/history/job-fail":
					switch mode {
					case "failed":
						io.WriteString(w, `{"job-fail":{"status":{"status_str":"error","messages":[["execution_error",{"exception_message":"OOM"}]]}}}`)
					case "missing-output":
						io.WriteString(w, `{"job-fail":{"status":{"completed":true},"outputs":{}}}`)
					case "bad-image":
						io.WriteString(w, `{"job-fail":{"status":{"completed":true},"outputs":{"7":{"images":[{"filename":"x.png","type":"output"}]}}}}`)
					case "cancel":
						cancel()
					default:
						io.WriteString(w, `{}`)
					}
				case "/view":
					io.WriteString(w, "<html>login</html>")
				default:
					t.Errorf("unexpected mutation/request %s", r.URL)
				}
			}))
			defer up.Close()
			c, g := sample(t)
			c.BaseURL, c.TimeoutSeconds = up.URL, 1
			reply := handle(ctx, c, g, Request{"POST", "/v1/images/generations", `{"model":"sdxl","prompt":"test"}`}, io.Discard, time.Millisecond)
			if reply.Status != 502 || posts.Load() != 1 {
				t.Fatalf("status=%d submissions=%d", reply.Status, posts.Load())
			}
			b, _ := json.Marshal(reply)
			if mode != "rejected" && !bytes.Contains(b, []byte("job-fail")) {
				t.Fatal("lost prompt_id")
			}
		})
	}
}

func TestNoSecretsOnRedirect(t *testing.T) {
	var reached atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
	}))
	defer destination.Close()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer up.Close()
	c, g := sample(t)
	c.BaseURL = up.URL
	reply := handle(context.Background(), c, g, Request{"POST", "/v1/images/generations", `{"model":"sdxl","prompt":"test"}`}, io.Discard, time.Millisecond)
	if reply.Status != 502 || reached.Load() {
		t.Fatal("followed redirect")
	}
}
