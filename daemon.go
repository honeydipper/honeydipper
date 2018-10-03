package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

var config Config

func init() {
	flag.Usage = func() {
		fmt.Printf("%v [ -h ] service1 service2 ...\n", os.Args[0])
		fmt.Printf("    Supported services include engie, receiver.\n")
		fmt.Printf("  Note: REPO environment variable is required to specify the bootstrap config.\n")
	}
}

func initEnv() {
	config = Config{initRepo: RepoInfo{}}

	flag.Parse()
	config.services = flag.Args()

	ok := true
	if config.initRepo.repo, ok = os.LookupEnv("REPO"); !ok {
		log.Fatal("REPO environment variable is required to bootstrap honey dipper")
	}
	if config.initRepo.branch, ok = os.LookupEnv("BRANCH"); !ok {
		config.initRepo.branch = "master"
	}
	if config.initRepo.path, ok = os.LookupEnv("BOOTSTRAP_PATH"); !ok {
		config.initRepo.path = "/"
	}

	config.revs = make(map[time.Time]ConfigRev)
}

func start() {
	services := config.services
	if len(services) == 0 {
		services = []string{"engine", "receiver"}
	}
	for _, service := range services {
		switch service {
		case "engine":
			startEngine(config)
		case "receiver":
			startReceiver(config)
		default:
			log.Fatalf("'%v' service is not implemented\n", service)
		}
	}
}

func main() {
	initEnv()
	config.bootstrap()
	start()
	config.watch()
}
