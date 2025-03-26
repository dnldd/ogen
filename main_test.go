package main

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/peterldowns/testy/assert"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

func TestGracefulShutdown(t *testing.T) {
	cfg := &Config{
		CollectorGRPCURL: "localhost:4317",
		CollectorHTTPURL: "localhost:4318",
		ServiceName:      "ogen",
		PprofURL:         "localhost:1777",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", cfg.ServiceName), attribute.String("library.language", "go")))
	if err != nil {
		log.Fatalf("Creating otel resource: %v", err)
	}

	traceCleanup := setupTracing(ctx, cfg, res)
	assert.NoError(t, err)

	meterCleanup := setupMetrics(ctx, cfg, res)
	assert.NoError(t, err)

	logCleanup := setupLogging(ctx, cfg, res)
	assert.NoError(t, err)

	var wg sync.WaitGroup

	ppf := setupPprof(cfg)

	teardown := func(ctx context.Context) {
		_ = ppf.Close()
		traceCleanup(ctx)
		meterCleanup(ctx)
		logCleanup(ctx)
	}

	logger := otelslog.NewLogger(cfg.ServiceName)

	// Ensure the generator can be started and terminated gracefully.
	time.AfterFunc(time.Second*5, func() {
		cancel()
		teardown(ctx)
	})

	wg.Add(2)
	go servePprof(ctx, ppf, logger, &wg)
	go generateData(ctx, cfg, logger, &wg)
	wg.Wait()
}
