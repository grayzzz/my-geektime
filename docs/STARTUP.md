# My GeekTime 启动指南

## 前提条件

- Docker 已安装并运行
- Tailscale 已安装并登录（设备 `gray.tail5ab2a8.ts.net`）

## 启动 MySQL

```powershell
cd D:\develop\myWebDemo\online\my-geektime\docker
docker-compose -f docker-compose.mysql.yml up -d
```

## 一键启动

```bash
# 1. 确保 Tailscale 已连接，开启 Funnel
tailscale funnel 8090

# 2. 进入项目目录并启动应用
cd D:\develop\myWebDemo\online\my-geektime
./my-geektime server --config=config.yml
```

访问地址：Tailscale Funnel 输出的公共 HTTPS URL

## 重启（二进制已构建过，直接启动）

```bash
./my-geektime server --config=config.yml
```

## 全新编译构建（首次或前端有改动时）

```bash
cd D:\develop\myWebDemo\online\my-geektime
make build          # 前端构建 + 后端编译
./my-geektime server --config=config.yml
```

## 省电设置

```
控制面板 → 电源选项 → 选择关闭显示器的时间 → 设为 1 分钟
控制面板 → 选择笔记本盖子的功能 → 合上盖子时"不采取任何操作"
电源计划 → 选"节能"模式
```

屏幕关闭后整机功耗约 10W，月电费约 ¥0.3。

## 关键：保持网络不中断

电脑锁屏/休眠会导致网卡断连，手机无法访问服务。必须关闭以下设置：

1. **关闭锁屏**：`Win + L` 设置自动锁屏时间为「从不」
2. **关闭休眠**：控制面板 → 电源选项 → 更改计划设置 → 让电脑进入睡眠状态 → **从不**
3. **关闭快速启动**：控制面板 → 电源选项 → 选择电源按钮的功能 → 更改当前不可用的设置 → 取消勾选「启用快速启动」
4. **允许网卡唤醒**：设备管理器 → 网络适配器 → 右键网卡 → 属性 → 电源管理 → 勾选「允许此设备唤醒计算机」

> 建议将显示器设置自动关闭（1-5 分钟），屏幕关闭但电脑保持运行，功耗最低且网络不中断。

| 配置项 | 值 | 说明 |
|--------|------|------|
| 端口 | 8090 | `server.http_port` |
| 数据库 | MySQL 3326 | `database.source` |
| 存储 | local / repo/ | 下载文件存放目录 |
| 远程域名 | gray.tail5ab2a8.ts.net | `storage.host` |

## 注意

我的go 安装在 D:\develop\go\go1.25
