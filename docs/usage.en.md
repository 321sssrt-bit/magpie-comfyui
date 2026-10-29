# Usage guide

[中文](usage.zh-CN.md) · English · [Back to README](../README.en.md)

- [Connect a server](#connect-a-server)
- [Set up image generation](#set-up-image-generation)
- [Multiple servers](#multiple-servers)
- [Browse models, LoRAs and nodes](#browse-models-loras-and-nodes)
- [Authentication and environment variables](#authentication-and-environment-variables)
- [Cancellation and failures](#cancellation-and-failures)
- [FAQ and disabling](#faq-and-disabling)

## Connect a server

You need a reachable ComfyUI server, running locally or on another computer. The plugin includes the required programs; no Go installation is needed.

```sh
magpie plugin login comfyui 2
```

You will be asked for:

| Field | Notes |
| --- | --- |
| Server URL | ComfyUI's HTTP(S) address, reachable from the computer running Magpie; blank means `http://127.0.0.1:8188` |
| Connection name | Optional, to tell connections apart |
| Bearer token environment variable name | Optional; the **name of the environment variable** holding the token, not the token itself. See [Authentication and environment variables](#authentication-and-environment-variables) |
| Workflow configuration path | Optional absolute path to a configuration file; without it the connection can only browse resources and offers no image model |

Before saving, the plugin contacts the real server and checks its resources and queue. It does not scan your LAN, start ComfyUI or load models. The URL entered at login takes precedence over `base_url` in the configuration file.

## Set up image generation

1. Copy [examples/config.json](../examples/config.json) and [examples/sdxl.json](../examples/sdxl.json) to your own configuration folder. Keep workflows and credentials out of the plugin's install folder.
2. The example is an SDXL workflow. Open `sdxl.json` and replace `sd_xl_base_1.0.safetensors` with an SDXL checkpoint installed on your server. Other model architectures need their corresponding API workflows.
3. Make sure `workflow` in `config.json` points to `sdxl.json`. The path is relative to the configuration file, not Magpie's working directory.
4. Run `magpie plugin login comfyui 2`, enter the target ComfyUI URL, and enter the absolute path to `config.json` in the last prompt. Do not leave a remote URL blank: that selects the local server. Login checks the workflow's nodes and dropdown values, including the checkpoint name.
5. In Magpie's image-generation model setting, choose `comfyui/sdxl` (or the `model` ID from your configuration). Agents using Magpie's image generation can then call it.

### Using your own workflow

Export the workflow from ComfyUI in **API format** (node IDs mapped to `class_type` and `inputs`). Editor canvas JSON with a `nodes` array is not accepted. The plugin does not build workflows or choose checkpoints for you.

To use a LoRA, add the corresponding LoRA loader node in ComfyUI before exporting the API workflow. Discovering a LoRA file does not enable it automatically.

`config.json` fields:

| Field | Meaning |
| --- | --- |
| `base_url` | Server address, including any reverse-proxy path prefix |
| `token_env` | Optional environment variable holding a Bearer token |
| `model` | The ID used to select this workflow in Magpie, not a checkpoint filename. Must start with `sdxl`, `stable-diffusion`, `flux` or `qwen-image`; add a suffix to tell instances apart |
| `name` | Display name; defaults to the model ID plus ComfyUI |
| `workflow` | API-format workflow file, relative to the configuration or absolute |
| `inputs` | Maps generation parameters to workflow nodes and fields |
| `output_node` | The `SaveImage` node whose results are returned; other outputs are ignored |
| `timeout_seconds` | Total time for queueing, running and downloading; default 900 seconds. Allow enough time in Magpie's image timeout setting too |

Only `prompt` is required in `inputs`; you can also map `negative_prompt`, `width`, `height`, `seed` and `batch_size`. Mappings must point to existing text or number fields in the workflow, not links between nodes. Unmapped parameters keep the workflow's values.

Generation parameters:

- `size`: `WIDTHxHEIGHT`, `auto`, or omitted. A size needs `width` and `height` mappings; omitting it keeps the workflow's size.
- `n`: 1–4 images, default 1; more than one needs a `batch_size` mapping.
- Output is PNG with the workflow's default quality (`quality` omitted or `auto`). Image editing and transparency are not supported.

Magpie's current image-generation endpoint does not forward `negative_prompt` or `seed`. Put the negative prompt and a fixed seed directly in the workflow (leave `seed` unmapped to use the workflow's seed).

## Multiple servers

Run `magpie plugin login comfyui 2` once for each server. Magpie saves them as separate connections, each with its own workflow configuration. To pick a server by name, give each configuration a distinct `model` ID, such as `sdxl-local` and `sdxl-remote`.

When connections share a `model` ID, Magpie's own account-selection rules decide which one is used. The plugin does not schedule by GPU load or guarantee failover.

## Browse models, LoRAs and nodes

From the plugin folder, run the program for your platform. It queries the server live and prints JSON with the version, model files by category (including LoRAs), node names and queue counts:

```sh
# Default local server, port 8188
bin/linux-arm64/comfyui-worker inspect

# Another server; include any reverse-proxy path prefix
bin/linux-arm64/comfyui-worker -server http://other-computer:8188 inspect

# An existing configuration, including its token_env and workflow checks
bin/linux-arm64/comfyui-worker -config "/absolute/path/config.json" inspect
```

On Windows use `bin/win32-x64/comfyui-worker.exe`; on macOS use `bin/darwin-arm64/comfyui-worker`.

Keep in mind:

- Listed LoRAs are files installed on the server, not standalone image models, and finding one does not mean it fits a given checkpoint.
- Workflow checks only confirm that node types and dropdown values exist. They don't check memory or model compatibility, and they don't run the workflow.
- If the server is unreachable, the command fails instead of returning an empty list.

## Authentication and environment variables

If ComfyUI sits behind a reverse proxy that requires a Bearer token:

1. Set an environment variable, for example `MY_COMFY_TOKEN`, in the environment Magpie starts from. If Magpie is already running, restart it from that environment so it inherits the new variable.
2. Enter `MY_COMFY_TOKEN` in the token prompt at login, or add `"token_env": "MY_COMFY_TOKEN"` to the configuration file.

Example fields for a remote server (merge into a complete configuration; not usable on their own):

```json
{
  "base_url": "https://comfy.example.test/comfy",
  "token_env": "MY_COMFY_TOKEN"
}
```

Inspect an authenticated server (using the Linux arm64 program here):

```sh
bin/linux-arm64/comfyui-worker -server https://comfy.example.test/comfy -token-env MY_COMFY_TOKEN inspect
```

The token stays in the environment variable. It is never written to the saved connection or passed as a command-line argument. HTTPS certificates are verified, and redirects are refused so credentials and workflows are not silently sent elsewhere. The plugin does not expose ComfyUI to the network or add authentication to it.

## Cancellation and failures

- Each generation is submitted once, then the plugin waits for that job's result. It never resubmits automatically, interrupts work on a shared GPU, or clears the queue.
- Cancelling a wait or disabling the plugin **does not withdraw a job already submitted to ComfyUI**, which may keep running. Once the plugin has received a job ID, later errors include it. Even without an ID, check the ComfyUI queue before deciding to retry.
- Magpie's own account and group fallback is outside the plugin's control. If duplicate jobs are unacceptable, don't enable fallback for image generation.

## FAQ and disabling

**`comfyui` doesn't appear in Magpie?**
Turning off **Settings → Privacy → Subscription accounts**, or setting `MAGPIE_NO_ACCOUNTS=1`, also hides this provider.

**The connection shows up but there is no image model?**
Check that a workflow configuration was entered, that the `model` ID has an accepted prefix, and that the checkpoint is installed on the server.

**How do I disable or re-enable it?**

```sh
magpie plugin off magpie-comfyui-link
magpie plugin on magpie-comfyui-link
```

Disabling the plugin does not stop ComfyUI or cancel submitted jobs.

**Does it work on other platforms?**
The prebuilt package contains programs for Linux arm64, Windows x64 and macOS Apple Silicon (arm64). On Linux x64, Intel Macs, Windows ARM, or x64 runtimes under Rosetta, the plugin fails with a clear error. On Apple Silicon, use an arm64 Bun.

**What isn't supported yet?**
Image editing, reference image uploads, video, model downloads and automatic device scheduling.

## License

[MIT](../LICENSE)
