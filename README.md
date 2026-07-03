# better-speedtest

better-speedtest 是一个跨平台、多源测速工具，也可以作为 UFI-TOOLS 面板插件的测速后端。它聚合三类测速源：

- 全球网测 / cnspeed：面向中国大陆的分省、分运营商测速节点。
- 自定义 CDN 源：适合下载测速、跨运营商兜底、按配置热更新。
- Speedtest.net / Ookla：适合海外网络和上传测速场景。

CLI 默认输出人类可读的实时进度；插件模式使用 NDJSON 输出驱动前端仪表盘。程序非常驻，测速完成即退出。

## Features

- 自动选源：中国大陆优先全球网测，海外自动在 CDN 与 Speedtest.net 间择优。
- 多源聚合：`--multi` 可同时使用多个最快源，突破单源限速。
- 自适应连接数：从较低连接数起步，按吞吐提升自动加连接，遇到平台期自动收敛。
- 真实吞吐采样：Linux 优先读取 `/proc/net/dev`，非 Linux 自动回退到应用层字节统计。
- 定位与运营商识别：手动覆盖、AT/PLMN、IP 定位多级合并。
- 配置驱动：节点、端点、下载源、线程、时长等都可以通过 JSON 配置覆盖。
- UFI-TOOLS 插件：提供浏览器面板、初始化下载、节点选择、实时曲线和结果展示。

## Downloads

GitHub Release 会包含裸二进制和归档包。插件初始化需要下载裸二进制，命名格式如下：

```text
better-speedtest-linux-amd64
better-speedtest-linux-arm64
better-speedtest-linux-armv7
better-speedtest-linux-386
better-speedtest-linux-mips
better-speedtest-linux-mipsle
better-speedtest-windows-amd64.exe
better-speedtest-windows-arm64.exe
better-speedtest-darwin-amd64
better-speedtest-darwin-arm64
better-speedtest-android-arm64
```

UFI-TOOLS 插件内置下载源：

```text
https://tapan.top/d/UFITOOLS-U60Pro/file/better-speedtest-linux-{arch}
https://github.com/Gaimoydev/better-speedtest/releases/latest/download/better-speedtest-linux-{arch}
```

其中 `{arch}` 由设备自动识别，例如 `x86_64 -> amd64`、`aarch64 -> arm64`。你给出的 amd64 国内源会展开为：

```text
https://tapan.top/d/UFITOOLS-U60Pro/file/better-speedtest-linux-amd64
```

## Build

需要 Go 1.24 或更新版本。项目不依赖 CGO，交叉编译不需要额外 C 工具链。

```sh
go test ./...
go build -trimpath -ldflags="-s -w" -o better-speedtest ./cmd/better-speedtest
```

构建设备端 arm64 二进制：

```sh
sh scripts/build.sh
```

构建全部 Release 资产：

```sh
sh scripts/release.sh
sh scripts/release.sh --archive
```

推送 `v*` tag 后，`.github/workflows/release.yml` 会自动运行测试、交叉编译并发布 GitHub Release。

## CLI Usage

```sh
better-speedtest version
better-speedtest ip
better-speedtest nodes --src all
better-speedtest nodes --src cnspeed --full
better-speedtest test --src auto
better-speedtest test --src cnspeed
better-speedtest test --src ookla
better-speedtest test --src cdn --node "Cloudflare"
better-speedtest test --src auto --multi 4
better-speedtest test --src auto --no-upload
better-speedtest test --src auto --json
better-speedtest update
```

常用参数：

- `--src auto|cnspeed|cdn|ookla`：选择测速源。
- `--node <关键字>`：指定节点、城市、IP 或 CDN/Ookla 源名。
- `--dur <秒>`：覆盖单方向测速时长。
- `--multi <N>`：多源聚合测速。
- `--no-upload` / `--no-download`：只测下行或上行。
- `--json`：输出 NDJSON，适合插件或脚本消费。

默认输出实时进度条和最终摘要。`--json` 模式会输出形如：

```json
{"phase":"download","t":3,"mbps":420.5,"peak":455.1}
{"phase":"result","node":"...","source":"ookla","dl_avg":510.2,"ul_avg":85.4,"ping":23.1,"jitter":4.8}
DONE
```

## Configuration

配置文件优先级：

1. `BETTER_SPEEDTEST_CONFIG` 环境变量。
2. Linux 默认：`/data/plugins/better-speedtest/config.json`。
3. 其他平台：用户配置目录下的 `better-speedtest/config.json`。

内置默认配置在 [assets/config.default.json](assets/config.default.json)。用户配置会和内置配置深合并。

MCC-MNC 运营商表会随二进制内置一份，默认可离线使用。运行 `ip`、`nodes`、`test` 或打开 UFI-TOOLS 插件面板时，程序会检查本地表是否缺失、损坏或超过 `updates.mccmnc_refresh_days`，默认 7 天；需要更新时会走后台任务，不阻塞定位、节点加载或测速主流程。也可以手动执行：

```sh
better-speedtest update
better-speedtest update --if-stale
```

示例：

```json
{
  "manual": {
    "carrier": "移动",
    "province": "广东",
    "city": "广州"
  },
  "engine": {
    "threads_dl": 8,
    "threads_ul": 4,
    "threads_dl_max": 64,
    "threads_ul_max": 32,
    "duration_s": 15,
    "wan_iface": "auto"
  },
  "cdn_sources": [
    {
      "name": "自定义 CDN",
      "dl": "https://example.com/large-file.bin",
      "region": "cn"
    }
  ],
  "install": {
    "binary_url": "https://tapan.top/d/UFITOOLS-U60Pro/file/better-speedtest-linux-{arch}",
    "proxy": "",
    "dest": "/data/plugins/better-speedtest/better-speedtest"
  }
}
```

## UFI-TOOLS Plugin

插件文件位于 [plugin/better-speedtest.txt](plugin/better-speedtest.txt)。在 UFI-TOOLS 面板里打开“插件功能”，选择该文件导入即可。

插件功能：

- 首次打开时自动下载安装 `better-speedtest` 二进制。
- 支持自动、全球网测、Ookla、自定义 CDN 四种测速类型。
- 支持上下行、仅下行、仅上行。
- 支持节点/源选择、实时曲线、延迟/抖动展示。
- 设置会写入 `/data/plugins/better-speedtest/config.json`，刷新后仍然生效。

UFI-TOOLS 设备常见架构是 `aarch64`，插件会下载：

```text
better-speedtest-linux-arm64
```

## Project Layout

```text
cmd/better-speedtest/       CLI entrypoint
assets/                    embedded default config and MCC-MNC table
internal/config/           config loading and deep merge
internal/engine/           transfer workers, adaptive connection ramp, stats
internal/geo/              carrier and location detection
internal/httpx/            HTTP clients and TLS profiles
internal/nodes/            cnspeed, CDN and Ookla node providers
internal/report/           progress output and NDJSON result format
internal/selector/         cnspeed node ordering
plugin/                    UFI-TOOLS plugin distribution file
scripts/                   build, deploy and release scripts
```

## Notes

better-speedtest is intended for measuring networks you own or are authorized to test. Some providers may rate-limit, reject datacenter IPs, or change endpoints. Keep test duration and concurrency reasonable, especially on mobile data connections.

## License

MIT
