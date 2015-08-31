package main

import (
	"flag"
	"log"
	"os"

	"github.com/datacratic/goship/ship"
)

func main() {
	log.SetFlags(0)

	command := flag.String("command", ".", "location of the command package to build")
	version := flag.String("version", "", "use specified version for deployment")
	rollback := flag.Bool("rollback", false, "rollback deployment")
	server := flag.String("server", "$GOBUILDSERVER", "address of the build server")

	flag.Parse()

	if *command == "" {
		log.Fatal("usage: gd options [deploy-list]")
	}

	url := os.ExpandEnv(*server)
	if url == "" {
		log.Fatal("missing build server HTTP address")
	}

	url = "http://" + url

	if *rollback && *version != "" {
		log.Fatal("version is implicit when using --rollback")
	}

	wd, err := os.Getwd()
	if err != nil {
		return
	}

	// create a list of targets from args
	targets := []string{}
	for i, n := 0, flag.NArg(); i < n; i++ {
		arg := flag.Arg(i)
		targets = append(targets, arg)
	}

	// handle rollbacks
	if *rollback {
		if err := ship.RequestRollback(url, *command, wd, targets); err != nil {
			log.Fatal(err)
		}

		return
	}

	h := *version

	// handle new build requests when needed
	if h == "" {
		result, err := ship.RequestBuild(url, *command, wd)
		if err != nil {
			log.Fatal(err)
		}

		h = result
	}

	// deploy?
	if len(targets) == 0 {
		return
	}

	// send deploy requests
	if err := ship.RequestDeploy(url, *command, wd, h, targets); err != nil {
		log.Fatal(err)
	}
}
