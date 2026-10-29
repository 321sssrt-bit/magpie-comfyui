# 开发说明

本项目唯一职责：`Agent → Magpie 的插件工具接口 → 胶水插件 → ComfyUI 官方 API`。

## 边界与源码

- `index.js`：导出标准 OpenCode 插件函数，仅返回 `tool`。Magpie 原生加载器传入的第二个参数是插件 options；工具上下文只使用已有的 `abort`。
- 连接真源是 `options.connections`，按 ID 分别配置地址与认证环境变量名。工具只向所选连接发请求，不读取或改写 Magpie 账号、供应商配置。
- `ui/settings.html`、`ui/settings.js`、`ui/settings.css`：通过官方 `magpie.panels` 的 `settings` 插槽显示连接表单，使用 `_magpie/bridge.js` 的 `magpie.options.get/set` 读写本插件的选项，沿用 Magpie 的主题与基础样式。
- 插件列表图标由 `package.json` 的 `magpie.icon` 声明，内容与 `assets/icon.png` 一致。
- `resources`、`submit`、`history`、`view` 分别转发官方资源、`/prompt`、`/history/{prompt_id}`、`/view`；不转换、推断、裁剪或保存工作流。
- 所有实现均在本插件包内。禁止增加 Magpie 私有接口、宿主补丁、定制供应商页面、原生中转进程或定制镜像。ComfyUI 也不需要修改。
- 原图响应使用 base64。插件限制单次 HTTP 响应为 16 MiB，最终序列化消息为 31 MiB；这些是插件自身的限制，为 Magpie 宿主的 32 MiB 单行消息上限留出空间。失败报告给调用方，不自动重复提交。

## 维护

运行环境为 Magpie 自带的 Bun；本地语法检查和单元测试可使用 Node.js 22+。没有依赖安装或编译步骤。

本地开发使用 `magpie plugin add /path/to/plugin` 安装源码目录。

```sh
node --check index.js
node --test index.test.js ui/settings.test.js
npm pack --ignore-scripts
```

`index.test.js` 使用内存中的 HTTP 响应替身，覆盖连接隔离、认证、原生图转发、取消、重定向和大小边界；不连接真实 ComfyUI。

`ui/settings.test.js` 使用页面与 options 接口替身检查连接编辑、删除、配置保留及冲突处理，不启动浏览器或 Magpie。

发行文件由 `package.json.files` 指定，包含工具实现、连接设置面板、使用文档、图标和许可证；实际连接配置、凭据、测试输出和生成图片不入包。发布前以当前源码生成产物，不沿用旧候选包。
