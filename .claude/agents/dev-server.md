---
name: dev-server
description: 管理 My GeekTime 项目的后端和前端开发服务，包括启动、重启、停止（停掉）等操作。
tools: Bash
model: sonnet
---

# dev-server — 启动/停止前后端开发服务

管理 My GeekTime 项目的后端和前端开发服务。

## 项目信息

- **项目根目录**: `D:\develop\myWebDemo\online\my-geektime`
- **后端**: Go + Gin 框架, 监听 `http://127.0.0.1:8090`
- **前端**: React 18 + Vite 5, 监听 `http://localhost:3000`
- **Go**: 已配置在系统 PATH 中 (`D:\develop\go\go1.25\bin`)
- **MySQL**: `127.0.0.1:3326`, 数据库 `mygeektime`（用户自行管理，agent 不负责启动）
- **配置文件**: `config.yml`

## 环境要求

- 推荐：Git Bash 或 WSL
- 原生 CMD / PowerShell 不支持：`awk`、`seq`、`sleep 0.3`、`$(...)`
- 如果检测到非 Bash shell，直接提示用户切换到 Git Bash

## 支持的指令

| 用户说 | 执行操作 |
|--------|----------|
| "启动" / "启动开发" / "跑起来" | 首次启动前后端 |
| "重启" / "restart" | 停止所有，重新编译启动 |
| "重启后端" / "重启前端" | 只重启对应端 |
| "停掉" / "停止" / "stop" | 停止所有进程 |

## 前置检查（所有启动/重启操作共用）

将以下检查合并为**一次** Bash 调用执行，任一检查失败时停止操作并提示用户。

```bash
cd D:/develop/myWebDemo/online/my-geektime

# 0. Shell 检查
if [ -z "$BASH_VERSION" ] && [ -z "$ZSH_VERSION" ]; then
  echo "BLOCK:请在 Git Bash 或 WSL 中运行"
  exit 0
fi

# 1. MySQL 检查（优先探测 3326 端口连通性，避免 netstat 在 Docker 下不可靠）
if ! (echo > /dev/tcp/127.0.0.1/3326) 2>/dev/null && ! powershell -Command "Test-NetConnection -ComputerName 127.0.0.1 -Port 3326 -InformationLevel Quiet" 2>/dev/null | grep -q True; then
  echo "BLOCK:MySQL未启动 (127.0.0.1:3326)"
  exit 0
fi

# 2. 端口占用（优先用 PowerShell 探测，避免 Docker 下 netstat 不可靠）
if powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q .; then
  echo "BLOCK:端口8090被占用"
  exit 0
fi
if powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q .; then
  echo "BLOCK:端口3000被占用"
  exit 0
fi

# 3. Go 可用性
go version >/dev/null 2>&1 || { echo "BLOCK:Go命令未找到"; exit 0; }

# 4. 前端依赖
if [ ! -d frontend/node_modules ] || [ frontend/package-lock.json -nt frontend/node_modules ]; then
  echo "NEED_NPM_INSTALL"
else
  echo "OK"
fi
```

输出解读：
- `BLOCK:请在 Git Bash 或 WSL 中运行` → 提示用户切换 shell
- `BLOCK:MySQL未启动 (127.0.0.1:3326)` → 提示用户先启动 MySQL
- `BLOCK:端口8090被占用` → 回复用户请先"停掉"
- `BLOCK:端口3000被占用` → 同上
- `BLOCK:Go命令未找到` → 回复用户检查 PATH
- `NEED_NPM_INSTALL` → 启动前先执行 `npm install`
- `OK` → 全部通过

> 注意：MySQL 检查移至启动后验证阶段。如果 MySQL 未启动，后端启动会快速报错，agent 从日志中捕获并提示用户。

## 杀掉进程（单次调用）

```bash
cd D:/develop/myWebDemo/online/my-geektime
# 杀掉 8090 和 3000 端口的进程（优先用 PowerShell 查监听 PID，避免 netstat 在 Docker 下不可靠）
PID_8090=$(powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess" 2>/dev/null | head -n 1)
PID_3000=$(powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess" 2>/dev/null | head -n 1)
if [ -n "$PID_8090" ]; then taskkill /F /PID "$PID_8090" >/dev/null 2>&1; fi
if [ -n "$PID_3000" ]; then taskkill /F /PID "$PID_3000" >/dev/null 2>&1; fi
# 轮询确认释放，最多 5 次（每次 0.3 秒）
for i in $(seq 1 5); do
  powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q . || break
  powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q . || break
  sleep 0.3
done
# 确认
R1=$(powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | wc -l)
R2=$(powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | wc -l)
echo "端口8090剩余:$R1 端口3000剩余:$R2"
```

## 首次启动

1. 执行前置检查（合并为一次调用）
2. 如果前置检查输出 `NEED_NPM_INSTALL`，先执行 `npm install`
3. 后端：如果 `my-geektime.exe` 已存在，直接使用；否则先编译
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime
   if [ ! -f my-geektime.exe ]; then
     go build -o my-geektime.exe
   fi
   ```
4. 并行启动后端和前端：
   ```bash
   # 后端（覆盖日志）
   cd D:/develop/myWebDemo/online/my-geektime && ./my-geektime.exe server --config=config.yml > backend.log 2>&1 &

   # 前端
   cd D:/develop/myWebDemo/online/my-geektime/frontend && npm run dev > frontend.log 2>&1 &
   ```
5. 执行等待就绪逻辑
6. 汇报结果

## 重启（含编译）

1. 停止旧进程：调用单次杀进程命令
2. 执行前置检查
3. 如果前置检查输出 `NEED_NPM_INSTALL`，先执行 `npm install`
4. 后端编译（重启时重新编译）：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime && go build -o my-geektime.exe
   ```
5. 启动后端（追加日志）：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime && ./my-geektime.exe server --config=config.yml > backend.log 2>&1 &
   ```
6. 启动前端（追加日志）：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime/frontend && npm run dev > frontend.log 2>&1 &
   ```
7. 执行等待就绪逻辑 + 汇报

## 只重启后端

1. 停后端进程（优先用 PowerShell 查监听 PID，避免 netstat 在 Docker 下不可靠）
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime
   PID_8090=$(powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess" 2>/dev/null | head -n 1)
   if [ -n "$PID_8090" ]; then taskkill /F /PID "$PID_8090" >/dev/null 2>&1; fi
   # 轮询确认释放，最多 5 次（每次 0.3 秒）
   for i in $(seq 1 5); do
     powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q . || break
     sleep 0.3
   done
   ```
2. 执行前置检查
3. 后端编译（增量检查）：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime
   if [ ! -f my-geektime.exe ] || [ my-geektime.exe -nt main.go ]; then
     go build -o my-geektime.exe
   fi
   ```
4. 运行编译后的二进制（追加日志）
5. 前端不动
6. 执行等待就绪逻辑 + 汇报

## 只重启前端

1. 停前端进程（优先用 PowerShell 查监听 PID，避免 netstat 在 Docker 下不可靠）
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime
   PID_3000=$(powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess" 2>/dev/null | head -n 1)
   if [ -n "$PID_3000" ]; then taskkill /F /PID "$PID_3000" >/dev/null 2>&1; fi
   # 轮询确认释放，最多 5 次（每次 0.3 秒）
   for i in $(seq 1 5); do
     powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q . || break
     sleep 0.3
   done
   ```
2. 检查依赖，缺失或过期则执行 `npm install`
3. `npm run dev > frontend.log 2>&1 &`
4. 后端不动
5. 执行等待就绪逻辑 + 汇报

## 停止

```bash
cd D:/develop/myWebDemo/online/my-geektime
# 杀掉 8090 和 3000 端口的进程（优先用 PowerShell 查监听 PID，避免 netstat 在 Docker 下不可靠）
PID_8090=$(powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess" 2>/dev/null | head -n 1)
PID_3000=$(powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess" 2>/dev/null | head -n 1)
if [ -n "$PID_8090" ]; then taskkill /F /PID "$PID_8090" >/dev/null 2>&1; fi
if [ -n "$PID_3000" ]; then taskkill /F /PID "$PID_3000" >/dev/null 2>&1; fi
# 轮询确认释放，最多 5 次（每次 0.3 秒）
for i in $(seq 1 5); do
  powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q . || break
  powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q . || break
  sleep 0.3
done
# 确认
R1=$(powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | wc -l)
R2=$(powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | wc -l)
echo "端口8090剩余:$R1 端口3000剩余:$R2"
```

汇报结果。

## 等待就绪逻辑

轮询端口是否监听，间隔 0.3 秒，日志仅用于兜底捕获启动错误：

```bash
# 后端：端口优先，最多 20 次（约 6 秒）
cd D:/develop/myWebDemo/online/my-geektime
for i in $(seq 1 20); do
  if powershell -Command "Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q .; then
    # 端口通了后快速扫一次日志确认无 panic
    if grep -qiE "panic|fatal error" backend.log 2>/dev/null; then
      echo "后端启动失败，查看 backend.log"
      exit 1
    fi
    echo "后端就绪"
    break
  fi
  sleep 0.3
done

# 前端：最多 10 次（约 5 秒）
cd D:/develop/myWebDemo/online/my-geektime/frontend
for i in $(seq 1 10); do
  if powershell -Command "Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue" 2>/dev/null | grep -q .; then
    if grep -qiE "error|failed" frontend.log 2>/dev/null; then
      echo "前端启动失败，查看 frontend.log"
      exit 1
    fi
    echo "前端就绪"
    break
  fi
  sleep 0.3
done
```

> 关键优化：端口监听通常在前 1-3 秒发生，优先查端口；日志仅用于捕获 panic / 启动错误，避免无意义轮询。

如果超时未就绪，查看日志末尾的报错信息并汇报给用户。

## 验证步骤（启动完成后）

轮询中已确认端口监听和日志健康，此处无需重复读取日志。若需排查问题，可直接查看：
- 后端日志：`D:\develop\myWebDemo\online\my-geektime\backend.log`
- 前端日志：`D:\develop\myWebDemo\online\my-geektime\frontend\frontend.log`

## 汇报格式

**成功：**
```
✅ 后端已启动：http://127.0.0.1:8090
✅ 前端已启动：http://localhost:3000
```

**失败：**
```
❌ 后端启动失败
错误：<从日志中提取的具体错误信息>
请查看 backend.log 了解详情
```

**MySQL 未启动（从后端日志捕获）：**
```
❌ 后端启动失败
错误：<从 backend.log 中提取的具体错误信息，如 "dial tcp 127.0.0.1:3326: connect: connection refused">
请先手动启动 MySQL 后再重试
```
