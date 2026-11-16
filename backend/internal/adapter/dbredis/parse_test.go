package dbredis

import (
	"reflect"
	"testing"
)

func TestParseInfo(t *testing.T) {
	out := "# Server\r\nredis_version:7.2.4\r\nredis_mode:standalone\r\n\r\n" +
		"# Clients\r\nconnected_clients:2\r\n\r\n" +
		"# Memory\r\nused_memory_human:1.5M\r\nmaxmemory_human:0B\r\n"
	got := parseInfo(out)
	if got["server"]["redis_version"] != "7.2.4" {
		t.Fatalf("server.redis_version = %v", got["server"]["redis_version"])
	}
	if got["clients"]["connected_clients"] != "2" {
		t.Fatalf("clients.connected_clients = %v", got["clients"]["connected_clients"])
	}
	if got["memory"]["used_memory_human"] != "1.5M" {
		t.Fatalf("memory.used_memory_human = %v", got["memory"]["used_memory_human"])
	}
	if len(got) != 3 {
		t.Fatalf("sections = %d, want 3", len(got))
	}
}

func TestParseInfoMalformed(t *testing.T) {
	got := parseInfo("stray:before_section\r\n#Srv\r\nk:v\r\nno_colon_line\r\n")
	if len(got) != 1 || got["srv"]["k"] != "v" {
		t.Fatalf("malformed info parse = %#v", got)
	}
}

func TestParseSlowlog(t *testing.T) {
	// 两条：第二条带 addr/name 元数据行。
	out := "14\n1309446981\n15\nping\n" +
		"15\n1309446990\n30\nkeys\n127.0.0.1:58217\nclient-name\n"
	got := parseSlowlog(out)
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2: %#v", len(got), got)
	}
	want := []SlowlogEntry{
		{ID: 14, Timestamp: 1309446981, DurationUs: 15, Command: "ping"},
		{ID: 15, Timestamp: 1309446990, DurationUs: 30, Command: "keys"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseSlowlogEmpty(t *testing.T) {
	if got := parseSlowlog(""); got != nil {
		t.Fatalf("empty slowlog = %#v", got)
	}
}
