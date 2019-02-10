// Package honeydipper is an event-driven, rule based orchestration platform tailor towards
// DevOps and system engineering workflows.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/honeyscience/honeydipper/internal/config"
	"github.com/honeyscience/honeydipper/internal/daemon"
	"github.com/honeyscience/honeydipper/internal/service"
	"github.com/honeyscience/honeydipper/pkg/dipper"
)

var cfg config.Config

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%v [ -h ] service1 service2 ...\n", os.Args[0])
		fmt.Printf("    Supported services include engie, receiver.\n")
		fmt.Printf("  Note: REPO environment variable is required to specify the bootstrap config.\n")
	}
}

func initEnv() {
	initFlags()
	flag.Parse()
	cfg = config.Config{InitRepo: config.RepoInfo{}, Services: flag.Args()}

	getLogger()
	var ok bool
	if cfg.InitRepo.Repo, ok = os.LookupEnv("REPO"); !ok {
		dipper.Logger.Fatal("REPO environment variable is required to bootstrap honeydipper")
	}
	if cfg.InitRepo.Branch, ok = os.LookupEnv("BRANCH"); !ok {
		cfg.InitRepo.Branch = "master"
	}
	if cfg.InitRepo.Path, ok = os.LookupEnv("BOOTSTRAP_PATH"); !ok {
		cfg.InitRepo.Path = "/"
	}
}

func start() {
	getLogger()
	services := cfg.Services
	if len(services) == 0 {
		services = []string{"engine", "receiver", "operator"}
	}
	for _, s := range services {
		switch s {
		case "engine":
			service.StartEngine(&cfg)
		case "receiver":
			service.StartReceiver(&cfg)
		case "operator":
			service.StartOperator(&cfg)
		default:
			dipper.Logger.Fatalf("'%v' service is not implemented", s)
		}
	}
}

func reload() {
	getLogger()
	for _, service := range service.Services {
		go service.Reload()
	}
}

func getLogger() {
	levelstr, ok := cfg.GetDriverDataStr("daemon.loglevel")
	if !ok {
		levelstr = "INFO"
	}
	dipper.Logger = nil
	_ = dipper.GetLogger("daemon", levelstr)
}

func main() {
	initEnv()
	cfg.OnChange = reload
	daemon.OnStart = start
	daemon.Run(&cfg)
}
