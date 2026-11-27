package main

import (
	"context"
	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/records"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	worker, err := records.OpenErasureWorker(ctx, os.Getenv("P09_ERASURE_WORKLOAD_DATABASE_URL"), os.Getenv("P09_ERASURE_WRITE_DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer worker.Close()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for ctx.Err() == nil {
		if err := worker.Step(ctx); err != nil {
			log.Print("erasure remains pending")
		}
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
}
