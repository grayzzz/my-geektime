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

- **必须用 Git Bash（MSYS），不要用 WSL** —— `taskkill` / `netstat` 是 Windows 原生程序，
  WSL 里调用不到。判定方法：`echo $MSYSTEM` 非空即 Git Bash（实测值 `MINGW64`）。
- 原生 CMD / PowerShell 不支持：`awk`、`seq`、`sleep 0.3`、`$(...)`
- 如果检测到非 Bash shell，直接提示用户切换
- **每个 Bash 调用都要先修 PATH**（见「环境限制」第 0 条），否则 `awk`/`sed`/`grep`/`seq`/`sleep`
  全部 `command not found` —— 这是本环境最容易踩的坑。

## 环境限制（实测踩坑，必读）

以下六条是实测结论，违反会导致操作失败。**第 0 条最先执行**：

0. **每个 Bash 调用开头都要修 PATH** —— 本环境默认 PATH 只含 Windows 的 `System32` 以及
   `go` / `node`，**MSYS 的 coreutils 完全不在其中**：`awk`、`sed`、`grep`、`seq`、`tr`、
   `head`、`sleep` 全部 `command not found`，文档里几乎所有代码块都会静默失败。必须在每条
   命令开头加：

   ```bash
   export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
   ```

   实测：加之前 `command -v awk` 为空，加之后为 `/usr/bin/awk`。`node` / `npm` / `go` 不受影响
   （`/usr/bin` 下没有同名文件，不会被遮蔽），`netstat` / `taskkill` 仍解析到 Windows 原生程序。

   > ⚠️ `/usr/bin` 必须排在 `System32` **前面**。否则 `find` 会解析成 Windows 的
   > `C:\Windows\System32\find.exe`（不支持 `-type` / `-newer` / `-name`），
   > 而不是 GNU find —— 会直接报参数错误。
   >
   > 同一个原因还有个坑：不修 PATH 时裸 `bash` 也不会解析到 MSYS 版（`/usr/bin/bash`），
   > 而会被路由到**已被安全策略禁用的 `wsl.exe`**，直接报 `PROGRAM BLOCKED BY SECURITY POLICY`。
   > 所以**命令一律内联执行**（该 Bash 工具本身就是 bash），不要写 `bash xxx.sh`。

1. **禁止在 Bash 命令里调用 `powershell -Command`** —— 安全钩子会直接拒绝整条命令
   （报错：`Command blocked for security: Invoking PowerShell from Bash bypasses PowerShell security checks`）。
   需要 PowerShell 时，用独立的 PowerShell 工具调用，不要塞进 Bash。

2. **端口探测统一用 Bash 原生 `/dev/tcp`** —— 最可靠，Docker 映射端口也能探测到。
   `netstat -ano` 只对原生端口（8090/3000）可见；本机 MySQL 走 Docker 端口映射时查不到记录，
   用它判断 3326 会误判为"未启动"。已验证可靠的写法：

   ```bash
   for p in 3326 8090 3000; do
     (echo > /dev/tcp/127.0.0.1/$p) 2>/dev/null && echo "PORT_$p=OPEN" || echo "PORT_$p=CLOSED"
   done
   ```

   > 例外：**取 PID 杀进程**时用 `netstat -ano | grep LISTENING | grep ":端口 "` 取最后一列，
   > 因为 `/dev/tcp` 拿不到 PID。

3. **后台启动必须用「常驻后台任务」，不能用 `cmd &`** —— 用 `&` 启动的进程会随命令 shell 一起被回收，
   端口瞬间关闭（尤其 `npm run dev`）。正确方式：把启动命令整体作为**常驻后台任务**运行（不带 `&`），
   让 shell 存活托管子进程：

   ```bash
   # 后端
   cd D:/develop/myWebDemo/online/my-geektime && ./my-geektime.exe server --config=config.yml > backend.log 2>&1
   # 前端
   cd D:/develop/myWebDemo/online/my-geektime/frontend && npm run dev > frontend.log 2>&1
   ```

   > 注：原生 exe 配 `nohup ... &` 可存活，但 `npm` / `vite` 这类脚本仍会被回收，统一用常驻方式最稳。

4. **`taskkill` 必须前置 `MSYS2_ARG_CONV_EXCL='*'`** —— 该 shim shell 下 MSYS 的 `//` → `/` 路径转换不生效，
   直接写 `taskkill //F //PID` 会报 `错误: 无效参数/选项 - '//F'`，命令静默失败、端口不释放。已验证可用的写法：

   ```bash
   MSYS2_ARG_CONV_EXCL='*' taskkill /F /T /PID "$PID"
   ```

   参数用 `/T` 终止整个进程树，**连带清理 vite 派生的 node 子进程**，以此取代
   `taskkill //F //IM node.exe` —— **按映像名杀 node 是危险的**：本机常年有 4 个左右 node 进程
   （含 WorkBuddy 自身、其它项目、MCP 服务），一律会被误杀。
   已实测：`/T` 能正确连带终止子进程，且操作后其它 node 进程数量不变。

5. **不要用 `wmic` 查进程命令行** —— 已被程序黑名单拦截（`PROGRAM BLOCKED BY SECURITY POLICY`），
   且规则明确禁止换 shell 绕过或重试。另外本环境中**独立的 PowerShell 工具可能返回空输出**
   （exit code 0 但无 stdout），此时查进程请改用：
   `tasklist /FI "PID eq <PID>" /FO LIST`（可取映像名、内存占用）；
   按名字枚举：`tasklist /FO CSV | grep -i "node.exe"`。

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
# 修 PATH（必须先执行，否则下面的 awk/grep/seq 都不可用，见「环境限制」第 0 条）
export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
cd D:/develop/myWebDemo/online/my-geektime

# 0. Shell 检查
if [ -z "$BASH_VERSION" ] && [ -z "$ZSH_VERSION" ]; then
  echo "BLOCK:请在 Git Bash 中运行"
  exit 0
fi
if [ -z "$MSYSTEM" ]; then
  echo "BLOCK:当前不是 Git Bash(MSYS)，taskkill/netstat 不可用"
  exit 0
fi

# 1. MySQL 检查（/dev/tcp 探测，不要用 netstat / powershell）
if ! (echo > /dev/tcp/127.0.0.1/3326) 2>/dev/null; then
  echo "BLOCK:MySQL未启动 (127.0.0.1:3326)"
  exit 0
fi

# 2. 端口占用（/dev/tcp 探测）
if (echo > /dev/tcp/127.0.0.1/8090) 2>/dev/null; then
  echo "BLOCK:端口8090被占用"
  exit 0
fi
if (echo > /dev/tcp/127.0.0.1/3000) 2>/dev/null; then
  echo "BLOCK:端口3000被占用"
  exit 0
fi

# 3. Go 可用性
go version >/dev/null 2>&1 || { echo "BLOCK:Go命令未找到"; exit 0; }

# 4. 前端依赖（node_modules 存在且有内容即视为就绪；不要用 -nt 比较 mtime，易误判）
if [ ! -d frontend/node_modules ] || [ ! -x frontend/node_modules/.bin/vite ]; then
  echo "NEED_NPM_INSTALL"
else
  echo "OK"
fi
```

输出解读：
- `BLOCK:请在 Git Bash 中运行` → 提示用户切换 shell
- `BLOCK:当前不是 Git Bash(MSYS)，taskkill/netstat 不可用` → 提示用户改用 Git Bash，不要用 WSL
- `BLOCK:MySQL未启动 (127.0.0.1:3326)` → 提示用户先启动 MySQL
- `BLOCK:端口8090被占用` → 回复用户请先"停掉"
- `BLOCK:端口3000被占用` → 同上
- `BLOCK:Go命令未找到` → 回复用户检查 PATH
- `NEED_NPM_INSTALL` → 启动前先执行 `npm install`
- `OK` → 全部通过

> 注意：MySQL 已在**上面的前置检查第 1 步**拦截，此处无需重复探测（原文「MySQL 检查移至启动后验证阶段」
> 与前置检查冲突，已更正）。若前置检查放行、但后端仍因连库失败起不来，再从 `backend.log` 提取报错，
> 按下面「MySQL 未启动」的格式汇报。

## 杀掉进程（单次调用）

> 本 agent 的工具集仅限 Bash（见 frontmatter `tools: Bash`），所以统一用下面的手动杀进程方式。
> 若所在环境另外提供了后台任务终止能力，且能定位到本次会话启动的服务，可优先用它。
> PID 取值用 `netstat` + `taskkill`（原生端口 8090/3000 可见；不要从 Bash 调 PowerShell）。

```bash
export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
cd D:/develop/myWebDemo/online/my-geektime
PID_8090=$(netstat -ano 2>/dev/null | grep "LISTENING" | grep ":8090 " | awk '{print $NF}' | head -n 1)
PID_3000=$(netstat -ano 2>/dev/null | grep "LISTENING" | grep ":3000 " | awk '{print $NF}' | head -n 1)
# taskkill 必须前置 MSYS2_ARG_CONV_EXCL='*'，否则 //F 不被转换会直接报"无效参数"（见"环境限制"第 4 条）
# 用 /T 终止进程树连带清理子进程（vite 会派生 node 子进程），
# 因此**不再需要** taskkill /IM node.exe —— 那是按映像名杀，会误杀本机其它 node 服务
if [ -n "$PID_8090" ]; then MSYS2_ARG_CONV_EXCL='*' taskkill /F /T /PID "$PID_8090" >/dev/null 2>&1; fi
if [ -n "$PID_3000" ]; then MSYS2_ARG_CONV_EXCL='*' taskkill /F /T /PID "$PID_3000" >/dev/null 2>&1; fi
# 轮询确认释放，最多 5 次（每次 0.3 秒）
# 必须「两个端口都释放」才退出：直接用 `探测 || break` 会在任一端口先释放时立刻退出，
# 导致另一端口还在关就被误判（实测：8090 仍占用时循环会 0 次退出）
for i in $(seq 1 5); do
  if ! (echo > /dev/tcp/127.0.0.1/8090) 2>/dev/null && ! (echo > /dev/tcp/127.0.0.1/3000) 2>/dev/null; then
    break
  fi
  sleep 0.3
done
# 确认
(echo > /dev/tcp/127.0.0.1/8090) 2>/dev/null && echo "端口8090仍占用" || echo "端口8090已释放"
(echo > /dev/tcp/127.0.0.1/3000) 2>/dev/null && echo "端口3000仍占用" || echo "端口3000已释放"
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
4. 启动后端和前端（**各作为一次常驻后台任务**，命令末尾不要加 `&`）：
   ```bash
   # 后端（覆盖日志）
   cd D:/develop/myWebDemo/online/my-geektime && ./my-geektime.exe server --config=config.yml > backend.log 2>&1
   ```
   ```bash
   # 前端
   cd D:/develop/myWebDemo/online/my-geektime/frontend && npm run dev > frontend.log 2>&1
   ```
   两次调用都设置 `run_in_background=true`，让 shell 常驻托管进程；否则进程会随命令结束被回收。
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
5. 启动后端（覆盖日志，作为常驻后台任务，末尾不加 `&`）：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime && ./my-geektime.exe server --config=config.yml > backend.log 2>&1
   ```
6. 启动前端（覆盖日志，作为常驻后台任务，末尾不加 `&`）：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime/frontend && npm run dev > frontend.log 2>&1
   ```
7. 执行等待就绪逻辑 + 汇报

## 只重启后端

1. 停后端进程（用 netstat 查监听 PID；不要从 Bash 调 PowerShell）
   ```bash
   export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
   cd D:/develop/myWebDemo/online/my-geektime
   PID_8090=$(netstat -ano 2>/dev/null | grep "LISTENING" | grep ":8090 " | awk '{print $NF}' | head -n 1)
   if [ -n "$PID_8090" ]; then MSYS2_ARG_CONV_EXCL='*' taskkill /F /T /PID "$PID_8090" >/dev/null 2>&1; fi
   # 轮询确认释放，最多 5 次（每次 0.3 秒）
   for i in $(seq 1 5); do
     (echo > /dev/tcp/127.0.0.1/8090) 2>/dev/null || break
     sleep 0.3
   done
   ```
2. 执行前置检查
3. 后端编译（源码有更新则重新编译）：
   ```bash
   export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
   cd D:/develop/myWebDemo/online/my-geektime
   # web/ 由 main.go 的 //go:embed web/* 内嵌进 exe，前端产物更新后同样需要重编译，
   # 因此 web 也要纳入 -newer 检查，否则会出现「exe 里是旧前端」的静默过期
   if [ ! -f my-geektime.exe ] || [ -n "$(find internal cmd libs web main.go -type f \( -name '*.go' -o -name '*.proto' -o -name '*.tpl' -o -name '*.js' -o -name '*.html' -o -name '*.css' \) -newer my-geektime.exe 2>/dev/null | head -1)" ]; then
     go build -o my-geektime.exe
   fi
   ```
4. 运行编译后的二进制（作为常驻后台任务，末尾不加 `&`）
5. 前端不动
6. 执行等待就绪逻辑 + 汇报

## 只重启前端

1. 停前端进程（用 netstat 查监听 PID；不要从 Bash 调 PowerShell）
   ```bash
   export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
   cd D:/develop/myWebDemo/online/my-geektime
   PID_3000=$(netstat -ano 2>/dev/null | grep "LISTENING" | grep ":3000 " | awk '{print $NF}' | head -n 1)
   if [ -n "$PID_3000" ]; then MSYS2_ARG_CONV_EXCL='*' taskkill /F /T /PID "$PID_3000" >/dev/null 2>&1; fi
   # /T 已连带终止 vite 派生的 node 子进程，不再按映像名杀 node.exe（会误杀本机其它 node 服务）
   # 轮询确认释放，最多 5 次（每次 0.3 秒）
   for i in $(seq 1 5); do
     (echo > /dev/tcp/127.0.0.1/3000) 2>/dev/null || break
     sleep 0.3
   done
   ```
2. 依赖检查：按前置检查的 `NEED_NPM_INSTALL` 输出判定（即 `frontend/node_modules/.bin/vite`
   不可用时才 `npm install`）。**不要用 mtime 比较判断依赖是否过期**——易误判。
3. 启动前端（覆盖日志，作为常驻后台任务，末尾不加 `&`）：
   ```bash
   cd D:/develop/myWebDemo/online/my-geektime/frontend && npm run dev > frontend.log 2>&1
   ```
4. 后端不动
5. 执行等待就绪逻辑 + 汇报

## 停止

```bash
export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
cd D:/develop/myWebDemo/online/my-geektime
# 杀掉 8090 和 3000 端口的进程（用 netstat 查监听 PID；不要从 Bash 调 PowerShell）
PID_8090=$(netstat -ano 2>/dev/null | grep "LISTENING" | grep ":8090 " | awk '{print $NF}' | head -n 1)
PID_3000=$(netstat -ano 2>/dev/null | grep "LISTENING" | grep ":3000 " | awk '{print $NF}' | head -n 1)
if [ -n "$PID_8090" ]; then MSYS2_ARG_CONV_EXCL='*' taskkill /F /T /PID "$PID_8090" >/dev/null 2>&1; fi
if [ -n "$PID_3000" ]; then MSYS2_ARG_CONV_EXCL='*' taskkill /F /T /PID "$PID_3000" >/dev/null 2>&1; fi
# 用 /T 终止进程树即已清理 vite 子进程，不要再 taskkill /IM node.exe（会误杀本机其它 node）
# 轮询确认释放，最多 5 次（每次 0.3 秒）；必须两个端口都释放才退出
for i in $(seq 1 5); do
  if ! (echo > /dev/tcp/127.0.0.1/8090) 2>/dev/null && ! (echo > /dev/tcp/127.0.0.1/3000) 2>/dev/null; then
    break
  fi
  sleep 0.3
done
# 确认
(echo > /dev/tcp/127.0.0.1/8090) 2>/dev/null && echo "端口8090仍占用" || echo "端口8090已释放"
(echo > /dev/tcp/127.0.0.1/3000) 2>/dev/null && echo "端口3000仍占用" || echo "端口3000已释放"
```

汇报结果。

## 等待就绪逻辑

轮询端口是否监听，间隔 0.3 秒，日志仅用于兜底捕获启动错误：

```bash
export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
# 后端：端口优先，最多 20 次（约 6 秒）
cd D:/develop/myWebDemo/online/my-geektime
for i in $(seq 1 20); do
  if (echo > /dev/tcp/127.0.0.1/8090) 2>/dev/null; then
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

# 前端：最多 20 次（约 10 秒，vite 首次冷启动约 3-5 秒）
cd D:/develop/myWebDemo/online/my-geektime/frontend
for i in $(seq 1 20); do
  if (echo > /dev/tcp/127.0.0.1/3000) 2>/dev/null; then
    echo "前端就绪"
    break
  fi
  sleep 0.5
done
```

> 关键优化：端口监听通常在前 1-5 秒发生，优先查端口；日志仅用于捕获 panic / 启动错误，避免无意义轮询。
> 前端日志含 `Browserslist: browsers data is 8 months old` 属正常噪音，不要据此判定失败。

如果超时未就绪，查看日志末尾的报错信息并汇报给用户。

## 健康校验（启动完成后建议执行）

```bash
export PATH="/usr/bin:/bin:/c/Windows/System32:$PATH"
for p in 3326 8090 3000; do
  (echo > /dev/tcp/127.0.0.1/$p) 2>/dev/null && echo "PORT_$p=OPEN" || echo "PORT_$p=CLOSED"
done
# ⚠️ curl 丢弃正文必须写 `-o NUL`，不要写 `-o /dev/null`：
# 本环境 `-o /dev/null` 会让 curl 以退出码 23（写正文失败）结束，即使 HTTP 200 也一样，
# 会被误判成校验失败。实测：`-o /dev/null` → exit 23；`-o NUL` → exit 0。
curl -s -o NUL -w "backend  -> HTTP %{http_code}\n" --max-time 10 http://127.0.0.1:8090/v2/base/setting
curl -s -o NUL -w "frontend -> HTTP %{http_code}\n" --max-time 10 http://127.0.0.1:3000/
```

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
