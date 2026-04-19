# Local SMTP Relay to Microsoft Graph (Go CLI)

这是一个**纯命令行 Go 项目**，用于把旧系统发往本地 SMTP 的邮件，转发到 Microsoft Graph `POST /me/sendMail`。

- 不包含前端页面
- 不包含 Web 登录页
- 仅绑定 1 个 Microsoft 365 工作/学校账号
- 登录采用 **Device Code Flow + Delegated Token**（兼容 MFA）
- 不要求输入 Office 账号密码
- 不使用 ROPC
- 不伪造 Microsoft 官方 SMTP 凭据

## “生成 SMTP 信息”是什么意思？

`bind` 成功后会生成并显示的是**本地 relay 服务的连接信息**，例如：

- host: `127.0.0.1`
- port: `2525`
- username: 随机生成
- password: 随机生成（仅显示一次）
- tls: enabled（STARTTLS）

这些凭据**只用于连接你本机上的这个 Go SMTP relay**。真正出站发信仍由程序调用 Microsoft Graph `/me/sendMail` 完成。

## 核心能力

- 单账号绑定：第一次绑定后即锁定该账号
- SMTP 协议支持：EHLO/HELO、STARTTLS、AUTH PLAIN、AUTH LOGIN、MAIL FROM、RCPT TO、DATA、QUIT
- 默认要求 TLS 后才允许 AUTH（未启用 TLS 拒绝认证）
- 支持按 SMTP `MAIL FROM` 请求发件；若绑定账号有该地址 SendAs 权限则按该地址发信，否则拒绝并返回权限错误
- SMTP 登录凭据为本地随机生成，磁盘只保存 bcrypt 哈希
- Token cache / 绑定配置 / 元数据持久化到挂载目录（默认 `/data`）
- Graph 错误按 401/403/429/5xx 分类映射并日志记录

## 项目结构

```text
.
├── cmd/
├── internal/
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── main.go
└── README.md
```

## 1) Microsoft Entra 应用注册

1. 进入 Microsoft Entra 管理中心 → **App registrations** → **New registration**。
2. 选择仅组织内账号（Single tenant 或按你租户策略）。
3. 创建后记录：
   - `Application (client) ID` → 对应 `CLIENT_ID`
   - `Directory (tenant) ID` → 对应 `TENANT_ID`
4. 在 **Authentication** 中启用 **Allow public client flows**（或等价项）。
5. 在 **API permissions** 中添加 Delegated 权限：
   - `User.Read`
   - `Mail.Send`
   - `Mail.Send.Shared`（当需要按非本人地址发件时）
   - `offline_access`
6. 根据租户策略决定是否执行管理员同意（Admin consent）。

## 2) 为什么用 public client + device code flow

- 本项目是 CLI 程序，不走 Web 回调页。
- Device Code Flow 在终端给出登录码，用户在浏览器完成 Microsoft 官方登录。
- 支持 MFA、条件访问等现代认证能力。
- 全程不在程序中输入 Office 账号密码。

## 3) 环境配置

```bash
cp .env.example .env
mkdir -p data certs
```

编辑 `.env`，至少填写：

- `TENANT_ID`
- `CLIENT_ID`
- `TLS_CERT_FILE`
- `TLS_KEY_FILE`

默认值：

- `SMTP_BIND_ADDR=0.0.0.0:2525`（容器内）
- `docker-compose` 端口映射为 `127.0.0.1:2525:2525`（宿主机仅本地访问）

## 4) 生成本地自签名证书（开发测试）

在项目目录执行：

```bash
openssl req -x509 -newkey rsa:2048 -sha256 -days 365 -nodes \
  -keyout certs/server.key -out certs/server.crt \
  -subj "/CN=localhost"
chmod 600 certs/server.key certs/server.crt
```

> 生产环境请使用受信任证书并妥善保护私钥。

## 5) 构建镜像

```bash
docker-compose build app
```

说明：只有 `app` 服务负责构建镜像，`relay` 直接复用同一个 `localrelay:latest` 镜像，避免两个服务重复构建/标签冲突。

若你的环境无法访问 `proxy.golang.org`（中国大陆常见），本项目已默认在构建阶段使用：

- `GOPROXY=https://goproxy.cn,direct`
- `GOSUMDB=sum.golang.google.cn`

你也可以在 `.env` 里覆盖这两个值。

## 6) 执行 bind（Device Code 登录）

```bash
docker-compose run --rm app bind
```

执行后终端会打印类似提示：

- 打开 `https://microsoft.com/devicelogin`
- 输入设备码
- 在浏览器完成 Microsoft 登录与 MFA

成功后会输出：

- Bound account
- SMTP host / port / username
- SMTP password（只显示一次）
- TLS 模式
- From address

## 7) 查看本地 SMTP 信息

```bash
docker-compose run --rm app smtp-info
```

> `smtp-info` 默认不会显示明文密码。

## 8) 启动长期运行 relay

```bash
docker-compose up -d relay
```

查看状态：

```bash
docker-compose ps
docker-compose logs -f relay
```

## 9) 旧系统如何连接本地 SMTP relay

请让旧系统使用如下参数（示例）：

- Host: `127.0.0.1`
- Port: `2525`
- TLS: `STARTTLS`
- Auth: `PLAIN` 或 `LOGIN`
- Username/Password: 来自 `bind` 或 `rotate-password`

注意：

- 默认必须先 `STARTTLS`，否则 AUTH 会被拒绝。
- 若确实无法信任证书且仅用于内网，可设置 `ALLOW_INSECURE_AUTH=true` 放宽为明文 AUTH（不安全，不建议生产使用）。
- SMTP `MAIL FROM` 与绑定账号不同也可尝试发送，但仅当绑定账号具备该地址 SendAs 权限时才成功；否则返回权限错误（`401 permission denied`）。

## 10) 用 test-send 验证 Graph 发信

```bash
docker-compose run --rm app test-send --to xxx@example.com --subject test --body hello
```

该命令**不经过 SMTP**，直接调用 Graph `/me/sendMail`。

## 11) 常用命令

```bash
# 查看状态
docker-compose run --rm app status

# 显示 SMTP 摘要
docker-compose run --rm app smtp-info

# 轮换本地 SMTP 密码（仅显示一次）
docker-compose run --rm app rotate-password

# 解绑（清理绑定、token cache、本地 SMTP 凭据）
docker-compose run --rm app unbind

# 查看构建版本信息
docker-compose run --rm app version
```

## 12) 日志、停止与清理

查看 relay 日志：

```bash
docker-compose logs -f relay
```

停止服务：

```bash
docker-compose down
```

连同容器网络一起清理（保留 `./data` 和 `./certs` 目录）：

```bash
docker-compose down --remove-orphans
```

如果要彻底重置绑定状态：

```bash
docker-compose run --rm app unbind
```

## 13) 常见错误排查

1. 用户取消登录
   - `bind` 会返回 device code 登录失败。
   - 重新执行 `docker-compose run --rm app bind`。

2. 浏览器登录了错误账号
   - 如果已绑定账号与当前登录账号不一致，会被拒绝。
   - 先 `unbind`，确认浏览器账号后重新 `bind`。

3. `Mail.Send` / `Mail.Send.Shared` 未授权
   - 检查 Entra 应用 API 权限是否为 Delegated，并已同意授权。
   - 必要时执行管理员同意。

4. token 刷新失败 / 需要重新认证
   - 先检查租户策略、账号授权是否被撤销。
   - 执行 `unbind` + `bind` 重新登录。

5. Graph 403
   - 常见原因：权限不足、策略阻止、邮箱不可用。
   - 检查日志中的 HTTP status / graph error code / request-id。

6. Graph 429
   - 表示限流。程序会做有限重试退避。
   - 稍后重试并降低发送峰值。

7. 已绑定其他账号
   - 项目只允许绑定 1 个账号。
   - 必须先 `unbind` 再 `bind`。

8. 本地 SMTP 用户名/密码错误
   - 使用 `smtp-info` 确认用户名。
   - 如遗忘密码，执行 `rotate-password`。

9. TLS 证书加载失败
   - 检查 `TLS_CERT_FILE`、`TLS_KEY_FILE` 路径和权限。
   - 文件缺失或证书/私钥不匹配会导致 `serve` 启动失败。

10. 客户端未启用 STARTTLS 导致 AUTH 被拒绝
   - 确保 SMTP 客户端启用了 STARTTLS。
   - 本服务默认要求 TLS 后才允许 AUTH。

## 安全说明

- 不记录 access token / refresh token / SMTP 明文密码。
- SMTP 密码仅在创建或轮换时明文显示一次。
- 绑定文件和 token cache 持久化到 `/data`（建议权限收紧）。
- 若 `/data` 无写权限，程序会自动回退到临时目录（通常 `/tmp/relayctl-data`）并给出警告，避免直接报错退出。
- 镜像运行用户为非 root。

## 配置优先级

程序按以下优先级读取配置（高 → 低）：

1. 命令行参数
2. 环境变量
3. 配置文件（默认 `/data/config.yaml`）

支持的关键环境变量：

- `TENANT_ID`
- `CLIENT_ID`
- `SMTP_BIND_ADDR`
- `DATA_DIR`
- `LOG_LEVEL`
- `TLS_CERT_FILE`
- `TLS_KEY_FILE`
- `TLS_MIN_VERSION`
- `ENABLE_SMTPS`
- `SMTPS_BIND_ADDR`
- `ALLOW_HTML`
- `ALLOW_INSECURE_AUTH`

