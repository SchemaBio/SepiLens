package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/pzweuj/sepilens/internal/api"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"github.com/pzweuj/sepilens/internal/secure"
	"github.com/pzweuj/sepilens/internal/ssrf"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
	"github.com/pzweuj/sepilens/internal/validation"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var version = "0.3.0-dev"

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	args := os.Args[1:]
	if len(args) > 1 && args[0] == "rules" && args[1] == "export" {
		fs := flag.NewFlagSet("rules export", flag.ContinueOnError)
		out := fs.String("out", "rules/packages", "output directory")
		if e := fs.Parse(args[2:]); e != nil {
			return e
		}
		return validation.Export(*out)
	}
	if len(args) > 0 && args[0] == "validate" {
		fs := flag.NewFlagSet("validate", flag.ContinueOnError)
		id := fs.String("engine", "", "engine ID")
		cases := fs.String("cases", "", "independently reviewed cases")
		bundle := fs.String("policy", "", "source policy JSON used for independent validation")
		out := fs.String("out", "validation.json", "comparison report")
		if e := fs.Parse(args[1:]); e != nil {
			return e
		}
		return validation.Run(engine.EngineID(*id), *cases, *bundle, *out)
	}
	if len(args) > 0 && args[0] == "rotate-key" {
		cfg, e := config.Load()
		if e != nil {
			return e
		}
		old, e := secure.New(cfg.MasterKey)
		if e != nil {
			return e
		}
		newKey, e := secure.New(os.Getenv("SEPILENS_NEW_MASTER_KEY"))
		if e != nil {
			return e
		}
		db, e := sqlite.Open(cfg.DBPath)
		if e != nil {
			return e
		}
		defer db.Close()
		return secure.Reencrypt(context.Background(), db, old, newKey, false)
	}
	if len(args) > 0 && args[0] == "version" {
		fmt.Println(version)
		return nil
	}
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("sepilens serve", flag.ContinueOnError)
	addr := fs.String("addr", "", "listen address")
	dir := fs.String("data-dir", "", "SQLite directory")
	env := fs.String("env-file", "", "environment file")
	rulesDir := fs.String("rules-dir", "", "signed approval and validation files")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("unsupported command or positional arguments; medical data import has been removed")
	}
	if *env != "" {
		if e := config.LoadDotEnvFile(*env); e != nil {
			return errors.New("environment file unavailable")
		}
	} else {
		config.AutoLoadDotEnv()
	}
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	cfg.Version = version
	if *addr != "" {
		cfg.Addr = *addr
	}
	if *dir != "" {
		cfg.DataDir = *dir
		cfg.DBPath = filepath.Join(*dir, "sepilens-v2.db")
	}
	if cfg.MasterKey == "" {
		return errors.New("SEPILENS_MASTER_KEY is required (base64 32-byte key)")
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	db, e := sqlite.Open(cfg.DBPath)
	if e != nil {
		return errors.New("database initialization failed")
	}
	defer db.Close()
	srv := api.NewServer(cfg, jobs.NewSQLiteStore(db), db, logger)
	if *rulesDir != "" {
		for _, id := range []string{"acmg_amp_2015", "cnv_loss_2020", "cnv_gain_2020"} {
			p := filepath.Join(*rulesDir, id+".approval.json")
			if _, e := os.Stat(p); os.IsNotExist(e) {
				continue
			}
			if e := srv.LoadRuleApproval(p, filepath.Join(*rulesDir, id+".validation.json"), os.Getenv("SEPILENS_APPROVAL_PUBLIC_KEY")); e != nil {
				return errors.New("ruleset approval invalid")
			}
		}
	}
	client := ssrf.NewNCBIRetrievalClient(cfg.RetrievalAllowFakeIP)
	retrievers := []retrieval.Retriever{&retrieval.OfficialRetriever{Database: "pubmed", HTTP: client}, &retrieval.OfficialRetriever{Database: "clinvar", HTTP: client}}
	srv.SetSearchCoordinator(retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, retrievers))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if e = srv.Start(ctx); e != nil {
		return errors.New("encrypted worker initialization failed")
	}
	defer srv.Close()
	server := &http.Server{Addr: cfg.Addr, Handler: srv.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 64 << 10}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	logger.Info("service started", "version", version, "addr", cfg.Addr)
	select {
	case e = <-done:
		if !errors.Is(e, http.ErrServerClosed) {
			return errors.New("HTTP server failed")
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e = server.Shutdown(shutdown); e != nil {
			return errors.New("HTTP shutdown timed out")
		}
	}
	return nil
}
