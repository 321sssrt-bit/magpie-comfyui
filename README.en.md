# magpie-comfyui-link

[中文](https://unpkg.com/magpie-comfyui-link@0.1.0/README.md) · English

Connect local or remote ComfyUI to Magpie, browse models and LoRAs, and generate images with your own workflows.

Supports Linux arm64, Windows x64 and macOS Apple Silicon.

## Install and connect

```sh
magpie plugin add magpie-comfyui-link
magpie plugin login comfyui 2
```

A blank URL means `http://127.0.0.1:8188`. If you skip the workflow configuration for now, the connection can browse resources but cannot generate images yet.

## Next steps

- **[Usage guide](https://unpkg.com/magpie-comfyui-link@0.1.0/docs/usage.en.md)**: workflows, multiple servers, models and LoRAs, authentication and troubleshooting.
- [Example configuration](https://unpkg.com/magpie-comfyui-link@0.1.0/examples/config.json) · [SDXL workflow](https://unpkg.com/magpie-comfyui-link@0.1.0/examples/sdxl.json).
- [Maintainers: build from source](https://unpkg.com/magpie-comfyui-link@0.1.0/docs/development.md).

[MIT](https://unpkg.com/magpie-comfyui-link@0.1.0/LICENSE) · [npm](https://www.npmjs.com/package/magpie-comfyui-link)
