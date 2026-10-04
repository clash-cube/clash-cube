package modules

// Template is a module offered ready-made, for the cases most people add
// one for. Hint says in a line what it does and when to want it. A template
// opens in the editor, to be read and changed before it's added.
type Template struct {
	Name string `json:"name"`
	Hint string `json:"hint"`
	Body string `json:"body"`
}

// Templates is the ready-made modules, in the order they're offered.
// Their names and hints are English, translated on the page.
var Templates = []Template{
	{
		Name: "LAN and Apple services direct",
		Hint: "Local names, private networks and Apple's push and update servers skip the proxy and fake-ip",
		Body: `dns:
  +fake-ip-filter:
    - "+.lan"
    - "+.local"
    - "+.home.arpa"
    - "+.push.apple.com"
    - "+.apple.com"
    - "time.*.com"
    - "ntp.*.com"
prepend-rules:
  - DOMAIN-SUFFIX,lan,DIRECT
  - DOMAIN-SUFFIX,local,DIRECT
  - DOMAIN-SUFFIX,push.apple.com,DIRECT
  - IP-CIDR,192.168.0.0/16,DIRECT,no-resolve
  - IP-CIDR,10.0.0.0/8,DIRECT,no-resolve
  - IP-CIDR,172.16.0.0/12,DIRECT,no-resolve
  - IP-CIDR,17.0.0.0/8,DIRECT,no-resolve
`,
	},
	{
		Name: "Chinese DNS for Chinese sites",
		Hint: "Chinese and private domains are looked up with Alibaba and Tencent DNS, the rest with Cloudflare and Google",
		Body: `dns:
  enable: true
  nameserver:
    - https://1.1.1.1/dns-query
    - https://8.8.8.8/dns-query
  nameserver-policy:
    "geosite:cn,private":
      - https://dns.alidns.com/dns-query
      - https://doh.pub/dns-query
  proxy-server-nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
`,
	},
	{
		Name: "Block ads",
		Hint: "Rejects the domains of the category-ads-all list",
		Body: `prepend-rules:
  - GEOSITE,category-ads-all,REJECT
`,
	},
	{
		Name: "Sniff domains",
		Hint: "Reads the real domain from TLS and HTTP, so domain rules match apps that connect by address",
		Body: `sniffer:
  enable: true
  parse-pure-ip: true
  sniff:
    HTTP:
      ports: [80, 8080-8880]
      override-destination: true
    TLS:
      ports: [443, 8443]
    QUIC:
      ports: [443, 8443]
`,
	},
	{
		Name: "Custom hosts",
		Hint: "Answers names you choose with the addresses you give, as /etc/hosts does",
		Body: `hosts:
  router.lan: 192.168.1.1
`,
	},
}
