# 外网访问方案：Oracle Cloud 永久免费 VPS

## 目标

将 My GeekTime 部署到云端，实现不依赖个人电脑、通过手机浏览器随时访问课程内容。

## 方案选择

### 为什么不选其他方案

| 方案 | 淘汰原因 |
|------|---------|
| Fly.io | 手机访问需翻墙，`.fly.dev` 域名国内被墙 |
| Render / Railway | 免费实例需信用卡，`.onrender.com` 国内被墙 |
| 阿里云/腾讯云 | 免费试用需付费验证，续费不便宜 |
| Google Cloud / AWS | 需信用卡，国内被墙 |

### 为什么选 Oracle Cloud

- **永久免费层**：Always Free ARM VM（4核 + 24GB 内存），永不过期
- **无需信用卡**：仅需手机号验证
- **国内可用**：有中国区域，直连 IP，手机浏览器直接访问
- **无需翻墙**：部署后手机正常访问

## 部署架构

```
┌─────────────────────────────────────────┐
│         Oracle Cloud ARM VM              │
│  ┌─────────────────────────────────┐    │
│  │  my-geektime.exe (Go 二进制)    │    │
│  │  ┌─────────┐  ┌─────────────┐  │    │
│  │  │ config   │  │ data.db     │  │    │
│  │  │ .yml     │  │ (SQLite)    │  │    │
│  │  └─────────┘  └─────────────┘  │    │
│  │        ↓ Gin :8090              │    │
│  │    前端静态资源 (embed)           │    │
│  └─────────────────────────────────┘    │
│        公网 IP: <你的IP>:8090            │
└─────────────────────────────────────────┘
          ↑
    手机浏览器直接访问
```

**核心优势**：整个应用是一个 Go 二进制文件 + 一个 SQLite 文件，不需要额外部署数据库或其他服务。

## 数据库方案：MySQL → SQLite

### 为什么切 SQLite

- 课程数据量不大，SQLite 完全够用
- 不需要单独部署和运维数据库
- 备份就是复制一个文件
- 迁移到云端时只需传一个文件

### 数据迁移方式

| 方式 | 操作 | 适合场景 |
|------|------|---------|
| 脚本导出 | 写 Go 脚本读 MySQL 写 SQLite | 已有数据需要保留 |
| 重新缓存 | 在云端重新调用极客时间 API | 有 API 权限，数据不多 |

## 实施步骤

### 第一步：本地切换 SQLite

1. 修改 `config.yml` 的 database 配置（driver 改为 `sqlite`）
2. 导出 MySQL 数据到 SQLite（或重新缓存）
3. 本地验证应用能正常启动和访问

### 第二步：编译含前端的二进制

```bash
make web    # 构建前端到 web/ 目录
make build  # 编译 Go 二进制（包含前端静态资源）
```

输出：`my-geektime.exe`（Windows）或 `my-geektime`（Linux）

### 第三步：注册 Oracle Cloud

1. 访问 https://www.oracle.com/cloud/free/
2. 用 +86 手机号注册（需翻墙操作注册页面）
3. 创建 Always Free VM：
   - 选择 **Ampere (ARM)** 架构
   - 选择 **Canonical Ubuntu** 镜像
   - 4 核 OCPU + 24GB 内存

### 第四步：配置 VM 并部署

```bash
# SSH 连接到 VM
ssh ubuntu@<你的公网IP>

# 上传文件（二进制 + config.yml + SQLite 数据库）
# 方法1: scp
scp my-geektime ubuntu@<IP>:/home/ubuntu/
scp config.yml ubuntu@<IP>:/home/ubuntu/
scp data.db ubuntu@<IP>:/home/ubuntu/

# 方法2: 或用 curl 下载

# 运行
cd /home/ubuntu
./my-geektime server --config=config.yml
```

### 第五步：防火墙开放端口

```bash
# Oracle Cloud 控制台 → 网络 → 虚拟网卡 → 添加入站规则
# 协议: TCP, 端口: 8090, 来源: 0.0.0.0/0
```

### 第六步：手机访问

浏览器打开 `http://<公网IP>:8090`

## 成本估算

| 项目 | 费用 |
|------|------|
| Oracle Cloud VM | 免费 |
| 流量（160GB/月） | 免费 |
| 数据库 | 免费（SQLite 本地文件） |
| **总计** | **¥0** |

## 注意事项

1. **课程缓存数据需重新获取**：如果之前用 MySQL 存的课程数据，需要导出或在云端重新缓存
2. **VM 不会自动休眠**：Always Free VM 永久运行，但需要偶尔登录控制台确认账号活跃
3. **手机访问需流量**：看视频课程消耗流量，建议 WiFi 环境下使用
4. **公网 IP 是固定的**：每次重启 VM 后 IP 不变（Oracle Always Free 提供固定 IP）

## 后续优化

- [ ] 配置 systemd 服务，VM 重启后自动运行
- [ ] 配置 HTTPS（Let's Encrypt 免费证书）
- [ ] 添加访问密码保护（Cloudflare Access 或应用层鉴权）
- [ ] 设置自动备份 SQLite 数据库
