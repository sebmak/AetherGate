package config

import (
	"os"
	"path/filepath"
	"strconv"
)

// Version is the current application release version
var Version = "2.1.0"

// AppConfig holds global application configuration.
// Rules: All port numbers must be positive and non-conflicting; DataDir, CertDir, and DBPath must be valid paths.
type AppConfig struct {
	AdminPort  int
	HTTPPort   int
	HTTPSPort  int
	DataDir    string
	CertDir    string
	ConfigPath string
	DBPath     string
}

// LoadConfig loads configuration from environment variables with sensible defaults
func LoadConfig() *AppConfig {
	dataDir := getEnv("DATA_DIR", "./data")
	if !filepath.IsAbs(dataDir) {
		cwd, err := os.Getwd()
		if err == nil {
			dataDir = filepath.Join(cwd, dataDir)
		}
	}

	certDir := getEnv("CERT_DIR", filepath.Join(dataDir, "certs"))
	configPath := getEnv("CONFIG_PATH", filepath.Join(dataDir, "config.json"))
	dbPath := getEnv("DB_PATH", filepath.Join(dataDir, "aethergate.db"))

	adminPort := getEnvInt("ADMIN_PORT", 8080)
	httpPort := getEnvInt("HTTP_PORT", 9080)
	httpsPort := getEnvInt("HTTPS_PORT", 9443)

	return &AppConfig{
		AdminPort:  adminPort,
		HTTPPort:   httpPort,
		HTTPSPort:  httpsPort,
		DataDir:    dataDir,
		CertDir:    certDir,
		ConfigPath: configPath,
		DBPath:     dbPath,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
