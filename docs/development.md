# 从源码构建 / Build from source

[中文入口](../README.md) · [English entry](../README.en.md)

## 中文

维护者需要 Go 1.22+ 和 Node.js（也可用 Bun）；没有第三方 Go 或 npm 依赖。在源码目录手动执行：

```sh
node scripts/build.mjs --all
```

程序分别写入 `bin/linux-arm64/`、`bin/win32-x64/`、`bin/darwin-arm64/`。省略 `--all` 只构建当前支持的平台；Bun 可替换命令中的 `node`。

构建会替换对应程序。安装插件时不会执行此命令，普通用户可直接使用预编译包。

## English

Maintainers need Go 1.22+ and Node.js (or Bun). There are no third-party Go or npm dependencies. From the source directory, run:

```sh
node scripts/build.mjs --all
```

The programs are written to `bin/linux-arm64/`, `bin/win32-x64/` and `bin/darwin-arm64/`. Omit `--all` to build only the current supported platform; `bun` can replace `node`.

Building replaces the corresponding programs. This command does not run during plugin installation; users can install the prebuilt package directly.
