package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"code.cloudfoundry.org/go-envstruct"
	"github.com/poy/cf-faas/internal/manifest"
)

type Config struct {
	Port       int `env:"PORT, required, report"`
	HealthPort int `env:"PROXY_HEALTH_PORT, report"`

	ResolverURLs map[string]string `env:"RESOLVER_URLS, report"`

	BootstrapManifest manifest.HTTPManifest `env:"BOOTSTRAP_MANIFEST"`

	Manifest      manifest.Manifest `env:"MANIFEST, required"`
	InstanceIndex int               `env:"CF_INSTANCE_INDEX, required, report"`

	VcapApplication VcapApplication `env:"VCAP_APPLICATION, required"`

	SkipSSLValidation bool `env:"SKIP_SSL_VALIDATION, report"`
}

type VcapApplication struct {
	CAPIAddr        string   `json:"cf_api"`
	ApplicationID   string   `json:"application_id"`
	ApplicationName string   `json:"application_name"`
	SpaceID         string   `json:"space_id"`
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

	for k, v := range cfg.ResolverURLs {
		if len(v) == 0 {
			continue
		}

		if v[0] == '/' {
			cfg.ResolverURLs[k] = fmt.Sprintf("http://localhost:%d%s", cfg.Port, v)
			continue
		}

		cfg.ResolverURLs[k] = "http://" + v
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
