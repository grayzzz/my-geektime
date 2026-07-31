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

# 1. 端口占用（Windows 兼容，使用 netstat 代替 lsof）
PORT_8090=$(netstat -ano 2>/dev/null | grep ':8090[[:space:]]' | grep LISTENING | awk '{print $5}')
PORT_3000=$(netstat -ano 2>/dev/null | grep ':3000[[:space:]]' | grep LISTENING | awk '{print $5}')
if [ -n "$PORT_8090" ]; then echo "BLOCK:端口8090被占用 PID:$PORT_8090"; exit 0; fi
if [ -n "$PORT_3000" ]; then echo "BLOCK:端口3000被占用 PID:$PORT_3000"; exit 0; fi

# 2. Go 可用性
go version >/dev/null 2>&1 || { echo "BLOCK:Go命令未找到"; exit 0; }

# 3. 前端依赖
if [ ! -d frontend/node_modules ]; then
  echo "NEED_NPM_INSTALL"
else
  echo "OK"
fi
```

输出解读：
- `BLOCK:端口8090被占用 PID:xxxx` → 回复用户请先"停掉"
- `BLOCK:端口3000被占用 PID:xxxx` → 同上
- `BLOCK:Go命令未找到` → 回复用户检查 PATH
- `NEED_NPM_INSTALL` → 启动前先执行 `npm install`
- `OK` → 全部通过

> 注意：MySQL 检查移至启动后验证阶段。如果 MySQL 未启动，后端启动会快速报错，agent 从日志中捕获并提示用户。

## 杀掉进程（单次调用）

```bash
cd D:/develop/myWebDemo/online/my-geektime
# 杀掉 8090 和 3000 端口的进程（Windows 兼容，使用 netstat + taskkill 代替 lsof）
PIDS_8090=$(netstat -ano 2>/dev/null | grep ':8090[[:space:]]' | grep LISTENING | awk '{print $5}' | tr '\n' ' ')
PIDS_3000=$(netstat -ano 2>/dev/null | grep ':3000[[:space:]]' | grep LISTENING | awk '{print $5}' | tr '\n' ' ')
if [ -n "$PIDS_8090" ]; then taskkill //F $PIDS_8090 2>/dev/null; fi
if [ -n "$PIDS_3000" ]; then taskkill //F $PIDS_3000 2>/dev/null; fi
sleep 1
# 确认释放
COUNT_8090=$(netstat -ano 2>/dev/null | grep ':8090[[:space:]]' | grep LISTENING | wc -l)
COUNT_3000=$(netstat -ano 2>/dev/null | grep ':3000[[:space:]]' | grep LISTENING | wc -l)
echo "$COUNT_8090 $COUNT_3000"
# 输出 "0 0" 表示全部释放
```

## 首次启动

1. 执行前置检查（合并为一次调用）
2. 如果前置检查输出 `NEED_NPM_INSTALL`，先执行 `npm install`
3. 后端编译：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime && go build -o my-geektime.exe
   ```
4. 并行启动后端和前端：
   ```bash
   # 后端（使用编译后的二进制，避免 go run 首次编译慢）
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
4. 后端编译：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime && go build -o my-geektime.exe
   ```
5. 启动后端（使用编译后的二进制）：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime && ./my-geektime.exe server --config=config.yml > backend.log 2>&1 &
   ```
6. 启动前端：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime/frontend && npm run dev > frontend.log 2>&1 &
   ```
7. 执行等待就绪逻辑 + 汇报

## 只重启后端

1. 停后端进程：`taskkill //F //PID $(netstat -ano | grep ':8090[[:space:]]' | grep LISTENING | awk '{print $5}') 2>/dev/null; sleep 1`
2. 执行前置检查
3. `go build -o my-geektime.exe`
4. 运行编译后的二进制
5. 前端不动
6. 执行等待就绪逻辑 + 汇报

## 只重启前端

1. 停前端进程：`taskkill //F //PID $(netstat -ano | grep ':3000[[:space:]]' | grep LISTENING | awk '{print $5}') 2>/dev/null; sleep 1`
2. 检查依赖，缺失则执行 `npm install`
3. `npm run dev`
4. 后端不动
5. 执行等待就绪逻辑 + 汇报

## 停止

```bash
cd D:/develop/myWebDemo/online/my-geektime
PIDS_8090=$(netstat -ano 2>/dev/null | grep ':8090[[:space:]]' | grep LISTENING | awk '{print $5}' | tr '\n' ' ')
PIDS_3000=$(netstat -ano 2>/dev/null | grep ':3000[[:space:]]' | grep LISTENING | awk '{print $5}' | tr '\n' ' ')
if [ -n "$PIDS_8090" ]; then taskkill //F $PIDS_8090 2>/dev/null; fi
if [ -n "$PIDS_3000" ]; then taskkill //F $PIDS_3000 2>/dev/null; fi
sleep 1
# 确认
P1=$(netstat -ano 2>/dev/null | grep ':8090[[:space:]]' | grep LISTENING | wc -l)
P2=$(netstat -ano 2>/dev/null | grep ':3000[[:space:]]' | grep LISTENING | wc -l)
echo "端口8090剩余:$P1 端口3000剩余:$P2"
```

汇报结果。

## 等待就绪逻辑

轮询日志内容，间隔 0.5 秒，同时做端口验证：

```bash
# 轮询后端日志，最多 15 次（约 7.5 秒）
cd D:/develop/myWebDemo/online/my-geektime
for i in $(seq 1 15); do
  if grep -qiE "(listening|server started|started successfully)" backend.log 2>/dev/null; then
    echo "后端就绪"
    break
  fi
  if grep -qiE "panic|fatal error" backend.log 2>/dev/null; then
    echo "后端启动失败，查看 backend.log"
    break
  fi
  # 同时检查端口是否已监听（比日志更可靠，使用 netstat 兼容 Windows）
  if netstat -ano 2>/dev/null | grep ':8090[[:space:]]' | grep -q LISTENING; then
    echo "后端就绪"
    break
  fi
  sleep 0.5
done

# 轮询前端日志，最多 5 次（约 2.5 秒）
cd D:/develop/myWebDemo/online/my-geektime/frontend
for i in $(seq 1 5); do
  if grep -qiE "(local:|vite ready|ready in)" frontend.log 2>/dev/null; then
    echo "前端就绪"
    break
  fi
  if grep -qiE "error|failed" frontend.log 2>/dev/null; then
    echo "前端启动失败，查看 frontend.log"
    break
  fi
  if netstat -ano 2>/dev/null | grep ':3000[[:space:]]' | grep -q LISTENING; then
    echo "前端就绪"
    break
  fi
  sleep 0.5
done
```

> 关键优化：`netstat` 端口检查作为日志检测的 fallback，兼容 Windows 环境。后端 Gin 启动后 `listening` 和端口监听几乎是同时的，但端口检查更直接。

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
