package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// dnsForwarder changes only the first dnsmasq instance's upstream options.
// Local records, DHCP, hosts and its LAN listener remain owned by dnsmasq.
// The undo record is persistent: a power loss must not strand DNS on mihomo.
type dnsForwarder struct {
	exec   command
	path   string
	active string
}

type dnsRecord struct {
	Section  string                     `json:"section"`
	Upstream string                     `json:"upstream"`
	Before   map[string]json.RawMessage `json:"before"`
}

func (d *dnsForwarder) uci(method string, args map[string]any) ([]byte, error) {
	args["config"] = "dhcp"
	body, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	return d.exec("ubus", "call", "uci", method, string(body))
}

func (d *dnsForwarder) read(section string) (map[string]json.RawMessage, error) {
	body, err := d.uci("get", map[string]any{"section": section})
	if err != nil {
		return nil, err
	}
	var result struct {
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	var kind string
	if err := json.Unmarshal(result.Values[".type"], &kind); err != nil || kind != "dnsmasq" {
		return nil, errors.New("LAN DNS requires a dnsmasq instance")
	}
	return result.Values, nil
}

func (d *dnsForwarder) load() (dnsRecord, error) {
	var saved dnsRecord
	body, err := os.ReadFile(d.path)
	if err != nil {
		return saved, err
	}
	err = json.Unmarshal(body, &saved)
	return saved, err
}

func (d *dnsForwarder) apply(l lan) error {
	// dnsmasq's server syntax uses #port, not host:port.
	upstream := l.Address + "#1053"
	if d.active == upstream {
		return nil
	}
	saved, err := d.load()
	if os.IsNotExist(err) {
		values, e := d.read("@dnsmasq[0]")
		if e != nil {
			return e
		}
		if e := json.Unmarshal(values[".name"], &saved.Section); e != nil {
			return e
		}
		saved.Before = map[string]json.RawMessage{"server": values["server"], "noresolv": values["noresolv"]}
	} else if err != nil {
		return err
	}
	saved.Upstream = upstream
	body, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	if err := os.WriteFile(d.path+".tmp", body, 0600); err != nil {
		return err
	}
	if err := os.Rename(d.path+".tmp", d.path); err != nil {
		return err
	}
	_, err = d.uci("set", map[string]any{"section": saved.Section, "values": map[string]any{"server": []string{upstream}, "noresolv": "1"}})
	if err != nil {
		return err
	}
	if err := d.reload(); err != nil {
		return err
	}
	d.active = upstream
	return nil
}

func (d *dnsForwarder) reload() error {
	if _, err := d.exec("/etc/init.d/dnsmasq", "reload"); err != nil {
		return err
	}
	return nil
}

func (d *dnsForwarder) restore() error {
	saved, err := d.load()
	if os.IsNotExist(err) {
		d.active = ""
		return nil
	}
	if err != nil {
		return err
	}
	values, err := d.read(saved.Section)
	if err != nil {
		return err
	}
	for _, key := range []string{"server", "noresolv"} {
		var current any
		if len(values[key]) != 0 {
			_ = json.Unmarshal(values[key], &current)
		}
		owned := current == "1" && key == "noresolv"
		if key == "server" {
			list, ok := current.([]any)
			owned = ok && len(list) == 1 && list[0] == saved.Upstream
		}
		// Leave an administrator's intervening upstream edits intact.
		if !owned {
			continue
		}
		before := saved.Before[key]
		if len(before) == 0 || string(before) == "null" {
			_, err = d.uci("delete", map[string]any{"section": saved.Section, "option": key})
		} else {
			var value any
			if err := json.Unmarshal(before, &value); err != nil {
				return err
			}
			_, err = d.uci("set", map[string]any{"section": saved.Section, "values": map[string]any{key: value}})
		}
		if err != nil {
			return err
		}
	}
	// Retry reload even if a prior attempt restored UCI but failed to reload
	// dnsmasq; its running upstream can still point at the stopped core.
	if err := d.reload(); err != nil {
		return err
	}
	if err := os.Remove(d.path); err != nil {
		return err
	}
	d.active = ""
	return nil
}

func dnsReady() error {
	deadline := time.Now().Add(4 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:53", 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("dnsmasq is not listening: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func dnsState(home string) string { return filepath.Join(home, "dns-restore.json") }
