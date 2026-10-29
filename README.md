# magpie-comfyui-link

中文 · [English](https://unpkg.com/magpie-comfyui-link@0.1.0/README.en.md)

将本机或远端 ComfyUI 接入 Magpie，查看模型与 LoRA，通过自定义工作流生成图片。

支持 Linux arm64、Windows x64 和 macOS Apple Silicon。

## 安装与连接

```sh
magpie plugin add magpie-comfyui-link
magpie plugin login comfyui 2
```

登录时 URL 留空即 `http://127.0.0.1:8188`。暂不填工作流配置的话，这个连接只能查看资源，还不能生图。

## 下一步

- **[使用指南](https://unpkg.com/magpie-comfyui-link@0.1.0/docs/usage.zh-CN.md)**：配置生图、多台服务器、模型和 LoRA、鉴权与排障。
- [示例配置](https://unpkg.com/magpie-comfyui-link@0.1.0/examples/config.json) · [SDXL 工作流](https://unpkg.com/magpie-comfyui-link@0.1.0/examples/sdxl.json)。
- [维护者：从源码构建](https://unpkg.com/magpie-comfyui-link@0.1.0/docs/development.md)。

[MIT](https://unpkg.com/magpie-comfyui-link@0.1.0/LICENSE) · [npm](https://www.npmjs.com/package/magpie-comfyui-link)
