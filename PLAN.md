# U60Pro 设备内置测速 — 项目规划

## 0. 目标
在 ZTE U60Pro(随身WiFi)上做**设备内置多源测速**,聚合「全球网测(cnspeed/CAICT)」+「自收集 CDN 源」,通过面板浏览器插件驱动一个 **Go CLI**(非常驻)完成测速并回显前端。

## 1. 设备事实(已实测锁定)
| 项 | 值 |
|---|---|
| 厂商/系统 | ZTE / OpenWrt 23.05.4(高通 rmnet 平台,Android 内核) |
| 架构 | **aarch64**(Cortex-A55 ×4)→ `GOARCH=arm64` |
| 内存 / 存储 | 1.6G(可用 ~570M) / `/data` 剩 ~1G |
| WAN 口 | **rmnet_data0**(默认路由蜂窝口,运行时从默认路由自动探测) |
| SIM | 中国移动(IMSI 46002 / 出网 46000) |
| 编译目标 | `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` 全静态(musl/OpenWrt 无依赖) |
| 插件模型 | 浏览器 JS → `runShellWithRoot` → `POST /api/run_shell {cmd,timeout}` → 跑我们的 CLI(root) |
| AT 口 | `/dev/smd11`(smd7 / at_mdm0 等价,探测+缓存) |

## 2. 范围(已定)
- ❌ **不做花瓣测速**(需 .so/凭据/自愈,冗余;cnspeed 覆盖国内运营商更全)。设备端**无 .so、无 cgo、无密钥、无常驻**。
- ✅ **两类节点源**:
  1. **cnspeed(全球网测)**:国内分省·分运营商真实节点。纯 Go,**插移动卡的设备上才跑得通**(非移动 IP 返 403)。token=MD5 公式、全量表 DES(key `dw!@#$%^`)、就近 `mobilematch_many.php`(明文)、裸 HTTP 传输。
  2. **自收集 CDN 源**(移动/华为/微信/Steam/Apple/Azure/Google/Cachefly/Cloudflare):下载型单 URL,**放可热加载配置**(微信 token 会过期→改配置即可),多连接 Range GET 兜文件大小。

## 3. 架构(CLI 一次性,零常驻)
```
浏览器插件(速度测试.js)                         better-speedtest (Go CLI, 一次性)
  ├─ 注入 UI / 手动填运营商(保存)   runShellWithRoot   ├─ test 后台跑: >/tmp/better-speedtest.log 2>&1 &
  ├─ 点"开始" ────────────────────────────────────▶ ├─ 每秒写 NDJSON 进度行
  ├─ 每秒 cat /tmp/better-speedtest.log ◀──────────────────┤ ├─ /proc/net/dev 真实吞吐
  └─ 解析末行 result → 富 toast 展示                └─ 写 result + DONE,退出
```
二进制只在一次测速(~30s)期间存在,跑完即退,**不吃常驻内存**。

## 4. CLI 契约
```
better-speedtest ip                    → {ip,carrier,province,city,lat,lon}     # 定位+运营商
better-speedtest nodes [--src cnspeed|cdn|all] [--nearest]  → JSON 节点表
better-speedtest test --auto [--src ...] [--dur 15]         → 自动选点(同运营商+就近)
better-speedtest test --node <id>                           → 指定节点
  全局: --iface auto|rmnet_data0  --threads-dl 8 --threads-ul 4  --carrier 移动(手动覆盖)
  输出: 逐行 NDJSON {"t":3,"phase":"download","mbps":452.1,"peak":..}
        末行 {"phase":"result","dl_avg","dl_peak","ul_avg","ul_peak","ping","jitter","node"} + "DONE"
```

## 5. 运营商/定位检测(优先级,手动最高)
**运营商 carrier:** `手动填(有则最高) > AT(COPS3,2/CIMI→PLMN→映射,离线) > aapq.net asname(utls) > ipip.net > cnspeed getIpLocSP`
**位置 location(含经纬度):** `手动填 > aapq.net(lat/lon+city,utls) > ipip.net(省/市) > cnspeed getIpLocSP(省;市常空)`

- **AT 读口坑**:裸 `cat $port &` 不被 SIGTERM 杀、挂死会话 → Go 里 `OpenFile`+goroutine 限时读(~1.5s)+关闭。
- **aapq.net**:JA3+UA 双闸,需 **utls 伪装 Chrome ClientHello + Chrome UA**;可能还查 h2 指纹(设备实测)。给最全数据(ASN/运营商/经纬度/省市)。跨域→**必须 CLI 抓,不能插件抓**。
- **ipip.net**:无指纹闸,稳妥兜底,返回 `["国","省","市","","运营商"]`。
- **cnspeed getIpLocSP**:明文 `IP|["国","省","市","区","运营商"]|..|运营商|code`。
- 注:cnspeed `mobilematch_many.php` 光用 IP 就能返回就近节点,省市可缺。

## 6. MCC-MNC 数据
- **内置** `mcc-mnc.csv`(`;`分隔+BOM,2696 条,最全:中国 12 码 + 独有 Region 字段)→ `go:embed`。
- **中国**:硬编码 12 PLMN → 移动/联通/电信;**手补 `46015→广电`**(三家表都没有,选点回退移动)。
- **海外**:`Lookup(plmn).Region` 分区路由;查不到用 MCC 前3位段位兜底;展示名用 CSV Operator/Brand 或 AT COPS 长名。
- **自动更新**:7 天从 `https://mcc-mnc.net/mcc-mnc.csv`(格式已确认一致)拉新,校验表头后落 `/data/plugins/better-speedtest/mcc-mnc.csv`,坏了回退内置;该文件存在则覆盖内置。
- `codes.json`/`b.json` 不用(中国码更少、无 Region、b 缺 46011 有噪声)。

## 7. 选点逻辑
```
mcc==460(中国大陆): cnspeed 同运营商+就近(mobilematch)→ 由近到远 fallback,叠 CDN
海外(已知区域):     跳过 cnspeed,只用 CDN,按 Region 排序(亚洲/欧洲/北美…)
未知:               纯 CDN 全测取最快
经纬度可用时(aapq): "由近到远"用真实距离排序,否则按省市匹配
```

## 8. 测量(YD/T,设备最准)
- **/proc/net/dev 锁 WAN 口(rmnet_data0 自动探测)** 取 RX/TX 增量=真实蜂窝吞吐(含开销);应用层字节数交叉校验。
- N≥4 并发,1s 采样,**avg=5–15s 均值、peak=最大**;jitter 从 ping RTT 方差;ping 用 TCP 连接 RTT(ICMP 需 root)。

## 9. 项目结构(Go)
```
better-speedtest/
  go.mod                        # module u60speedtest (Go 1.22+)
  cmd/better-speedtest/main.go         # 子命令分发(stdlib flag)
  internal/
    geo/
      carrier.go                # AT 端口探测 + 限时读 + COPS/CIMI → PLMN
      mccmnc.go                 # go:embed csv + 查表 + 中国映射 + Region + 自动更新
      iploc.go                  # aapq(utls)→ipip→cnspeed getIpLocSP
      manual.go                 # 读手动覆盖(config)
      resolve.go                # 按优先级合并 → {carrier,province,city,lat,lon,ip}
    nodes/
      model.go                  # 统一 Node schema
      cnspeed.go                # md5 token / dovalid / mobilematch(就近) / serverlist 拉取
      cnspeed_des.go            # DES-ECB dw!@#$%^ 解 serverlist_encrypt.json
      cdn.go                    # 读 config CDN 源 → 下载测试(多连接 Range)
    engine/
      transfer.go               # N 并发 GET(下)/POST(上),Range/循环,1s 采样
      procnetdev.go             # /proc/net/dev RX/TX 增量(WAN 自动探测)
      stats.go                  # YD/T avg/peak + jitter
      ping.go                   # TCP RTT
    selector/pick.go            # 同运营商+就近 → fallback;海外按 Region
    report/progress.go          # NDJSON 进度 + 最终 result
    config/config.go            # /data/plugins/better-speedtest/config.json 热加载
    httpx/client.go             # utls Chrome 客户端(aapq)+ 普通客户端
  assets/
    mcc-mnc.csv                 # 内置(embed)
    config.default.json         # 默认 CDN 源 + cnspeed 参数
  plugin/better-speedtest.js         # 前端(装二进制+UI+轮询+手动填运营商+展示)
  scripts/build.sh              # GOARCH=arm64 CGO_ENABLED=0 + upx
  scripts/deploy.sh             # scp 到设备联调
  PLAN.md  README.md
```

## 10. 配置文件 `/data/plugins/better-speedtest/config.json`(用户可改·热加载)
```json
{
  "cnspeed": { "enabled": true, "threads_dl": 8, "threads_ul": 4 },
  "manual": { "carrier": "", "province": "", "city": "" },
  "cdn_sources": [
    {"name":"移动云盘","dl":"https://yun.mcloud.139.com/.../mCloud_Setup-001.exe","region":"cn"},
    {"name":"华为","dl":"https://consumer.huawei.com/.../privacy-safe-center.webm","region":"cn"},
    {"name":"微信","dl":"https://finder.video.qq.com/...&token=...","region":"cn","note":"token会过期,失效换URL"},
    {"name":"Cloudflare","dl":"https://speed.cloudflare.com/__down?bytes=200000000","ul":"https://speed.cloudflare.com/__up","region":"global"},
    {"name":"Cachefly","dl":"https://cachefly.cachefly.net/100mb.test","region":"global"}
  ]
}
```
- cnspeed 常量(token 公式/DES key/端点)**编死**;CDN 源与手动运营商**放配置**。手动运营商若填,优先级最高。

## 11. 构建 & 分发
- `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w"` → upx → **~3-5MB 单文件**。
- GitHub Release 出 `better-speedtest-linux-arm64`;插件"安装"按钮 `ghfast.top` 代理兜底下载 → `/data/plugins/better-speedtest/better-speedtest` → `chmod +x`(照 `OpenList插件.js`)。
- **无开机自启**(CLI 按需),不碰 `/etc/rc.local`。

## 12. 计划表
| 阶段 | 任务 | 交付 | 验收(设备 SSH/面板) |
|---|---|---|---|
| **P0 脚手架** | go.mod、main 子命令骨架、httpx(utls+plain)、config、build.sh | 能编出 arm64 静态二进制 | `./better-speedtest version` 在设备跑通 |
| **P1 定位** | geo 全套(AT carrier / mccmnc embed+更新 / iploc 三级 / manual / resolve) | `better-speedtest ip` | 设备返回 `移动/广东/广州/经纬度` |
| **P2 节点** | nodes(cnspeed token+DES+mobilematch、cdn 读配置) | `better-speedtest nodes --src all` | 设备列出 cnspeed 就近 + CDN 源 |
| **P3 引擎** | engine(transfer/procnetdev/stats/ping)+ selector | `better-speedtest test --auto` NDJSON+result | 设备实测下/上行 Mbps,对比 App 官方 |
| **P4 插件** | 速度测试.js(装二进制+UI+轮询+手动填运营商保存+展示) | 面板插件 | Chrome MCP 贴进 `custom_head`,点按钮跑通 |
| **P5 收尾** | 表自动更新、jitter、错误处理、GitHub Release+代理安装、README | 发布版 | 端到端联调 |

## 13. 待确认(不阻塞 P0/P1)
1. aapq.net 的 hex 子域是否稳定(临时的→降兜底);utls 是否需再补 h2 指纹(设备实测)。
2. GitHub 仓库地址(出 Release);无则先 scp 联调。
