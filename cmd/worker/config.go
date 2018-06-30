package main

import (
	"encoding/json"
	"log"
	"strings"

	"code.cloudfoundry.org/go-envstruct"
)

type Config struct {
	PoolAddr    string   `env:"POOL_ADDR, required"`
	AppInstance string   `env:"X_CF_APP_INSTANCE, required"`
	AppNames    []string `env:"APP_NAMES, required"`
	HTTPProxy   string   `env:"HTTP_PROXY, required"`
	DataDir     string   `env:"DATA_DIR, required"`

	VcapApplication VcapApplication `env:"VCAP_APPLICATION, required"`
}

type VcapApplication struct {
	CAPIAddr        string   `json:"cf_api"`
	SpaceID         string   `json:"space_id"`
	ApplicationID   string   `json:"application_id"`
	ApplicationURIs []string `json:"application_uris"`
}

func (a *VcapApplication) UnmarshalEnv(data string) error {
	return json.Unmarshal([]byte(data), a)
}

func LoadConfig(log *log.Logger) Config {
	cfg := Config{
		DataDir: "/dev/shm",
	}
	if err := envstruct.Load(&cfg); err != nil {
		log.Fatal(err)
	}

	// Use HTTP so we can use HTTP_PROXY
	cfg.VcapApplication.CAPIAddr = strings.Replace(cfg.VcapApplication.CAPIAddr, "https", "http", 1)

	return cfg
}
