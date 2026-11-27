package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
)

func main() {
	config := flag.String("config", "", "participant configuration file")
	flag.Parse()
	if *config == "" {
		log.Fatal("-config is required")
	}
	data, err := os.ReadFile(*config)
	if err != nil {
		log.Fatal("cannot read participant configuration")
	}
	var cfg ceremony.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatal("invalid participant configuration")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := ceremony.Serve(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
