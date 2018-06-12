package main

import (
	"encoding/json"
	"log"
	"strings"

	"code.cloudfoundry.org/go-envstruct"
)

type Config struct {
	Port       int    `env:"PORT, required, report"`
	HealthPort int    `env:"PROXY_HEALTH_PORT, report"`
	Manifest   string `env:"MANIFEST_PATH, required, report"`

	VcapApplication VcapApplication `env:"VCAP_APPLICATION, required"`

	SkipSSLValidation bool `env:"SKIP_SSL_VALIDATION, report"`
}

type VcapApplication struct {
	CAPIAddr        string   `json:"cf_api"`
	ApplicationID   string   `json:"application_id"`
	ApplicationURIs []string `json:"application_uris"`
}

func (a *VcapApplication) UnmarshalEnv(data string) error {
	return json.Unmarshal([]byte(data), a)
}

func LoadConfig(log *log.Logger) Config {
	cfg := Config{}
	if err := envstruct.Load(&cfg); err != nil {
		log.Fatal(err)
	}

	// Use HTTP so we can use HTTP_PROXY
	cfg.VcapApplication.CAPIAddr = strings.Replace(cfg.VcapApplication.CAPIAddr, "https", "http", 1)

	envstruct.WriteReport(&cfg)

	return cfg
}
