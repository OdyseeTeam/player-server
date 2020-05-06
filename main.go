package main

import (
	"github.com/google/gops/agent"
	"github.com/lbryio/lbrytv-player/cmd"
)

func main() {
	if err := agent.Listen(agent.Options{}); err != nil {
		panic(err)
	}
	cmd.Execute()
}
