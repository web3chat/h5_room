package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds HTTP server and room configuration.
type ServerConfig struct {
	Listen  string `yaml:"listen"`
	WebDir  string `yaml:"web_dir"`
	TLSCert string `yaml:"tls_cert"` // HTTPS 证书文件路径; 与 TLSKey 都配置时启用 HTTPS
	TLSKey  string `yaml:"tls_key"`  // HTTPS 私钥文件路径
}

// LiveKitConfig holds LiveKit connection and credential configuration.
type LiveKitConfig struct {
	APIBase   string `yaml:"api_base"`
	WSURL     string `yaml:"ws_url"`
	APIKey    string `yaml:"api_key"`
	APISecret string `yaml:"api_secret"`
}

// RedisConfig holds Redis connection and room PIN storage settings.
type RedisConfig struct {
	Addr          string `yaml:"addr"`
	Password      string `yaml:"password"`
	DB            int    `yaml:"db"`
	PinTTLSeconds int64  `yaml:"pin_ttl_seconds"` // 房间 PIN 有效期(秒); 0 = 不过期
}

// Config is the top-level YAML configuration.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	LiveKit LiveKitConfig `yaml:"livekit"`
	Redis   RedisConfig   `yaml:"redis"`
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Listen: ":3000",
			WebDir: "web/dist",
		},
		LiveKit: LiveKitConfig{
			APIBase: "http://127.0.0.1:7880",
			WSURL:   "ws://127.0.0.1:7880",
		},
		Redis: RedisConfig{
			Addr:          "127.0.0.1:6379", // 本项目独立 Redis
			PinTTLSeconds: 0,                // 0 = 房间 PIN 不过期
		},
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, fmt.Errorf("config file not found: %s", path)
		}
		return cfg, fmt.Errorf("read config file: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config file: %w", err)
	}
	return cfg, nil
}

// sanitizeRoom normalizes a room name: lowercase, alphanumeric + underscore only.
func sanitizeRoom(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			out = append(out, c)
		}
	}
	result := string(out)
	if len(result) > 48 {
		result = result[:48]
	}
	return result
}

// tokenTTL is the default LiveKit access token lifetime.
const tokenTTL = 2 * time.Hour
