# 使用指南

中文 · [English](usage.en.md) · [返回 README](../README.md)

- [连接服务器](#连接服务器)
- [配置生图](#配置生图)
- [多台服务器](#多台服务器)
- [查看模型、LoRA 和节点](#查看模型lora-和节点)
- [鉴权与环境变量](#鉴权与环境变量)
- [取消与失败](#取消与失败)
- [常见问题与停用](#常见问题与停用)

## 连接服务器

需要一台可访问的 ComfyUI，可以运行在本机或另一台电脑。插件自带所需程序，无需安装 Go。

```sh
magpie plugin login comfyui 2
```

按提示填写：

| 项目 | 说明 |
| --- | --- |
| 服务器 URL | ComfyUI 的 HTTP(S) 地址，需要能从运行 Magpie 的电脑访问；留空即 `http://127.0.0.1:8188` |
| 连接名 | 可选，用来区分多个连接 |
| Bearer token 环境变量名 | 可选，填存放 token 的**环境变量名字**，不是 token 本身，见[鉴权与环境变量](#鉴权与环境变量) |
| 工作流配置路径 | 可选，配置文件的绝对路径；留空则只能查看资源，没有可用的生图模型 |

保存前插件会实际访问服务器，检查资源和队列。它不会扫描局域网，也不会启动 ComfyUI 或加载模型。登录时填写的 URL 优先于配置文件里的 `base_url`。

## 配置生图

1. 把 [examples/config.json](../examples/config.json) 和 [examples/sdxl.json](../examples/sdxl.json) 复制到你自己的配置目录。工作流和凭据不要放进插件安装目录。
2. 示例使用 SDXL 工作流。打开 `sdxl.json`，把 `sd_xl_base_1.0.safetensors` 换成服务器上已安装的 SDXL checkpoint 文件名。其他模型架构请使用相应的 API 工作流。
3. 确认 `config.json` 中的 `workflow` 指向 `sdxl.json`。这个路径相对于配置文件本身，而不是 Magpie 的工作目录。
4. 运行 `magpie plugin login comfyui 2`，填写目标 ComfyUI 的 URL，并在最后一项填入 `config.json` 的绝对路径。远端 URL 不要留空，否则会连接本机。登录时会检查工作流节点和下拉选项（包括 checkpoint 名称）。
5. 在 Magpie 的生图模型设置中选择 `comfyui/sdxl`（或你在配置中设定的 `model` ID）。之后使用 Magpie 生图功能的 agent 就能调用它。

### 使用自己的工作流

在 ComfyUI 中把工作流导出为 **API 格式**（节点 ID 对应 `class_type` 和 `inputs`）。编辑器画布格式的 JSON（含 `nodes` 数组）不被接受。插件不会替你生成工作流或挑选 checkpoint。

要使用 LoRA，先在 ComfyUI 工作流中加入对应的 LoRA 加载节点，再导出 API 工作流；读取到 LoRA 文件不等于已启用它。

`config.json` 字段：

| 字段 | 说明 |
| --- | --- |
| `base_url` | 服务器地址，包括反向代理的路径前缀 |
| `token_env` | 可选，存放 Bearer token 的环境变量名 |
| `model` | 在 Magpie 中选择此工作流的 ID，不是 checkpoint 文件名。需以 `sdxl`、`stable-diffusion`、`flux` 或 `qwen-image` 开头，可加后缀区分实例 |
| `name` | 显示名称，默认为模型 ID 加 ComfyUI |
| `workflow` | API 格式工作流文件，相对于配置文件或使用绝对路径 |
| `inputs` | 把生图参数映射到工作流的节点和字段 |
| `output_node` | 返回结果的 `SaveImage` 节点，其他中间输出会被忽略 |
| `timeout_seconds` | 排队、执行和下载的总时限，默认 900 秒；Magpie 的生图超时设置也要留够时间 |

`inputs` 里只有 `prompt` 是必填的，另外可映射 `negative_prompt`、`width`、`height`、`seed`、`batch_size`。映射必须指向工作流中已有的文本或数字字段，不能指向节点之间的连线；未映射的参数保持工作流原值。

生图参数：

- `size`：`宽x高`、`auto` 或省略；指定尺寸需要映射 `width` 和 `height`，省略则沿用工作流尺寸。
- `n`：1–4 张，默认 1；多张需要映射 `batch_size`。
- 输出为 PNG，画质使用工作流默认值（`quality` 留空或 `auto`）。不支持图像编辑和透明背景。

Magpie 当前的生图入口不传递 `negative_prompt` 和 `seed`。负面提示词和固定种子请直接写在工作流里（不映射 `seed` 即使用工作流中的种子）。

## 多台服务器

对每台服务器分别运行一次 `magpie plugin login comfyui 2`，Magpie 会把它们保存为不同的连接，各自使用自己的工作流配置。想按名字选择某台服务器，就给各自的配置设定不同的 `model` ID，例如 `sdxl-local` 和 `sdxl-remote`。

使用相同 `model` ID 的连接，由 Magpie 自己的账号选择规则决定使用哪一个。插件不按 GPU 负载调度，也不保证故障切换。

## 查看模型、LoRA 和节点

在插件目录下运行对应平台的程序，会实时查询服务器并输出 JSON，包括版本、按类别列出的模型文件（含 LoRA）、节点名称和队列数量：

```sh
# 默认本机服务器，端口 8188
bin/linux-arm64/comfyui-worker inspect

# 其他服务器；如有反向代理路径前缀请一并写上
bin/linux-arm64/comfyui-worker -server http://other-computer:8188 inspect

# 使用已有配置，包括其中的 token_env 和工作流检查
bin/linux-arm64/comfyui-worker -config "/absolute/path/config.json" inspect
```

Windows 使用 `bin/win32-x64/comfyui-worker.exe`，macOS 使用 `bin/darwin-arm64/comfyui-worker`。

需要注意：

- 列出的 LoRA 只是服务器上已安装的文件，不是独立的生图模型，也不代表它和某个 checkpoint 匹配。
- 工作流检查只确认节点类型和下拉选项存在，不检查显存、模型兼容性，也不会实际运行工作流。
- 服务器无法访问时命令会报错，不会返回空列表。

## 鉴权与环境变量

如果 ComfyUI 前面有需要 Bearer token 的反向代理：

1. 在启动 Magpie 的环境中设置一个环境变量，例如 `MY_COMFY_TOKEN`。已运行的 Magpie 需要从该环境重新启动，才能继承新变量。
2. 登录时在 token 一项填 `MY_COMFY_TOKEN`，或在配置文件中加上 `"token_env": "MY_COMFY_TOKEN"`。

远端服务器的配置片段示例（合并进完整配置，不能单独使用）：

```json
{
  "base_url": "https://comfy.example.test/comfy",
  "token_env": "MY_COMFY_TOKEN"
}
```

查看需要鉴权的服务器（此处使用 Linux arm64 程序）：

```sh
bin/linux-arm64/comfyui-worker -server https://comfy.example.test/comfy -token-env MY_COMFY_TOKEN inspect
```

token 只存在环境变量里，不会写进保存的连接，也不作为命令行参数传递。HTTPS 证书会被校验；重定向会被拒绝，以免凭据和工作流被悄悄发到别处。插件不会把 ComfyUI 暴露到网络，也不会为它加装认证。

## 取消与失败

- 每次生图只提交一次，然后等待这个任务的结果。插件不会自动重新提交，不会中断共享 GPU 上的任务，也不会清空队列。
- 取消等待或停用插件，**不会撤回已提交到 ComfyUI 的任务**，它可能仍会继续执行。插件已取得的任务 ID 会附在后续错误中；未取得 ID 时也应先检查 ComfyUI 队列，再决定是否重试。
- Magpie 自己的账号和分组回退策略不由插件控制；如果不能接受重复任务，请不要为生图开启回退。

## 常见问题与停用

**在 Magpie 中看不到 `comfyui`？**
如果关闭了「设置 → 隐私 → 订阅账号」，或设置了 `MAGPIE_NO_ACCOUNTS=1`，这个 provider 也会被隐藏。

**能看到连接，但没有生图模型？**
检查是否填写了工作流配置、`model` ID 前缀是否符合要求、checkpoint 是否已安装在服务器上。

**怎么停用或重新启用？**

```sh
magpie plugin off magpie-comfyui-link
magpie plugin on magpie-comfyui-link
```

停用插件不会停止 ComfyUI，也不会取消已提交的任务。

**其他平台能用吗？**
预编译包包含 Linux arm64、Windows x64 和 macOS Apple Silicon（arm64）的程序。在 Linux x64、Intel Mac、Windows ARM 或 Rosetta 下的 x64 运行时上，插件会明确报错。Apple Silicon 上请使用 arm64 的 Bun。

**还有哪些暂不支持？**
图像编辑、参考图上传、视频、模型下载和按设备自动调度。

## 许可证

[MIT](../LICENSE)
