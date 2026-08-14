# Easy Proxies

[English](README.md) | 简体中文

Easy Proxies 是一个基于 sing-box 的代理池管理工具。

目标是把大量上游节点统一成稳定的本地 HTTP/SOCKS5 代理入口，同时支持按节点独立端口访问。

## 当前能力

- 运行模式：`pool`、`multi-port`、`hybrid`。
- 实际构建的上游协议：`vmess`、`vless`、`trojan`、`ss/shadowsocks`、`hysteria2/hy2`、`socks5/socks`、`http/https`、`anytls`、`tuic`。
- 节点来源：
  - `config.yaml` 的 `nodes`
  - `nodes_file`（每行一个 URI）
  - `subscriptions`（支持 Base64/纯文本/Clash YAML 解析）
- 自动健康检查、失败熔断和黑名单恢复。
- 拨号失败自动重试：节点拨号失败时自动切换到另一个健康节点重试（可配置次数）。
- 端口稳定（multi-port/hybrid）：每个节点按 URI 稳定标识（忽略名称与参数顺序），订阅刷新或重启后保持同一本地端口（持久化到 config.yaml 同目录的 `node_ports.json`）。
- 粘性代理：可选的独立端口，按来源 IP 把客户端固定绑定到同一上游节点，保持出口 IP 稳定，与轮询的 pool 入口共存（仅 pool/hybrid 模式）。
- Web 管理面板 + API：
  - 节点状态/探测/导出
  - **手动拉黑/解封节点**
  - 动态设置（`external_ip`、`probe_target`、`skip_cert_verify`、`geoip`）
  - 节点配置增删改查 + 重载
  - 订阅状态查询 + 手动刷新 + **保存即时生效**
  - **实时日志控制台**（最近 1000 行，WebSocket 流式传输）
- 新增可配置 DNS 解析器（对 VMess 域名节点非常关键）。
- 可选 GeoIP 标记（支持 JP/KR/US/HK/TW/SG 地域分区，可在 WebUI 中开关，支持自动更新和热重载）。
- **可配置日志轮转**，支持大小限制、备份数量和压缩。

## 快速开始

### 1）准备配置

```bash
cp config.example.yaml config.yaml
cp nodes.example nodes.txt
```

编辑 `config.yaml`，并配置节点来源（`nodes.txt` / `subscriptions` / `nodes`）。

> 为什么要 `touch nodes.txt`？如果你用文件级挂载（如 `-v ./data/nodes.txt:/etc/easy_proxies/nodes.txt`）而宿主机上该文件不存在，Docker 会在宿主机上创建一个名为 `nodes.txt` 的**目录**并挂载进去，容器内就出现"本应是文件却是目录"的坑。预先创建文件（或直接挂载**目录** `./data:/etc/easy_proxies`，首启动会自动生成文件）可避免。若已踩坑：`rm -rf ./data/nodes.txt && touch ./data/nodes.txt` 后重启。

### 2）启动

Docker：

```bash
docker compose up -d
```

本地运行：

```bash
go run ./cmd/easy_proxies -config config.yaml
```

### 3）本机直连 VPNCheap（Windows / macOS 推荐）

本模式只启动 `easy_proxies`，不启动 `proxypool`，也不监听 `18080`。启动脚本会从本机 VPNCheap 状态文件自动读取订阅地址，生成受保护的 `runtime/easy_proxies-config.yaml`，然后以 `hybrid` 模式运行：`127.0.0.1:2323` 为无认证代理池入口，每个节点还有稳定的独立 HTTP 端口（默认从 `24000` 起），`127.0.0.1:9091` 为 WebUI/API。

#### Windows

```powershell
powershell -ExecutionPolicy Bypass -File runtime\proxy-chain.ps1 start
powershell -ExecutionPolicy Bypass -File runtime\proxy-chain.ps1 status
powershell -ExecutionPolicy Bypass -File runtime\proxy-chain.ps1 stop
```

#### macOS

```bash
make build
make run
make restart
make status
make stop
make package
```

- VPNCheap Windows 客户端已在本机登录，状态文件位于 `%APPDATA%\vpncheap\app_state.json`。
- VPNCheap macOS 客户端已在本机登录，订阅地址来自 `~/Library/Containers/com.vpncheap.macnative/Data/Library/Preferences/com.vpncheap.macnative.plist`。
- `runtime\easy_proxies-config.yaml` 自动生成且已被 Git 忽略，不要手动提交或复制其中的订阅地址。
- macOS 生成文件权限为仅当前用户可读写，且同样不会提交或打印订阅地址。
- 订阅地址缺失、非法或读取失败时，`start` 会失败退出，不会回退到 `proxypool`。
- 所有代理监听仅绑定 `127.0.0.1` 且不启用用户名密码；如需远程使用，先改监听地址并加认证，不要直接暴露公网。
- macOS 的 `make build`、`make run`、`make package` 不再依赖 `proxypool`；Linux 保持原启动方式，Windows `-Legacy` 兼容路径仍保留。
- 如需保留旧 `proxypool` 链路，可显式运行 `powershell -ExecutionPolicy Bypass -File runtime\proxy-chain.ps1 start -Legacy`。

#### 导出到 9router

9router 的代理池按每行一个代理地址导入。使用 WebUI 的 `9router 导出` 按钮，或直接下载：

```text
http://127.0.0.1:9091/api/export?target=9router
```

导出内容只包含当前运行节点的 HTTP 地址，每行一个，例如：

```text
http://127.0.0.1:24000
http://127.0.0.1:24001
http://127.0.0.1:24002
```

该导出包含全部已建立本地监听的节点，不按健康状态过滤；不带用户名密码，也不包含注释、池入口、GeoIP 入口或 SOCKS 地址。把导出内容复制后，在 9router 的 Proxy Pools 中批量导入即可。

此方式要求 9router 与 easy_proxies 位于同一主机的网络命名空间。当前节点地址只绑定 `127.0.0.1`，Docker 容器或其他机器不能直接使用这些地址。

#### 验证

```powershell
powershell -ExecutionPolicy Bypass -File runtime\tests\sync-vpncheap-subscription.Tests.ps1
powershell -ExecutionPolicy Bypass -File runtime\tests\proxy-chain.Tests.ps1
powershell -ExecutionPolicy Bypass -File runtime\tests\e2e.Tests.ps1
go test -race ./internal/config ./internal/subscription
```

macOS：

```bash
bash runtime/tests/sync-vpncheap-subscription.sh
bash runtime/tests/proxy-chain-macos.sh
go test -race ./internal/config ./internal/subscription
```

e2e 测试会使用临时端口启动本地 mock 上游，验证代理池与独立节点端口可用、9router 导出格式正确、重启后端口保持，以及代理池停止后 strict 客户端不会直连到目标。

## 最小配置示例（Pool）

```yaml
mode: pool

listener:
  address: 0.0.0.0
  port: 2323
  username: user
  password: pass

pool:
  mode: sequential    # sequential / random / balance / latency
  failure_threshold: 3
  blacklist_duration: 24h
  retry_enabled: true # 拨号失败时切换到另一节点重试
  retry_attempts: 3   # 每个请求的最大拨号次数

management:
  enabled: true
  listen: 0.0.0.0:9091
  probe_target: http://cp.cloudflare.com/generate_204
  password: ""

dns:
  server: 223.5.5.5
  port: 53
  strategy: prefer_ipv4

nodes_file: nodes.txt
```

## 粘性代理（可选，仅 Pool/Hybrid 模式）

开启后会额外监听一个独立端口（默认 `listener.port + 1`，即 `2324`），与原 `2323` 端口共存。通过粘性端口接入的客户端会按**来源 IP** 固定绑定到同一个上游节点，保持出口 IP 稳定（避免轮询导致 IP 频繁跳变触发风控/掉登录态）。绑定为永久保持，仅当该节点被拉黑/移除时才重新选择。监听地址与认证复用 `listener` 配置。

```yaml
sticky:
  enabled: true
  port: 2324    # 留空或 0 则默认为 listener.port + 1
```

## DNS 配置说明

`dns` 会同时影响 sing-box DNS 客户端和 VMess 域名拨号解析：

```yaml
dns:
  server: 223.5.5.5
  fallback_servers:    # 备用 DNS 服务器（主 DNS 解析失败时使用）
    - 8.8.8.8
    - 1.1.1.1
  port: 53
  strategy: prefer_ipv4
```

`strategy` 可选值：

- `as_is`
- `prefer_ipv4`
- `prefer_ipv6`
- `ipv4_only`
- `ipv6_only`

如果日志中出现 `lookup <domain>: empty result`，请优先检查该 DNS 配置是否可达且策略合理。

## 运行模式

- `pool`：所有节点共享一个本地 HTTP/SOCKS5 入口。
- `multi-port`：每个节点一个独立本地 HTTP/SOCKS5 端口。
- `hybrid`：同时启用 pool + multi-port。

## 节点来源行为

- 配置了 `subscriptions` 时：
  - 会抓取订阅节点并追加到运行节点列表
  - `nodes_file` 作为订阅节点写入路径
  - 启动阶段不再从 `nodes_file` 读取节点
  - 可通过 `subscription_refresh.fetch_concurrency` 调整订阅抓取并发数（默认 16，最大 32）
- `nodes`（内联节点）只要存在就会参与运行。
- **多来源节点合并**：当同时配置 `nodes` 和 `subscriptions` 时：
  - 内联节点（config.yaml 中的 `nodes`）和订阅节点会合并使用
  - 订阅更新时会保留内联节点，不会覆盖
  - 节点顺序：内联节点在前，订阅节点在后
  - 各节点的来源标识（inline/subscription）会在管理界面中显示
- **端口稳定**（multi-port/hybrid）：节点按 URI 稳定标识（忽略名称与参数顺序），订阅改名或重排都保持同一本地端口；分配结果保存到 config.yaml 同目录的 `node_ports.json`，重启后自动恢复。外部进程占用已发布端口时启动/重载会报错，不会偷偷改端口；请释放冲突端口后重试。删除该文件可强制重新分配。

## 协议支持注意事项

运行时真正支持的协议：

- `vmess`
- `vless`
- `trojan`
- `ss` / `shadowsocks`
  - 支持 SIP002：`ss://base64(method:password)@server:port#name`
  - 支持旧格式：`ss://base64(method:password@server:port)#name`
- `hysteria2` / `hy2`
- `socks5` / `socks5h` / `socks`
- `http` / `https`
- `anytls`
- `tuic`

订阅解析阶段可能识别到更多 URI 前缀（兼容输入），但不在上述列表中的协议会在构建阶段被跳过。

## 管理 API（核心）

- `POST /api/auth`
- `GET|PUT /api/settings`
- `GET /api/nodes`
- `POST /api/nodes/{tag}/probe`
- `POST /api/nodes/{tag}/release`
- `POST /api/nodes/{tag}/blacklist`
- `POST /api/nodes/probe-all`（SSE）
- `GET /api/export`
- `GET /api/export?target=9router`（9router 批量导入格式，全量节点、每行一个 HTTP 地址）
- `GET|PUT /api/subscription/config`
- `GET|POST /api/subscription/status|refresh`
- `GET|POST|PUT|DELETE /api/nodes/config[...]`
- `POST /api/reload`

`management.password` 为空时，Web/API 不要求登录。

## 重要运行说明

- 重载（`/api/reload` 或订阅刷新）会中断现有连接。
- Settings API 会把配置写回 `config.yaml`；部分设置需要重载后才能完全生效。
- 省略项默认值可在 `internal/config/config.go` 中查看。
- 日志轮转通过 `log` 配置段设置；当 `output: file` 时，日志同时写入控制台和文件，并自动轮转。

## 常见问题

### 配置持久化问题

**问题描述**：在 Docker 环境中通过 WebUI 修改配置后，重启或重建容器时配置被重置。

**快速诊断**：
```bash
# 检查 data 目录结构和权限
ls -la data/
[ -f data/config.yaml ]  || echo "缺少 data/config.yaml"
[ -d data/config.yaml ]   && echo "异常：data/config.yaml 是目录（见快速开始说明）"
[ -d data/nodes.txt ]    && echo "异常：data/nodes.txt 是目录（见快速开始说明）"
```

**常见原因和解决方案**：

1. **文件权限问题**：
   ```bash
   # 修复权限
   chown -R $(id -u):$(id -g) data
   chmod 755 data
   chmod 644 data/config.yaml data/nodes.txt
   ```

2. **卷映射错误**：
   - 确保 `docker-compose.yml` 中使用 `./data:/etc/easy_proxies`
   - 不要使用绝对路径或错误的目录

3. **启动时未传递 UID/GID**：
   ```bash
   # 正确的启动方式
   UID=$(id -u) GID=$(id -g) docker-compose up -d
   ```

**验证配置是否保存**：
```bash
# 查看文件修改时间
ls -lh data/config.yaml data/nodes.txt

# 查看容器日志，确认保存成功
docker-compose logs -f | grep "Saved"
```

**详细故障排查**：参见 [docs/troubleshooting-persistence.md](docs/troubleshooting-persistence.md)

### Docker 权限问题

**问题描述**：使用 `docker-compose.yml` 映射配置目录时，可能遇到 "permission denied" 或 "cannot write to /etc/easy_proxies" 等权限错误。

**原因分析**：容器以非 root 用户运行（docker-compose.yml 中指定 `user: "${UID:-10001}:${GID:-10001}"`），但宿主机挂载目录的所有权可能不匹配。

**解决方案**：

1. **使用 docker compose 挂载目录（推荐）**：
   ```bash
   mkdir -p data logs
   sudo chown -R $(id -u):$(id -g) data logs
   docker compose up -d
   ```
   直接挂载整个 `./data` 目录，首启动会自动生成 `config.yaml` 和 `nodes.txt` 文件。

2. **预先创建配置文件**（备选，适用于文件级挂载）：
   ```bash
   mkdir -p data
   cp config.example.yaml data/config.yaml
   touch data/nodes.txt
   chown -R $(id -u):$(id -g) data
   docker compose up -d
   ```

**docker run 命令方式**：
```bash
mkdir -p data logs
chmod -R u+w data logs
docker run --user $(id -u):$(id -g) \
  -v $(pwd)/data:/etc/easy_proxies \
  -v $(pwd)/logs:/app/logs \
  --network host \
  ghcr.io/jasonwong1991/easy_proxies:latest
```

### 其他常见问题

- **"配置文件未找到"**：确保挂载目录中存在 `config.yaml` 文件
- **"无法绑定端口"**：检查端口是否被其他服务占用
- **"所有节点健康检查失败"**：验证代理 URI 格式正确，且上游服务器可达
- **"代理之前正常使用，突然失效"**：检查节点是否被加入黑名单（连续失败 3 次后触发，默认持续 24 小时）
  - **解决方案 1**：通过 WebUI 释放 - 点击节点旁边的"释放"按钮
  - **解决方案 2**：通过 API 释放 - `POST http://localhost:9091/api/nodes/{tag}/release`
  - **解决方案 3**：在 `config.yaml` 中降低黑名单持续时间：
    ```yaml
    pool:
      blacklist_duration: 1h  # 从默认的 24h 改为 1h
    ```
  - 查看黑名单事件日志：`docker compose logs | grep "BLACKLISTED"`

## 更新日志

详见 [CHANGELOG.md](CHANGELOG.md)。

## 开发验证

```bash
go test ./...
```

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=jasonwong1991/easy_proxies&type=Date)](https://star-history.com/#jasonwong1991/easy_proxies&Date)

## 致谢

本项目基于 [sing-box](https://github.com/SagerNet/sing-box) 构建 —— 底层所有协议实现、传输层与拨号逻辑都由 sing-box 提供。特别感谢 SagerNet 团队及所有贡献者的卓越工作。

## 许可证

MIT License
