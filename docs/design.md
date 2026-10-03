# MihomoBar 设计文档

> 版本：v0.1（首版，仅 macOS）
> 技术栈：Go 1.25+ · Wails v3 (beta.27) · React 18 + TypeScript + Vite · mihomo（`third_party/mihomo` 子模块，`Demogorgon314/mihomo@openconnect-support`）

## 1. 目标与范围

一个 macOS 的 mihomo 管理工具，有菜单栏面板和完整主窗口两套界面。mihomo 作为 Go 依赖从源码编进 app，不额外下载或打包预编译的内核二进制。

### 首版（v0.1）
| 模块 | 内容 |
|---|---|
| 内核 | 启动、停止、重启；启动前校验配置；崩溃检测 |
| 模式 | 规则、全局、直连 |
| 代理 | 代理组列表、切换节点、单节点/整组测速、按延迟排序 |
| 订阅 | 导入 URL 或本地 YAML、更新（单个/全部，可设自动更新间隔）、删除、切换当前配置 |
| 系统代理 | 开关，可配置绕过列表 |
| TUN | 开关，选择栈（system / gvisor / mixed）；首次使用需授权 |
| 连接 | 实时列表、搜索、关闭单条或全部 |
| 规则 | 列表、搜索 |
| 日志 | 实时流、按级别过滤、搜索 |
| 概览 | 实时上下行速率与曲线、内存、连接数、累计流量 |
| 设置 | 端口、allow-lan、ipv6、日志级别、开机启动、主题、语言、Dock 图标 |

### 后续版本
- SSID 自动切换配置
- 远程机器管理
- 应用自动更新
- Provider 管理页
- 配置编辑器

内核随 app 一起升级，不需要 ClashBar 那套单独更新内核的逻辑。

## 2. 总体架构

同一个可执行文件，三种运行模式：

```
MihomoBar.app/Contents/MacOS/mihomobar
│
├── (默认)  GUI 模式 ── Wails v3 应用：托盘面板 + 主窗口
│               │  只通过 REST/WS 调用内核，不直接访问 mihomo 的全局状态
│               ▼
├── core     内核模式 ── import mihomo，hub.Parse(runtime.yaml)
│               在 127.0.0.1:<端口> 提供 external-controller，用随机 secret 鉴权
│
└── helper   特权模式 ── root LaunchDaemon（只在用户开启 TUN 或授权系统代理后安装）
                负责以 root 身份拉起 core、设置系统代理；通过 unix socket 和 GUI 通信，并校验对端 uid
```

### 2.1 为什么内核要单独一个进程，而不是跑在 GUI 进程里
调研结论（详见第 9 节）：
1. mihomo 的状态都是包级单例：`executor.Shutdown()` 不会关闭端口监听和 REST 服务，`/restart` 会对宿主进程执行 `syscall.Exec`。
2. TUN 需要 root，而 WebView 进程不能以 root 运行。
3. 内核崩溃不能把 UI 一起带走。

所以 GUI 用 `os.Executable()` 加上 `core` 参数把自己拉起来作为内核进程。整个产物仍然只有一个二进制，mihomo 也仍然是从源码编译的 Go 依赖。

### 2.2 两种内核运行方式
| | 用户模式（默认） | 服务模式（开启 TUN 时） |
|---|---|---|
| 谁拉起 core | GUI 直接拉起子进程 | helper（root）拉起子进程 |
| core 身份 | 当前用户 | root |
| 是否支持 TUN | 否 | 是 |
| 系统代理怎么设 | `networksetup`（当前用户身份） | helper 用 SCPreferences / networksetup |
| 首次使用 | 无需授权 | osascript 弹一次管理员授权，安装 LaunchDaemon |

开启 TUN 时：helper 没装就先安装，然后让 helper 以 root 重新拉起 core，GUI 再重连控制接口。关闭 TUN 时 core 继续留在服务模式，避免来回切换。

### 2.3 控制接口与安全
- **core 控制接口**：`127.0.0.1:<空闲端口>`，secret 是每次启动随机生成的 32 字节。不用 mihomo 自带的 unix socket，因为它不校验 secret，而且文件权限是 0666；core 以 root 运行时，任何本地进程都能控制它。
- **helper 的 socket**：`/var/run/mihomobar-helper.sock`。每个连接都用 `getsockopt(LOCAL_PEERCRED)` 取出对端 uid，只放行安装时记录的 uid。这是对 ClashBar 的修正，它的 XPC helper 不校验调用方。
- **helper 的接口**：只做固定的几件事：`core.start(home, configPath)`、`core.stop`、`core.status`、`proxy.set/clear/get`、`version`。不执行任意命令。configPath 必须在安装用户的数据目录下。
- **残余风险**：root core 读取的是用户目录里的配置文件，所以以该用户身份运行的进程可以通过改配置影响 root core。所有 clash 类 GUI 的“服务模式”都有这个问题，在设置页的说明里写明。

## 3. 目录与文件布局

数据目录：`~/Library/Application Support/MihomoBar/`
```
settings.json          应用设置（主题、语言、当前配置、端口覆盖、开关状态等）
profiles/
  index.json           订阅元数据（名称、URL、更新间隔、上次更新时间、流量/到期信息）
  <id>.yaml            每个配置一个文件
core/                  mihomo 的 home 目录（geo 数据库、cache.db、providers 等）
  runtime.yaml         实际交给 core 的配置（由“当前配置 + 应用覆盖项”生成）
logs/app.log
```

### 3.1 runtime 配置生成（相对 ClashBar 的改进）
ClashBar 是先启动 core、再用 `PATCH /configs` 补用户设置，中间有一段状态不一致的窗口期。我们改成启动前就把覆盖项合并好：
1. 读取当前配置的 YAML，解析成 `map[string]any`，未知字段原样保留。
2. 用应用设置覆盖这些键：`mixed-port`、`allow-lan`、`ipv6`、`log-level`、`mode`、`external-controller`、`secret`、`tun.enable/stack`，同时删除 `external-controller-unix`。
3. 写入 `core/runtime.yaml`（权限 0600），然后执行 `core -t` 校验，再启动或热重载（`PUT /configs?force=true {path}`）。

运行中切换模式、端口、开关时，先 `PATCH /configs` 立即生效，同时写回 settings.json，下次生成 runtime 配置时也会带上。

## 4. Go 代码结构

```
main.go                     入口：根据参数分发到 gui / core / helper
internal/
  appdir/                   数据目录路径
  settings/                 settings.json 读写
  profiles/                 订阅：下载（UA clash.meta）、校验、存储、定时更新
  runtimecfg/               生成 runtime.yaml（YAML 合并）
  core/                     【core 模式】进程内运行 mihomo：hub.Parse、信号处理、-t 校验
  coremgr/                  【GUI 侧】内核进程管理：拉起/停止/重启、崩溃检测、健康检查；用户模式与服务模式两种实现
  mihomoapi/                mihomo REST/WS 客户端（类型化）
  sysproxy/                 系统代理（networksetup 实现，helper 内复用同一实现）
  helper/                   【helper 模式】root 守护进程，以及安装/卸载（osascript + launchctl）
  autostart/                开机启动（SMAppService.mainApp）
  gui/
    app.go                  Wails 应用：托盘、面板、主窗口
    glide_darwin.go         面板高度的原生动画、Dock 图标开关
    services.go             暴露给前端的 Wails 服务
    events.go               推送给前端的事件：traffic/memory/logs/connections/state
frontend/                   React + TS + Vite
third_party/mihomo/         子模块
```

### 4.1 go.mod
- `replace github.com/metacubex/mihomo => ./third_party/mihomo`
- 原样复制 fork 里的 5 条 `replace`：protobuf、sing-openconnect、dtls/v3、sing-wireguard、gvisor。fork 升级时用脚本 `scripts/sync-replaces.sh` 同步。
- 构建参数：`-tags with_gvisor,production`；ldflags 注入 `constant.Version`。

### 4.2 Wails 服务（前端通过自动生成的 TS 绑定调用）
| 服务 | 方法 |
|---|---|
| `AppService` | `State()`、`Start/Stop/Restart()`、`SetMode`、`SetSystemProxy`、`SetTun`、`ShowMain(view)`、`HidePanel`、`FitPanel(h, ms)`、`Quit` |
| `ProxyService` | `Groups()`、`Select(group, name)`、`TestLatency(kind, name)`（node/group/provider/all） |
| `ProfileService` | `List()`、`Import(url\|file)`、`Update(id)`、`UpdateAll()`、`Remove(id)`、`Use(id)`、`Reveal()` |
| `ConnService` | `Close(id)`、`CloseAll()` |
| `RuleService` | `Rules()` |
| `SettingsService` | `Get()`、`Patch(partial)` |

**事件（Go → 前端）**：
- `state`：内核状态、模式、开关
- `traffic`：每秒一次的上下行速率
- `memory`
- `connections`：只在连接页或概览页可见时订阅，减少开销
- `log`：只在日志页可见时订阅

前端通过 `Watch(topic, on)` 告诉 Go 当前需要哪些流，Go 侧按订阅情况打开或关闭对应的 mihomo WebSocket。

## 5. 界面设计

### 5.1 窗口
| 窗口 | 规格 |
|---|---|
| 托盘面板 | 宽 440pt，高度跟随内容，范围 220–560；无边框、半透明背景（`MacBackdropTranslucent`）、圆角 12；失焦或按 Esc 自动隐藏；`AttachWindow(...).WindowOffset(6)`；高度变化走原生动画，与 CSS 使用同一条贝塞尔曲线 |
| 主窗口 | 默认 860×600，最小 640×440，记住上次大小；`MacTitleBarHiddenInset`；页面顶栏可拖动，左侧留 96px 给红绿灯按钮；点关闭只隐藏，全屏时先退出全屏再隐藏；启用 `MoveToActiveSpace` |
| Dock 图标 | `LSUIElement`；可在设置里选：始终显示 / 不显示 / 主窗口打开时显示 |

### 5.2 主窗口布局
- **顶栏（50px）**：
  - 左侧：logo 和名称。
  - 中间：分段 Tab，只有一个滑动的高亮块，没有灰色底轨。Tab 依次是：概览、代理、配置、连接、规则、日志。
  - 右侧：内核状态胶囊（运行中显示绿点，停止显示灰点，出错显示红点）、模式切换、设置按钮（26px 图标按钮）。
- **内容区**：页面进入时淡入并上移 5px（`view-in .24s`）。内容是圆角 12 的卡片列表 `.list`，每行 `.row` 高 48，行与行之间用细分割线。
- **概览**：
  - 第一行：流量曲线卡片（上下行两条线，60 秒滑动窗口）。
  - 第二行：四个数字卡片：上传速率、下载速率、连接数、内存。
  - 第三行：快捷开关卡片：系统代理、TUN、模式；以及当前配置卡片。
- **代理**：每个代理组一张卡片。卡片头显示组名、类型、当前节点和测速按钮；可以折叠展开，展开动画用 grid-rows 过渡，各行依次错开出现。节点以网格排列（名称加延迟胶囊），选中的节点用 `--sel` 背景色标出。延迟颜色：小于 200ms 绿色，小于 500ms 琥珀色，更高为红色，超时为灰色。
- **配置**：配置列表，当前使用的配置有标记。每行显示名称、来源（URL 或本地）、流量用量进度条、到期时间、上次更新时间。行内操作：更新、更多菜单（复制链接、在 Finder 中显示、删除），删除需要二次确认。顶部有“导入”按钮，点开是一个从按钮位置展开的弹出层，里面输入 URL 或选择本地文件。
- **连接**：保留进程／Host／规则分组和侧栏详情，提供活跃／已关闭／全部、暂停画面、时间／Host／上下行速度／累计流量排序。搜索按空格分词并匹配所有词，覆盖源地址和端口、目标、嗅探域名、进程及代理链；关闭操作只提交当前结果中活跃连接的 ID，失败的连接保留并可重试。上下行速度和累计流量分别展示。主窗口持续串行采样，即使切换页面或暂停画面仍记录最近 500 条已结束连接；切换配置清空历史。历史为会话内存数据，结束时间和流量是最近采样估计，采样间隔内完成的短连接可能无法记录。
- **规则**：搜索框加列表，每行显示类型、内容和策略。
- **日志**：级别分段控件加搜索框，等宽字体，自动滚动到底部（用户向上滚动后暂停自动滚动），可清空。
- **设置**：分组卡片，包括：通用（开机启动、Dock 图标、主题、语言）、网络（混合端口、allow-lan、ipv6、日志级别、系统代理绕过列表）、TUN（栈、服务模式状态与卸载）、关于（版本号、mihomo 版本）。

### 5.3 托盘面板布局
- **顶部**：左边是状态点、配置名和实时速率（↑ ↓），右边是“全部测速”（内核运行时）、“打开主窗口”和设置三个图标按钮。
- **模式分段控件**：规则、全局、直连，带滑动高亮块。
- **开关行**：系统代理、TUN，使用 iOS 风格开关。
- **流量迷你曲线**，同一张卡片下面是“活跃应用”：流量最多的 3 个应用（图标、名称、当前速率），面板显示期间每秒刷新，点击打开连接页；没有连接时显示“暂无活跃应用”。
- **代理组列表**：每个组一行，显示组名和当前节点。点击后在面板内展开该组的节点（折叠展开动画），面板高度随之平滑变化。
- **底部**：内核启动/重启、退出。

### 5.4 设计变量
- 颜色变量：`--bg / --card / --pill / --line / --fg / --muted / --accent(#4f46e5 · 暗色 #a5b4fc) / --green / --red / --amber` 及对应的 `-soft` 版本。面板在毛玻璃背景上另有一组覆盖值。
- 字体：13px/1.4，系统字体栈；等宽字体用 `ui-monospace`。全局 `user-select:none` 和 `cursor:default`，让界面更像原生应用。
- 动效：
  - 通用缓动曲线 `--ease: cubic-bezier(.2,.8,.2,1)`。
  - 滑动高亮块：`transform/width .28s`。
  - 弹出层：`pop-in .42s cubic-bezier(.16,1,.3,1)`，从触发按钮的位置展开（`--ox`）；关闭时播放 `pop-out .22s`。
  - 页面切换：`view-in .24s`。
  - 折叠展开：高度 `.62s cubic-bezier(.22,1,.36,1)`，各行错开 48ms 依次出现。
  - 状态更新时行背景闪烁一下（`flash`）。
  - 测速中的闪电图标变成主题色，轮廓变暗，一段亮的描边沿轮廓流动（`zap`）；刷新类的圆形箭头图标仍然旋转（`spin`）。
  - 遵循系统的“减弱动态效果”设置（`prefers-reduced-motion`）。
- 主题：跟随系统、浅色、深色三种。在首屏绘制前通过 `data-theme` 设定，避免闪烁。
- 国际化：以英文原文为 key 的中文词典，`t("Proxies")`；语言可选跟随系统、中文、英文；托盘菜单文案同步切换。

### 5.5 前端技术选型
- React 18 + TS + Vite（Wails 模板）、`@wailsio/runtime`，以及自动生成的绑定。
- 状态管理：zustand。路由：不用路由库，tab 状态存在 store 里，面板通过 `?mode=panel` 区分。
- 样式：手写 CSS 和 CSS 变量，不引入 Tailwind，以保证细节一致。
- 图表：自己写 SVG 折线，不引入图表库。

## 6. 关键流程

**启动**
1. 加载 settings。
2. 生成 runtime.yaml。
3. 执行 `core -t` 校验。
4. 拉起 core：用户模式由 GUI 直接拉起；如果上次是服务模式且 helper 可用，交给 helper 拉起。
5. 轮询 `/version`，就绪后打开 WebSocket 流。
6. 恢复系统代理状态。
7. 后台刷新 provider，并对所有组测一次速。

**退出**：关闭系统代理（如果是本应用开启的），停止 core（先 SIGTERM，2 秒后 SIGKILL）。服务模式下通知 helper 停止 core。

**崩溃**：core 意外退出时，状态变为 `crashed`，并显示最后 50 行输出；3 秒后自动重启一次，再次失败就停止并提示。

**系统代理**：设置所有已启用网络服务的 web、secureweb、socks 代理，加上绕过列表。退出或 core 停止时清除。启动时如果发现系统代理指向我们的端口，但 core 没有运行，就先清理掉。

## 7. 里程碑
| 阶段 | 内容 |
|---|---|
| M1 骨架 | 依赖共存验证、`core` 子命令、coremgr（用户模式）、托盘和面板、主窗口外壳、设计变量 |
| M2 核心功能 | 模式切换、代理组和测速、配置导入与切换、系统代理、概览流量 |
| M3 | 连接、规则、日志、设置 |
| M4 | helper 与 TUN、开机启动、打包（`.app`、签名、DMG）——已完成；需要在真机上用管理员授权端到端验证 TUN |

## 8. 构建与打包
- `task dev`：`wails3 dev`，带 Vite 热更新。
- `task build`：先构建前端，再 `go build -tags with_gvisor,production`。
- `task package`：生成 `.app`；Info.plist 设置 `LSUIElement`、最低系统 12.0、bundle id `com.localhost-copilot.mihomobar`；做 ad-hoc 签名，然后用 `hdiutil` 打 DMG。
- 没有 Developer ID：helper 通过 osascript 授权安装，安装后不在 Gatekeeper 的评估范围内（安装时由 root 拷贝到 `/Library/PrivilegedHelperTools`，并清除 quarantine 属性）。

## 9. 调研记录（摘要）
- **作为库调用**：实测`hub.Parse(bytes)` 加 `/version` 正常，同一进程内连续加载两次也正常。带 gvisor 的二进制约 60MB。
- **cgo**：darwin 上只有 `tailscale/certstore` 用到 cgo，关掉 cgo 时有替代实现；openconnect 不用 cgo。
- **`replace` 不会被依赖方继承**：使用方必须复制那 5 条，否则 `go mod tidy` 报 `unknown revision`。
- **`init()` 副作用**：logrus 默认输出到 stdout；`statistic` 包会启动一个 1s ticker；系统 DNS 兜底为 114/8.8.8.8。GUI 和 core 是同一个二进制，所以 GUI 进程也会执行这些 init()。不过它们只是一个空转的 ticker 和几个全局变量，影响可以忽略；GUI 从不调用 hub/executor。
- **ClashBar 可借鉴的点**：先 `-t` 校验再启动；订阅下载使用 `clash.meta` UA；网络恢复后自动重启 core；按标签页的可见性和前后台状态调整轮询和 WebSocket 订阅。

## 10. 交互逻辑参考 Surge for Mac

调研来源：manual.nssurge.com、kb.nssurge.com（release notes）。Surge 的菜单项都在官方资料中能找到，但官方没有公开菜单的排列顺序，下面的顺序按 Surge 5/6 的实际使用整理。

### 10.1 采纳的概念
- **出站模式**（Direct / Global / Rule）是凌驾于规则之上的全局开关，与 mihomo 的 `mode` 一一对应。主窗口和托盘都把它放在最显眼的位置。
- **系统代理**与**增强模式（TUN）**是两个独立开关，可以同时开启。图标状态要反映“当前是否真的在接管流量”。
- **代理组延迟**取当前选中成员的延迟；测速结果可以排序；HTTPS 测速不计入 TLS 握手时间（mihomo `unified-delay` 已经支持）。
- **手动测速**对齐 zashboard 默认 Dashboard 模式：页面、面板和原生菜单统一使用设置中的全局测速 URL；Go 后端共享 5 个请求槽位并合并重复的在途请求。单节点测试跟随嵌套组当前选中的节点，超时 5 秒；Selector、LoadBalance、Smart 组和 Provider 逐节点测试，超时 2 秒；其他组调用核心整组测速接口，超时 5 秒，以更新核心的健康状态和自动选择。“全部测速”覆盖核心和 Provider 的非组节点，按名称去重，排除 Reject、RejectDrop 和 Block，不受当前搜索过滤影响。`proxy-latency` 事件同步进度和逐节点结果，结束后各界面重新获取核心数据。未提供独立测速 URL 模式。
- **托管配置不可编辑，新配置不合法就不加载**：我们已经做到“更新后先 `core -t` 校验，失败就保留旧配置”。

### 10.2 托盘：左键面板 + 右键原生菜单
左键打开面板（第 5.3 节），右键弹出 Surge 风格的原生菜单，顺序如下：
1. 内核没在运行时才有状态行（已停止 / 正在启动 / 异常退出）；运行中的状态由图标表达（第 10.3 节）
2. 显示主窗口 ⌘M
3. “出站模式”子菜单（三个单选项），右侧用强调色徽标显示模式字母 R / G / D，和托盘图标一致
4. 每个代理组一个子菜单：组名在左，当前成员在右（灰色、右对齐）；子菜单第一项是“测速”，下面是节点：当前成员前打 ✓，延迟是右侧的彩色徽标。所有组之后是“全部测速”。点“测速”或“全部测速”不会关闭菜单，节点先显示“···”，结果出来后原地换成延迟
5. 连通质量：右侧徽标显示经代理（直连模式下为直连）的延迟，按延迟着色。菜单打开时如果结果超过 30 秒就重测；内核启动、切换模式或配置后也会测一次
6. 活跃应用：固定 5 行，按当前速率排序，显示应用图标、名称和速率，空行显示“—”。同一个 .app 里的辅助进程算作该应用。只在菜单打开期间每秒采样 `/connections`，数字实时更新；点击打开连接页
7. 仪表盘… ⌘D（打开连接页）
8. 设置为系统代理 ⌘S / 增强模式 (TUN) ⌘E
9. “配置”子菜单（右侧显示当前配置；✓ 标出当前配置，点击即切换）、更新全部订阅；复制终端代理命令
10. 启动 / 重启 / 停止内核
11. 退出 ⌘Q

AppKit 原生菜单项会在标题和子菜单箭头之间留出很宽的空隙，快捷键也要单独占一列，排不出 Surge 那样的紧凑布局。所以 `traymenu_darwin.go` 给每个菜单项都配一个自绘视图（`MBRowView`），自己画勾选标记、应用图标、标题、右侧的灰色文字或徽标，以及子菜单箭头，悬停高亮也自己画；点击时先关闭菜单，再触发菜单项原本的动作。Go 这边只在 Wails 菜单项的标题里加几个标记，表示右侧文字、徽标和图标。快捷键当作右侧文字显示（“⌘ M”），真正的快捷键挂在一个隐藏的同名菜单项上（`allowsKeyEquivalentWhenHidden`）。行高 22pt。菜单打开期间不整体重建，只原地更新连通质量和活跃应用两部分；期间的其他变化等菜单关闭后再重建。

### 10.3 图标状态
- 内核未运行：图标半透明、只有描边。
- 内核运行中，但系统代理和 TUN 都没开（没有流量被接管）：同样显示描边图标。这一点学 Surge，提醒用户“其实没生效”。
- 正在接管流量：实心图标。可选在图标旁显示速率（设置“在菜单栏显示速率”）。
- 后续：检测到系统代理被其他应用改写时，图标变灰并发通知。

### 10.4 后续可做（按价值排序）
1. 概览页增加连通性卡片：路由器 (ICMP)、DNS、互联网、代理 (HTTP) 四项延迟。
2. 连接页：按进程 / 主机 / 规则分组；右侧详情栏（规则、链路、进程路径、时间）；右键“为此主机添加规则”（写入用户规则覆盖层）。
3. 规则命中次数（mihomo 的 `/rules` 有 `extra.hitCount`）。
4. 远程配置只读，提供“创建可编辑副本”，以覆盖层的形式保留用户自己的规则。
5. 自定义隐藏的代理组和代理组分类折叠（mihomo 的 `hidden` 字段已支持）；按住 Option 打开菜单时显示隐藏的组。
6. Option 点击节点：只测该节点；Option 点击“复制终端代理命令”：使用局域网 IP。
7. 事件列表与通知（自动选择组切换了节点、订阅更新失败等）。

### 10.5 不适用于 mihomo 的部分
网关 / DHCP / 设备管理、Surge Ponte、Snell / MTProto 服务端、计费网络模式、Panels 脚本、MitM / Rewrite / HTTP 抓包、`#!include` 关联配置。
