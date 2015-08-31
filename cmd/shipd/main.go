package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/datacratic/goship/ship"
)

func main() {
	address := flag.String("address", ":8080", "address of the web server")
	directory := flag.String("directory", "", "directory location")
	hostname := flag.String("hostname", "", "URL used by clients to reach the server")

	flag.Parse()

	s := &ship.Server{
		Root: *directory,
		Host: *hostname,
	}

	if s.Root == "" {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}

		s.Root = wd
	}

	if s.Host == "" {
		result, err := exec.Command("hostname", "-f").Output()
		if err != nil {
			log.Fatal(err)
		}

		s.Host = "http://" + strings.TrimSpace(string(result)) + *address
	}

	if err := s.Start(); err != nil {
		log.Fatal(err)
	}

	log.Println("installing server at", s.Host)

	err := http.ListenAndServe(*address, nil)
	if err != nil {
		log.Fatal(err)
	}
}
