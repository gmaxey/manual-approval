package main

import (
	"github.com/cloudbees-io/manual-approval/cmd"
	"log"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
