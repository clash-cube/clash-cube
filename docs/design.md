# ClashCube 设计文档

只记录从代码里不容易看出来的东西：架构上的取舍、安全边界、配置生成规则，以及几处行为约定。界面细节以代码为准。

## 1. 架构

同一个可执行文件，三种角色（`main.go` 按 `os.Args[1]` 分发）：

```
ClashCube.app/Contents/MacOS/clashcube
├── (默认)        GUI：Wails v3，托盘面板 + 主窗口；只通过 REST/WS 访问内核
├── core          进程内运行 mihomo（hub.Parse(runtime.yaml)），控制接口在 127.0.0.1
└── helper serve  root LaunchDaemon，以 root 拉起 core（TUN 需要）
```

### 1.1 内核为什么单独一个进程
1. mihomo 的状态都是包级单例：`executor.Shutdown()` 不会关闭端口监听和 REST 服务，`/restart` 会对宿主进程 `syscall.Exec`。
2. TUN 需要 root，WebView 进程不能以 root 运行。
3. 内核崩溃不能带走界面。

GUI 用 `os.Executable()` 加 `core` 参数拉起自己。GUI 进程也会执行 mihomo 各包的 `init()`（一个空转的 statistic ticker 和几个全局变量），影响可忽略；GUI 从不调用 hub/executor。

### 1.2 两种运行方式
| | 用户模式（默认） | 服务模式（TUN） |
|---|---|---|
| 谁拉起 core | GUI | helper（root） |
| 支持 TUN | 否 | 是 |
| 首次使用 | 无需授权 | osascript 弹一次管理员授权，安装 LaunchDaemon |

系统代理两种模式下都由 GUI 以当前用户身份调用 `networksetup`。开启 TUN 时如果 helper 没装就先安装，再由 helper 重新拉起 core。关闭 TUN 后 core 仍留在服务模式，避免来回切换。

### 1.3 控制接口与安全
- **core 控制接口**：`127.0.0.1:<空闲端口>`，secret 每次启动随机生成。不用 mihomo 的 unix socket：它不校验 secret、文件权限 0666，core 以 root 运行时任何本地进程都能控制它。
- **helper socket**：`/var/run/clashcube-helper.sock`，每个连接只处理一条 JSON 请求；用 `LOCAL_PEERCRED` 取对端 uid，只放行安装用户和 root。
- **helper 操作**：只有 `start`、`stop`、`version`、`update`。`start` 只接受 `<数据目录>/core/runtime.yaml`，拒绝符号链接和非本人所有的文件。不执行任意命令或路径。
- **helper 免密更新**：helper 和 app 是同一个二进制，每次发版 hash 都变。没有 Developer ID，所以用自己的 ed25519 钥匙：`bundle.sh` 对 `go build` 的产物签名（`Contents/Resources/helper.sig`），签的是去掉 Mach-O 签名区后的 hash，ad-hoc codesign 不影响它。GUI 发现 helper 不是当前版本时发 `update`；helper 把文件读进内存（`O_NOFOLLOW`、必须属于本人）再验签，要求构建时间比自己新，然后 `rename` 替换自己并退出，由 launchd 拉起新版本。验签失败（自己编译、没有钥匙）就退回弹密码重装。私钥泄露等于所有装了 helper 的机器的 root，只放在构建者本机或 CI secret 里。
- **app 自更新**（`internal/appupdate`）：每个 release 带一份 `update.json`，列出版本、构建时间戳和两个架构 zip 的 SHA-256，用同一把 ed25519 钥匙签名（消息带 `clashcube-app-update` 前缀，和 helper 签名区分开）。app 从 GitHub API 找版本最高的、带 `update.json` 的非草稿 release（预发布也算），验清单签名，下载 zip 校验 hash，解包后再要求 `codesign --verify` 通过、可执行文件的 `helper.sig` 能用内置公钥验过、时间戳等于清单所写且比自己新。之后把新 bundle `rename` 到旧 bundle 的位置，运行中的进程不受影响；“重启以更新”立即换并重新打开，直接退出时也会换（不弹密码）。所在目录不可写就用 osascript 要管理员密码；从 DMG 或 App Translocation 里运行时只给下载链接。下载经过 mixed 端口（有监听时）。自己编译的版本（`git describe` 带提交数或 dirty）只检查不下载。新版启动后照常发现 helper 不是当前版本，免密更新它。
- **残余风险**：root core 读取用户目录里的配置，所以以该用户身份运行的进程能通过改配置影响 root core。所有 clash 类 GUI 的服务模式都有这个问题。
- helper 安装时由 root 拷贝到 `/Library/PrivilegedHelperTools` 并清除 quarantine；没有 Developer ID，app 只做 ad-hoc 签名。

### 1.4 AI 服务检测

AI 服务检测通过内核的 `/clashcube/route` 调用同一个规则匹配器，只分析
TCP/443 域名对应的规则和节点链，不连接 AI 服务域名。IP 类规则可能需要
DNS 解析；没有实际应用连接时，不模拟进程规则、嗅探或 UDP 行为。
自动策略组的结果是当时的选择，不保证后续连接仍选择同一个节点。

出口 IP 统一经对应节点向 Cloudflare 查询，所有 AI 服务共享按内核实例和
节点区分的三分钟缓存，并发请求合并，失败缓存三十秒。手动刷新仍复用
有效的节点出口缓存，只强制刷新 IP 属性。配置重载和网络重置清空出口缓存。
节点若按目标二次分流，AI 服务实际看到的出口可能不同；这里的地区判断
只是根据节点出口地区推断，不代表服务实际可用。

IP 属性使用 IPLocate 的 `/api/lookup/<ip>` 查询已取得的出口地址，不需要
再经该节点访问 AI 服务。属性按 IP 缓存十分钟，失败缓存三十秒；缺失或
限流不影响路由和出口 IP 的展示。`isp` 只显示为运营商，不推断为住宅 IP。

## 2. 数据目录


`~/Library/Application Support/ClashCube/`（开发时用 `CLASHCUBE_HOME` 指到别处）：

```
settings.json          应用设置，含网络规则
profiles/index.json    订阅元数据；profiles/<id>.yaml 每个配置一个文件
modules.json           模块：全局的叠加到每个配置上，带 profile 的只叠加到那个配置
rules.json             用户规则
usage/<日期>.json      流量统计，一天一个文件，保留 90 天
core/                  mihomo 的 home（geo、cache.db、providers）
core/runtime.yaml      实际交给 core 的配置（0600）
```

## 3. runtime 配置生成（`runtimecfg.Build`）

配置文件从不原地修改。每次启动或重载前把覆盖项合并好再交给内核（ClashBar 是先启动再 `PATCH /configs`，中间有状态不一致的窗口期）：

1. 把当前配置解析成 `map[string]any`，未知字段原样保留。
2. 按顺序合并启用的模块：先全局模块，再当前配置自己的模块（`profile` 为其 ID），所以配置模块的规则排在前面。复制配置时连同它的模块一起复制，删除配置时一起删除。写法与 Clash Verge 的扩展配置一致：映射逐键深度合并，其他值替换；`+key` 前插列表，`key+` 追加，`key!` 整体替换；`prepend-/append-rules`、`-proxies`、`-proxy-groups` 作用于规则、节点和策略组。
3. 插入用户规则（在配置自带规则之前）。策略在当前配置里不存在的规则跳过，不让整个配置校验失败。规则类型限定为单值类型，值和策略里不允许逗号和换行。
4. 用应用设置覆盖：`mixed-port`、`allow-lan`、`ipv6`、`unified-delay`、`log-level`、`mode`、`find-process-mode`、`tun`、`external-controller`、`secret`；删除其他控制接口（含 `external-controller-unix`）。模块写了这些键也会被覆盖。「设置 → 网络」的统一延迟默认开启，尽可能复用连接测量第二次请求；关闭后计入首次建连和握手。切换会重载内核，失败则回滚设置。
5. 防泄露开关（默认都关）：
   - **IPv6 流量进入 TUN**：TUN 开、IPv6 关时写 `ipv6: true`、`dns.ipv6: false`。mihomo 在 `ipv6: false` 时去掉 TUN 的 IPv6 地址，auto-route 就不加 IPv6 路由，IPv6 流量会绕过 TUN。core 还设 `SKIP_SYSTEM_IPV6_CHECK=1`。
   - **接管 DNS**：强制 `dns.enable`（没有就补一个 fake-ip 段），TUN 下 `dns-hijack` 改为 `any:53`、`tcp://any:53`。
   - **拦截 UDP STUN**：最前面加 `AND,((NETWORK,UDP),(DST-PORT,3478/5349/19302-19309)),REJECT`。
   - **DNS 查询遵循规则**：`dns.respect-rules: true`，缺 `proxy-server-nameserver` 时用 `nameserver` 补。
6. 写 `core/runtime.yaml`，执行 `core -t` 校验，再启动或热重载（`PUT /configs?force=true`）。校验失败就保留原来的配置；模块、用户规则、设置被拒绝时都回滚到原值并提示内核的错误。

被拒绝的配置连同出错行一起记下（`State.Refusal`），直到下一次通过校验。行号由内核的报错换算：YAML 语法错误取解析器停下的那一行（它报的行是所在块的开头，可能差很远）；`rules[i]`、`proxy i`、策略组名、`proxy-providers`、`sub-rules`、`listeners`、`dns.fake-ip-filter` 等按路径在 `runtime.yaml` 里找到对应节点。「配置 → 合并配置」只读显示 `runtime.yaml`（原样，含 `secret`），以 diff 形式对照配置文件：基准是配置文件经同一编码器重新输出的结果（格式、键序才一致），`runtimecfg.Layers` 逐个来源（模块、设置、应用、我的规则）记下合并后的样子，逐层做行 diff，每处增删归到引入它的来源；`runtime.yaml` 与按当前设置重建的结果不同的部分标为「待重新加载」。有拒绝时可切到被拒绝的版本并定位到出错行。内核停止时，有拒绝的配置文件被修改也会重新校验，修好后提示自动消失。

运行中切换模式、端口等先 `PATCH /configs` 立即生效，同时写回 settings.json。正在使用的配置文件在外部被修改时（3 秒轮询修改时间和大小）热重载；被拒绝的同一份内容不重试。

### 3.1 服务分流模块
`modules.Services`（Google、YouTube、OpenAI、Claude、AI 服务等）存的是 `route`（服务、策略、地区，或按配置 ID 分开的节点名单加关键词），不是 YAML。节点名属于某个订阅，所以名单按配置存；关键词对所有配置生效。生成时：
- 规则前插，再追加一个 `include-all` 的策略组；同名组已存在时改名为 `名称 (ClashCube)`。
- 固定名单里在顶层 `proxies` 中存在的节点直接写进 `proxies`；provider 节点和失效名字用精确匹配的 `filter`（转义，反引号写成 `\x60`，因为 mihomo 用反引号分隔多个 filter）。地区和关键词用 `filter`，不分大小写。
- 组设 `empty-fallback: REJECT`；指定节点却没有任何勾选时生成 `proxies: [REJECT]`。范围为空时拒绝连接，不会悄悄直连。
- 前置节点按配置 ID 保存在 `route.upstream`，直连策略不使用它。启用时为当前模块生成独立节点和 provider 副本，副本通过隐藏的前置策略组拨号；原节点保持不变。前置节点从出口候选中排除，前置消失时隐藏组回退到 `REJECT`。provider 副本保留原有筛选和重命名，在其后添加私有名称和 `dialer-proxy`，HTTP 副本使用独立缓存并按原更新周期刷新。其他 `include-all` 组排除这些私有节点，节点选择界面也不把副本列为新的原始节点。
- 模块按列表从上到下合并，每个的规则都前插，所以后面的优先。通用的“AI 服务”应放在 OpenAI、Claude 上方。

## 4. 运行时行为

- **启动**：加载设置 → 生成并校验 runtime.yaml → 拉起 core（上次是服务模式且 helper 可用时交给 helper）→ 等 `/version` 就绪后打开 WebSocket 流 → 恢复系统代理。启动时如果系统代理指向我们的端口但 core 没在运行，先清掉。
- **退出**：清除本应用设的系统代理；停止 core（SIGTERM，3 秒后 SIGKILL）。
- **崩溃**：状态变为 `crashed`，保留最后 50 行输出，释放系统代理，记事件并通知。
- **系统代理被改写**：每 3 秒用 `scutil --proxy` 检查，连续两次不是我们的端口就判定被改写；此后不再视为归我们管（退出时不清除），提示“重新接管”。
- **换网与唤醒**：同一轮询读主接口和路由器（TUN 的 utun 换成底下的物理接口）；唤醒来自 `SystemDidWake` 及墙钟跳变超过 30 秒。合并防抖 2 秒后清 DNS 缓存（含 fake-ip）、关闭所有连接，必要时在新网络服务上重设系统代理，然后重测连通性。
- **测速**：对齐 zashboard。后端共享 5 个请求槽位并合并重复请求；单节点 5 秒超时；Selector / LoadBalance / Smart 组和 provider 逐节点测（2 秒）；其他组调用内核整组测速（5 秒），以更新健康状态。进度经 `proxy-latency` 事件推送。
- **流量统计**（`backend/usage.go`、`internal/usage`）：每 2 秒取 `/connections` 按连接 ID 求差，按应用（.app 内的辅助进程归到所属应用）、主机、策略、网络归类；总量用内核的 `uploadTotal/downloadTotal` 差值。采样间隔内开始又结束的连接由 `/clashcube/closed`（fork 里 `statistic.Manager.OnLeave` 的回调）补上。每天每维度保留前 200 项，按小时记分布。
- **通知**：只有 Applications 文件夹里的 app 能请求通知授权，与签名无关，所以用 `wails3 task install` 装到 `~/Applications`。被拒绝或是裸二进制时退回 `osascript display notification`（标题和正文作为参数传入，不拼进脚本）。

### 4.1 网络规则与省流量
需求是“在某个网络下流量怎么走”，不是“按 SSID 换配置”。规则存在 `settings.NetworkRules`，不写进 profile。

- **条件**：Wi-Fi SSID（完整、区分大小写）、有线网络、其他网络（兜底，恰好一条）。顺序：SSID → 有线 → 其他。
- **动作**（都可选）：配置、代理组成员、出站模式、系统代理、TUN，按此顺序应用；某步失败记警告，其余照做。内核停止时只改设置。TUN 只在 helper 已装且是当前版本时开，绝不弹密码框。
- **确定性**：其他网络是基准。任何规则设了某项，保存时其他网络没设就自动补上，所以状态只由当前网络决定，离开公司一定回到基准。
- **手动更改**：网络身份（SSID + 主接口 + 路由器）连续两次相同才算稳定，换网时清掉手动更改。手动改了规则管理的某项，只这一项脱离规则，持续到换网、“恢复规则”或编辑规则。
- **权限**：读 SSID 要定位权限，只在用户点“允许访问”时请求；没权限时 Wi-Fi 只能匹配到其他网络。
- **省流量**：跟随系统标记（`NWPathMonitor` 的 `isExpensive` / `isConstrained`，排除 utun）。开启时暂停订阅自动更新和连通性的定时测量。

## 5. 界面

交互参考 Surge for Mac：
- **出站模式**（规则 / 全局 / 直连）对应 mihomo 的 `mode`，主窗口和托盘都放在最显眼处。
- **系统代理**与**增强模式（TUN）**是两个独立开关。托盘图标反映是否真的在接管流量：内核没运行，或运行但两个开关都没开（以及系统代理被改写），都显示描边图标；接管中为实心。
- **托管配置不可编辑**：订阅提供“创建可编辑副本”，或用模块叠加改动。

### 5.1 窗口
- **托盘面板**：宽 440pt，高度跟随内容（220–560），失焦或 Esc 隐藏；高度变化用原生动画，与 CSS 同一条曲线（`glide_darwin.go`）。
- **主窗口**：点关闭只隐藏；页面顶栏可拖动，左侧留出红绿灯。顶栏 Tab：概览、代理、配置、连接、规则、日志。子页面用页头的分段控件切换，可用 `CLASHCUBE_VIEW` / `view#tab` 直接打开（如 `profiles#modules`、`rules#lookup`、`settings#tun`）。
- **Dock 图标**：`LSUIElement`，设置里可选始终显示 / 不显示 / 主窗口打开时显示。

### 5.2 托盘右键菜单
菜单从状态重建（`traymenu.go`），不原地修改；菜单打开期间只原地更新连通质量和活跃应用，其他变化等关闭后再重建。顺序：状态行（仅内核未运行时）、显示主窗口、出站模式、各代理组（第一项“测速”）与“全部测速”、连通质量、活跃应用（固定 5 行）、仪表盘、系统代理 / 增强模式、配置与更新订阅、复制终端代理命令、内核启停、退出。

AppKit 菜单项排不出 Surge 那样紧凑的布局，所以 `traymenu_darwin.go` 给每项配自绘视图（`MBRowView`），自己画勾选、图标、标题、右侧文字或徽标、子菜单箭头和悬停高亮。Go 侧在标题里加标记来描述右侧内容。快捷键挂在隐藏的同名菜单项上（`allowsKeyEquivalentWhenHidden`）。测速不关闭菜单，结果原地替换。

### 5.3 全局快捷键
默认都不设：全局快捷键先注册者得，默认占用会抢走之后启动的应用的快捷键。提供“使用推荐快捷键”（⌃⌥⌘ + P/M/O/S/E），只填没设的项。录制期间注销所有快捷键；拒绝只加 ⇧、只加 ⌥、只加 ⌘ 的组合，以及与其他项、macOS（`CopySymbolicHotKeys`）或其他应用冲突的组合。用 Carbon `RegisterEventHotKey`，不需要辅助功能权限。

### 5.4 视觉
- 颜色、圆角、阴影、缓动都取自 `src/styles/tokens.css`。延迟着色：<200ms 绿，<500ms 琥珀，更高红，超时灰。
- 全局 `user-select:none`、`cursor:default`，更像原生应用。主题在首屏绘制前通过 `data-theme` 设定，避免闪烁。遵循 `prefers-reduced-motion`。
- 不引入路由库、Tailwind 和图表库：Tab 状态在 store 里，面板用 `?mode=panel` 区分；流量曲线是手写 SVG。

## 6. 不做的
网关 / DHCP / 设备管理、Surge Ponte、Snell / MTProto 服务端、Panels 脚本、MitM / Rewrite / 抓包、`#!include`。内核随 app 一起升级，没有单独更新内核的逻辑。
