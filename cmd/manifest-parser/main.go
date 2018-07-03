package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	envstruct "code.cloudfoundry.org/go-envstruct"
	"github.com/apoydence/cf-faas/internal/manifest"
)

func main() {
	log := log.New(os.Stderr, "[Manifest-Parser] ", log.LstdFlags)

	var cfg Config
	if err := envstruct.Load(&cfg); err != nil {
		log.Fatalf("failed to load config: %s", err)
	}

	fmt.Println(strings.Join(cfg.Manifest.OpenEndpoints(), ","))
}

type Config struct {
	Manifest manifest.Manifest `env:"MANIFEST, required"`
}
