package config

import "testing"

func TestNewConfigDefaultsRedisToLocalhost(t *testing.T) {
	t.Setenv("REDIS_HOST", "")
	t.Setenv("REDIS_PORT", "")
	t.Setenv("REDIS_DB", "")
	t.Setenv("REMOTE_HOSTS", "")

	conf := NewConfig()
	if conf.RedisHost != "localhost" {
		t.Fatalf("expected redis host localhost, got %s", conf.RedisHost)
	}
	if conf.RedisPort != "6379" {
		t.Fatalf("expected redis port 6379, got %s", conf.RedisPort)
	}
	if len(conf.RemoteHosts) != 0 {
		t.Fatalf("expected no remote hosts, got %v", conf.RemoteHosts)
	}
}

func TestNewConfigParsesRemoteHosts(t *testing.T) {
	t.Setenv("REMOTE_HOSTS", " worker-a:30031 ; ;worker-b:30031 ")

	conf := NewConfig()
	if len(conf.RemoteHosts) != 2 {
		t.Fatalf("expected 2 remote hosts, got %v", conf.RemoteHosts)
	}
	if conf.RemoteHosts[0] != "worker-a:30031" || conf.RemoteHosts[1] != "worker-b:30031" {
		t.Fatalf("unexpected remote hosts: %v", conf.RemoteHosts)
	}
}
