package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	envstruct "code.cloudfoundry.org/go-envstruct"
	"github.com/poy/cf-faas/internal/manifest"
)

func main() {
	log := log.New(os.Stderr, "[Manifest-Parser] ", log.LstdFlags)

	var cfg Config
	if err := envstruct.Load(&cfg); err != nil {
		log.Fatalf("failed to load config: %s", err)
	}

	if cfg.ValidateResolvers {
		for _, name := range cfg.Manifest.EventNames() {
			// http events do not resolve
			if name == "http" {
				continue
			}

			if cfg.ResolverURLs[name] == "" {
				log.Fatalf("event %s does not have a Resolver URL", name)
			}
		}
		return
	}

	fmt.Println(strings.Join(cfg.Manifest.OpenEndpoints(), ","))
}

type Config struct {
	Manifest          manifest.Manifest `env:"MANIFEST, required"`
	ValidateResolvers bool              `env:"VALIDATE_RESOLVERS"`
	ResolverURLs      map[string]string `env:"RESOLVER_URLS"`
}
