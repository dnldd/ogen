package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/resource"

	olg "go.opentelemetry.io/otel/sdk/log"
	mtc "go.opentelemetry.io/otel/sdk/metric"
	trc "go.opentelemetry.io/otel/sdk/trace"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
)

var (
	serviceName = "ogen"
)

func setupTracing(ctx context.Context, cfg *Config, res *resource.Resource) func(context.Context) error {
	client := otlptracegrpc.NewClient(otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(cfg.CollectorGRPCURL))
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		log.Fatalf("Creating otel trace exporter: %v", err)
	}

	traceProvider := trc.NewTracerProvider(trc.WithSampler(trc.AlwaysSample()),
		trc.WithBatcher(exporter), trc.WithResource(res))

	otel.SetTracerProvider(traceProvider)

	return traceProvider.Shutdown
}

func setupMetrics(ctx context.Context, cfg *Config, res *resource.Resource) func(context.Context) error {
	exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithInsecure(), otlpmetrichttp.WithEndpoint(cfg.CollectorHTTPURL), otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
	if err != nil {
		log.Fatalf("Creating otel metrics exporter: %v", err)
	}

	meterProvider := mtc.NewMeterProvider(mtc.WithReader(mtc.NewPeriodicReader(exporter)), mtc.WithResource(res))

	err = runtime.Start(runtime.WithMeterProvider(meterProvider), runtime.WithMinimumReadMemStatsInterval(10*time.Second))
	if err != nil {
		log.Fatalf("Collecting runtime metrics: %v", err)
	}

	otel.SetMeterProvider(meterProvider)

	return meterProvider.Shutdown
}

func setupLogging(ctx context.Context, cfg *Config, resource *resource.Resource) func(context.Context) error {
	logExporter, err := otlploghttp.New(ctx, otlploghttp.WithInsecure(), otlploghttp.WithEndpoint(cfg.CollectorHTTPURL), otlploghttp.WithCompression(otlploghttp.GzipCompression))
	if err != nil {
		log.Fatalf("Creating log exporter: %v", err)
	}

	logProvider := olg.NewLoggerProvider(olg.WithProcessor(olg.NewBatchProcessor(logExporter)), olg.WithResource(resource))
	global.SetLoggerProvider(logProvider)

	return logProvider.Shutdown
}

// rollDice generates metrics, logs and traces based on the provided roll.
func rollDice(ctx context.Context, roll int64, cfg *Config, logger *slog.Logger) {
	tracer := otel.Tracer(cfg.ServiceName)
	_, span := tracer.Start(ctx, "dice_roll")
	defer span.End()

	span.SetAttributes(attribute.String("action", "roll"))

	span.SetAttributes(attribute.Int64("roll", roll))
	fmt.Print(roll)

	switch {
	case roll == 0:
		logger.LogAttrs(ctx, slog.LevelInfo, "Rolled zero", slog.Int64("roll", roll))
		time.Sleep(time.Second)
	case roll%2 == 0:
		logger.LogAttrs(ctx, slog.LevelInfo, "Rolled even", slog.Int64("roll", roll))
		time.Sleep(time.Second * time.Duration(roll/2))
	case roll == 1:
		logger.LogAttrs(ctx, slog.LevelInfo, "Rolled one", slog.Int64("roll", roll))
		time.Sleep(time.Second)
	case roll%2 != 0:
		logger.LogAttrs(ctx, slog.LevelInfo, "Rolled odd", slog.Int64("roll", roll))
		time.Sleep(time.Second * time.Duration(roll/2))
	}
}

// generateData uses a pseudo-random dice roll to generate observability metrics and traces.
func generateData(ctx context.Context, cfg *Config, logger *slog.Logger, wg *sync.WaitGroup) {
	defer wg.Done()

	meter := otel.Meter(cfg.ServiceName)

	diceRolls, err := meter.Int64Counter("dice-rolls", metric.WithDescription("Counts the total number of dice rolls"))
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Creating counter", slog.String("name", "dice-rolls"))
		return
	}

	rollFreq := make(map[int64]metric.Int64Counter)
	for idx := int64(0); idx < 11; idx++ {
		counter, err := meter.Int64Counter(fmt.Sprintf("roll-%d-count", idx),
			metric.WithDescription(fmt.Sprintf("Counter the total number of dice rolls for the number %d", idx)))
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "Creating roll counter", slog.String("name", "roll"), slog.Int64("number", idx))
			return
		}

		rollFreq[idx] = counter
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			roll := int64(rand.Intn(10))
			rollDice(ctx, roll, cfg, logger)

			diceRolls.Add(ctx, 1)
			rollFreq[roll].Add(ctx, 1)
		}
	}
}

func setupPprof(cfg *Config) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:    cfg.PprofURL,
		Handler: mux,
	}
}

func servePprof(ctx context.Context, server *http.Server, logger *slog.Logger, wg *sync.WaitGroup) {
	defer wg.Done()

	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.ErrorContext(ctx, "Serving pprof data", slog.String("err", err.Error()))
	}
}

// handleTermination processes context cancellation signals or interrupt signals from the OS.
func handleTermination(ctx context.Context, cancel context.CancelFunc, teardown func(context.Context), wg *sync.WaitGroup) {
	defer wg.Done()

	// Listen for interrupt signals.
	signals := []os.Signal{os.Interrupt}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, signals...)

	// Wait for the context to be cancelled or an interrupt signal.
	for {
		select {
		case <-ctx.Done():
			teardown(ctx)
			return

		case <-interrupt:
			cancel()
		}
	}
}

func main() {
	cfg := &Config{}
	err := loadConfig(cfg, "")
	if err != nil {
		log.Fatalf("Loading configuration: %v", err)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", cfg.ServiceName), attribute.String("library.language", "go")))
	if err != nil {
		log.Fatalf("Creating otel resource: %v", err)
	}

	logCleanup := setupLogging(ctx, cfg, res)
	if err != nil {
		log.Fatalf("Loading configuration: %v", err)
	}

	logger := otelslog.NewLogger(cfg.ServiceName)

	traceCleanup := setupTracing(ctx, cfg, res)
	if err != nil {
		log.Fatalf("Setting up tracing: %v", err)
	}

	meterCleanup := setupMetrics(ctx, cfg, res)
	if err != nil {
		log.Fatalf("Setting up metrics: %v", err)
	}

	ppf := setupPprof(cfg)

	teardown := func(ctx context.Context) {
		_ = ppf.Close()
		logCleanup(ctx)
		traceCleanup(ctx)
		meterCleanup(ctx)
	}

	wg.Add(3)
	go servePprof(ctx, ppf, logger, &wg)
	go handleTermination(ctx, cancel, teardown, &wg)
	go generateData(ctx, cfg, logger, &wg)
	wg.Wait()
}
