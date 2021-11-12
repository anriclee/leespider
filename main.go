package main

import (
	"context"
	"flag"
	"github.com/google/subcommands"
	"leespider/cmd"
	_ "net/http/pprof"
	"os"
)

func main() {
	subcommands.Register(&cmd.OutlineCmd{}, "")
	subcommands.Register(&cmd.FetchCmd{}, "")
	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
