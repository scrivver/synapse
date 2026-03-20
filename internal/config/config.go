package config

import (
	"net/url"
	"os"
	"time"
)

type Config struct {
	RabbitMQURL        string
	S3Endpoint         string
	S3AccessKey        string
	S3SecretKey        string
	S3UseSSL           bool
	HotBucket          string
	ColdBucket         string
	EngramAPIURL       string
	ReconcileInterval  time.Duration
	MaxRetries         int
	StorageBackend     string
	StorageFSRoot      string
	MetadataFile       string
	EventLogFile       string
	EngramAMQPURL      string
	EngramExchange     string
	EngramRoutingKey   string
}

func Load() Config {
	s3Endpoint := envOr("S3_ENDPOINT", "http://127.0.0.1:9000")
	host, useSSL := parseEndpoint(s3Endpoint)

	interval, err := time.ParseDuration(envOr("RECONCILE_INTERVAL", "30s"))
	if err != nil {
		interval = 30 * time.Second
	}

	return Config{
		RabbitMQURL:       envOr("RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672"),
		S3Endpoint:        host,
		S3AccessKey:       envOr("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:       envOr("S3_SECRET_KEY", "minioadmin"),
		S3UseSSL:          useSSL,
		HotBucket:         envOr("S3_HOT_BUCKET", "synapse-hot"),
		ColdBucket:        envOr("S3_COLD_BUCKET", "synapse-cold"),
		EngramAPIURL:      os.Getenv("ENGRAM_API_URL"),
		ReconcileInterval: interval,
		MaxRetries:        5,
		StorageBackend:    envOr("STORAGE_BACKEND", "s3"),
		StorageFSRoot:     envOr("STORAGE_FS_ROOT", ".data/storage"),
		MetadataFile:      envOr("METADATA_FILE", ".data/metadata.json"),
		EventLogFile:      envOr("EVENT_LOG_FILE", ".data/events.log"),
		EngramAMQPURL:     os.Getenv("ENGRAM_AMQP_URL"),
		EngramExchange:    envOr("ENGRAM_EXCHANGE", "engram.events"),
		EngramRoutingKey:  envOr("ENGRAM_ROUTING_KEY", "synapse.move"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseEndpoint extracts host:port and SSL flag from an endpoint URL.
// Accepts both "http://host:port" and bare "host:port".
func parseEndpoint(raw string) (host string, useSSL bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw, false
	}
	return u.Host, u.Scheme == "https"
}
