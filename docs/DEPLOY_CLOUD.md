# 云端部署方案

## 目标

将 My GeekTime 部署到云端，实现 24 小时在线访问，无需额外购买服务器。

---

## 方案选择：Tailscale Funnel + 本地运行

### 为什么选这个方案

| 条件 | Tailscale Funnel | 廉价 VPS |
|------|-----------------|---------|
| 费用 | **0 元** | ¥15-38/月 |
| 改代码吗 | **不用改** | 不用改 |
| 需要公网 IP | **不需要** | 需要 |
| 注册门槛 | **Google 账号即可** | 需要身份证实名 |
| 数据安全 | **完全在自己手里** | 在云厂商 |
| 电费 | ~¥0.5/月（屏幕关闭） | ¥15-38/月 |

### 核心原理

```
手机/电脑 ───HTTPS──▶ Tailscale 边缘节点 ──内网穿透──▶ 你的笔记本
```

Tailscale 提供零配置的内网穿透，开启 Funnel 后会自动获得一个 `https://xxx.tailxxxxx.ts.net` 的 HTTPS 域名，通过 Tailscale 的边缘节点安全地转发到你家设备。

### 核心组件

| 组件 | 选型 | 费用 | 说明 |
|------|------|------|------|
| 应用运行 | 你的笔记本 | 0 元 | 屏幕关闭状态，每月电费约 ¥0.5 |
| 内网穿透 | Tailscale Funnel | 免费 | 个人使用免费，提供 HTTPS 域名 |
| 数据库 | 本地 MySQL（不变） | 0 元 | 无需迁移，直接使用现有数据 |
| 域名 | Tailscale 自动分配 | 免费 | `xxx.tailxxxxx.ts.net`，自带 HTTPS |

---

## 部署步骤

### 第一步：注册 Tailscale 账号

1. 访问 https://tailscale.com/download
2. 下载对应操作系统版本（Windows / macOS / Linux / Android / iOS）
3. 使用你的 **Google 邮箱** 注册并登录
4. 登录成功后，设备状态显示为"Online"

### 第二步：开启 Funnel

**Windows / macOS / Linux：**

```bash
# 在终端执行
tailscale funnel 8090
```

这会暴露你笔记本上的 8090 端口（你的应用运行端口），并分配一个 HTTPS 域名。

**命令行会输出类似：**

```
tcp://mydevice.tail12345.ts.net:8090
```

然后提供一个公共 URL 供访问。

**Android / iOS：**

1. 打开 Tailscale App
2. 点击你的设备
3. 找到"Funnel"开关并开启
4. 端口设置为 8090

### 第三步：配置应用

你的应用**不需要任何代码改动**。只需确保：

- 应用监听 `0.0.0.0:8090`（`config.yml` 中已配置 `http_addr: 0.0.0.0`）
- 数据库连接使用本地 MySQL（`config.yml` 中保持 `driver: sqlite` 或 `mysql` 不变）

启动应用：

```bash
# Windows
my-geektime server --config=config.yml

# 或使用 docker
docker run -d \
  --name my-geektime \
  -p 8090:8090 \
  -v $(pwd)/config.yml:/config.yml \
  -v $(pwd)/repo:/repo \
  zkep/mygeektime:latest
```

### 第四步：配置笔记本节能设置

**屏幕关闭（推荐）：**

```
控制面板 → 电源选项 → 选择关闭显示器的时间 → 设为 1 分钟
控制面板 → 选择笔记本盖子的功能 → 合上盖子时"不采取任何操作"
```

**省电模式：**

```
Windows 电源计划 → 选"节能"模式
```

**月度电费估算：**

| 状态 | 功耗 | 月电费 |
|------|------|--------|
| 屏幕关闭 + 节能模式 | ~10W | ~¥0.3 |
| 正常下载时 | ~30W | ~¥1.0 |

### 第五步：配置 MySQL（如果使用）

如果你本地用的是 MySQL：

```yaml
# config.yml
database:
  driver: mysql
  source: root:123456@tcp(127.0.0.1:3306)/mygeektime?charset=utf8&parseTime=True&loc=Local&timeout=1000ms
```

如果你用的是 SQLite（默认）：

```yaml
# config.yml
database:
  driver: sqlite
  source: mygeektime.db
```

### 第六步：配置存储（下载文件的存储位置）

```yaml
# config.yml
storage:
  driver: local
  directory: repo      # 下载的文件存储路径
  bucket: object
  host: http://127.0.0.1:8090   # 本地访问地址
```

> **注意**：如果使用 Tailscale Funnel 远程访问，`host` 需要改为你的 Tailscale 域名：
> ```yaml
> host: https://mydevice.tailxxxxx.ts.net
> ```

### 第七步：启动并测试

1. 启动应用
2. 确认 Tailscale Funnel 正在运行（终端中能看到公共 URL）
3. 在手机或其他设备上通过分配的 HTTPS 域名访问
4. 确认可以正常登录和使用

### 第八步：设置开机自启（可选）

**Windows：**

1. 按 `Win + R` 输入 `shell:startup` 打开启动文件夹
2. 创建一个快捷方式指向你的应用启动命令

**或创建 Windows 服务：**

```powershell
# 以管理员身份运行 PowerShell
New-Service -Name "MyGeekTime" -BinaryPathName "C:\path\to\my-geektime.exe" -StartupType Automatic
Start-Service MyGeekTime
```

**Linux：**

```bash
# 创建 systemd 服务
sudo tee /etc/systemd/system/my-geektime.service > /dev/null <<EOF
[Unit]
Description=My GeekTime Server
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/app
ExecStart=/path/to/my-geektime server --config=config.yml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable my-geektime
sudo systemctl start my-geektime
```

---

## 数据持久化说明

| 数据类型 | 存储位置 | 持久化策略 |
|---------|---------|-----------|
| 数据库 | 本地磁盘 | ✅ SQLite 文件 / MySQL 数据目录 |
| 下载的文件 | 本地 `repo/` 目录 | ✅ |
| 缓存文件 | 应用内存 | ❌ 重启丢失（正常） |

> **建议**：定期备份 `mygeektime.db` 或 `repo/` 目录到云存储，防止硬件故障。

---

## 注意事项

### 笔记本长期运行

- **屏幕关闭状态**下，功耗约 10-20W，每月电费约 ¥0.3-0.6
- 建议拔掉所有 USB 外设减少功耗
- 笔记本电池在插电状态下通常不会放电（电源直供）
- 如果担心电池鼓包，可以启用电池养护模式（联想有 Lenovo Vantage 软件）

### 网络要求

- 笔记本需要保持网络连接
- 家庭宽带上行速度决定了访问流畅度
- Tailscale 使用 DERP 中继服务器，全球都有节点

### 安全

- Tailscale 域名是加密的 HTTPS 访问
- 应用本身有 JWT 鉴权机制
- 不建议将 Tailscale 账号分享给他人
- 建议定期修改 `jwt.secret`

### 带宽和性能

- Tailscale Funnel 的出口带宽取决于 Tailscale 网络状态
- 一般文字浏览和看课程完全够用
- 大量下载时可能受家庭宽带上行限制

---

## 备选方案

### 方案 B：Tailscale Funnel + 旧手机运行

- 在旧手机上装 Termux（Android）
- Termux 中编译运行 Go 应用
- 手机插电作为服务器
- 功耗比笔记本更低（约 5W）

### 方案 C：租最便宜 VPS

- 腾讯云/阿里云学生机约 ¥15-38/月
- 用 MySQL 不用迁移
- Docker Compose 一键部署

---

## 常见问题

### Q: Tailscale Funnel 免费吗？

A: 是的，个人使用完全免费。Tailscale 付费版仅支持更高级功能（如 SSO、SAML），个人使用不需要。

### Q: Tailscale 域名会不会变？

A: 不会。只要你的设备名不变，`xxx.tailxxxxx.ts.net` 域名是固定的。

### Q: 手机/电脑怎么访问？

A: 在手机浏览器或电脑浏览器中直接输入 Tailscale 分配的 HTTPS 域名即可。

### Q: 下载速度受什么限制？

A: 取决于你家宽带的上行带宽。一般家庭宽带上行 20-100Mbps，下载视频时速度会受此限制。

### Q: 笔记本一直开着会坏吗？

A: 现代笔记本设计时就考虑长期运行。关闭屏幕、保持通风、控制温度，正常使用 3-5 年没问题。

---

## 执行清单

- [x] 注册 Tailscale 账号（GitHub）
- [x] 在笔记本上安装 Tailscale 并登录
- [x] 开启 Funnel 暴露 8090 端口
- [x] 启动 My GeekTime 应用
- [x] 修改 storage.host 为 Tailscale 域名
- [x] 在手机上测试访问
- [ ] 配置笔记本省电设置
- [ ] 设置开机自启
- [ ] （可选）备份数据库和下载文件
