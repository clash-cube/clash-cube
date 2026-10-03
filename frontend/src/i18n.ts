import { useStore } from "./store";

// Chinese, keyed by the English: a missing entry shows the
// English rather than a key.
const zh: Record<string, string> = {
  "Overview": "概览", "Proxies": "代理", "Profiles": "配置", "Connections": "连接", "Rules": "规则", "Logs": "日志", "Settings": "设置",
  "Rule": "规则", "Global": "全局", "Direct": "直连",
  "Running": "运行中", "Stopped": "已停止", "Starting…": "正在启动…", "Stopping…": "正在停止…", "Restarting…": "正在重启…", "Reloading…": "正在重载…", "Error": "出错",
  "Start": "启动", "Stop": "停止", "Restart": "重启", "Start core": "启动内核", "Restart core": "重启内核", "Stop core": "停止内核",
  "Upload": "上传", "Download": "下载", "Memory": "内存", "Total": "累计",
  "System Proxy": "系统代理", "Enhanced Mode": "增强模式", "Outbound Mode": "出站模式",
  "Route apps that respect the macOS proxy settings": "接管遵循系统代理设置的应用",
  "TUN: capture all traffic, including terminals and games": "TUN：接管所有流量，包括终端与游戏",
  "Service mode": "服务模式", "Privileged helper": "特权助手",
  "Installed": "已安装", "Not installed": "未安装", "Needs update": "需要更新", "Install": "安装", "Install and turn on TUN": "安装并开启 TUN", "Uninstall": "卸载",
  "Runs the core as root through a LaunchDaemon, which TUN needs. macOS asks for an administrator password once.": "通过 LaunchDaemon 以 root 身份运行内核（TUN 需要）。安装时 macOS 会要求输入一次管理员密码。",
  "Core runs as root": "内核以 root 运行", "Core runs as you": "内核以当前用户运行",
  "The root core reads its configuration from your user folder, so programs running as you can influence it.": "root 内核从你的用户目录读取配置，因此以你的身份运行的程序可以影响它。",
  "Installs a privileged helper on first use": "首次开启时会安装特权助手",
  "Current profile": "当前配置", "Open Dashboard": "打开主界面", "Quit": "退出",
  "Top Clients": "活跃应用", "No active apps": "暂无活跃应用",
  "Test": "测速", "Testing…": "测速中…", "Test all": "全部测速", "Sort by latency": "按延迟排序", "Default order": "默认顺序",
  "Core is not running": "内核未运行", "Start the core to see proxies.": "启动内核后即可查看代理。",
  "No proxy groups": "没有代理组", "This profile has no proxy groups.": "此配置没有代理组。",
  "Import": "导入", "Import from URL": "从 URL 导入", "Import a file…": "导入本地文件…", "Subscription URL": "订阅链接", "Name (optional)": "名称（可选）",
  "Auto update": "自动更新", "Never": "从不", "Every {n}h": "每 {n} 小时", "Cancel": "取消", "Update": "更新", "Update all": "全部更新", "Updating…": "正在更新…",
  "Use": "使用", "In use": "使用中", "Local file": "本地文件", "Updated {t}": "更新于 {t}", "Expires {d}": "{d} 到期", "Expired": "已过期",
  "Copy URL": "复制链接", "Show in Finder": "在 Finder 中显示", "Open in editor": "在编辑器中打开", "Rename": "重命名", "Remove": "删除", "Click again to remove": "再次点击以删除",
  "Imported {name}": "已导入 {name}", "Updated {name}": "已更新 {name}", "Switched to {name}": "已切换到 {name}", "Removed": "已删除", "Copied": "已复制",
  "just now": "刚刚", "{n}m ago": "{n} 分钟前", "{n}h ago": "{n} 小时前", "{n}d ago": "{n} 天前",
  "Search": "搜索", "Close all": "全部关闭", "No connections": "没有连接", "{n} connections": "{n} 个连接", "Host": "主机", "Chain": "链路", "Time": "时长",
  "No rules": "没有规则", "{n} rules": "{n} 条规则",
  "Clear": "清空", "Paused": "已暂停", "No logs yet": "暂无日志", "Logs appear here as the core writes them.": "内核输出的日志会显示在这里。",
  "General": "通用", "Network": "网络", "Appearance": "外观", "About": "关于", "Core": "内核",
  "Start core when the app opens": "打开应用时启动内核", "Open at login": "开机启动", "Show in Dock": "在 Dock 中显示",
  "Always": "始终", "While the window is open": "窗口打开时", "Show speed in the menu bar": "在菜单栏显示速率",
  "Theme": "主题", "System": "跟随系统", "Light": "浅色", "Dark": "深色", "Language": "语言",
  "Mixed port": "混合端口", "Allow LAN": "允许局域网连接", "IPv6": "IPv6", "Log level": "日志级别", "Latency test URL": "测速链接",
  "Bypass": "绕过代理", "One host or network per line": "每行一个主机或网段", "Save": "保存", "Saved": "已保存",
  "TUN stack": "TUN 协议栈", "ICMP forwarding": "ICMP 转发",
  "Pings go out directly, never through a proxy. Off: the core answers every ping itself. Pinging a domain under fake-ip always gets a local answer.": "ping 直连发出，不经过代理。关闭后由内核直接回复所有 ping。fake-ip 下 ping 域名得到的始终是本地回复。",
  "Flush DNS cache": "清除 DNS 缓存", "Update GEO databases": "更新 GEO 数据库", "Copy shell export command": "复制终端代理命令",
  "Open data folder": "打开数据目录", "Version": "版本", "mihomo": "mihomo 内核",
  "Core stopped with an error": "内核异常退出", "Show logs": "查看日志", "Dismiss": "关闭",
  "Profile": "配置", "Router": "路由器", "Internet": "互联网", "Proxy": "代理", "Failed": "失败", "Identify processes": "识别进程", "Show which app made each connection": "显示每个连接来自哪个应用", "test this node only": "只测该节点", "⌥-click: use this Mac's LAN address": "⌥ 点击：使用本机局域网地址", "Profile order": "配置顺序", "Most hits": "命中最多", "Last hit {t}": "最近命中 {t}", "Process": "进程", "List": "列表", "Path": "路径", "Source": "来源", "Copy host": "复制主机", "Close connection": "关闭连接", "Double-click to copy": "双击复制", "System resolver": "系统 DNS", "Click to show the interface and egress IP": "点击显示网卡和出口 IP", "Looking up…": "正在查询…", "Domestic": "国内", "Overseas": "海外", "Overseas traffic leaves elsewhere: something upstream, such as the router, proxies it": "海外流量从别处出口：上游（如路由器）有代理", "Copy": "复制", "Mode": "模式", "Speed": "速率",
  "Click to show the upstream and egress IP": "点击显示上游和出口 IP", "Egress": "出口", "The address authoritative servers see the queries come from": "权威服务器看到的查询来源地址",
  "Click to show the egress IP": "点击显示出口 IP", "Looked up through {p}, not {q} that the test URL takes": "此次查询经过 {p}，而非测速链接所走的 {q}",
};

export type Vars = Record<string, string | number>;

function lang(setting: string | undefined): "zh" | "en" {
  if (setting === "zh" || setting === "en") return setting;
  return navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en";
}

export function translate(s: string, vars?: Vars, l?: "zh" | "en"): string {
  let out = (l ?? lang(useStore.getState().settings?.lang)) === "zh" ? zh[s] ?? s : s;
  if (vars) for (const k in vars) out = out.split("{" + k + "}").join(String(vars[k]));
  return out;
}

// useT is t() for a component, which then redraws on a change of language.
export function useT() {
  const l = lang(useStore((s) => s.settings?.lang));
  return (s: string, vars?: Vars) => translate(s, vars, l);
}
