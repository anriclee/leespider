package cmd

import (
	"context"
	"flag"
	"fmt"
	"github.com/google/subcommands"
)

type OutlineCmd struct {
}

func (o OutlineCmd) Name() string {
	return "outline"
}

func (o OutlineCmd) Synopsis() string {
	return "generate outline of all logs"
}

func (o OutlineCmd) Usage() string {
	return "-outline"
}

func (o OutlineCmd) SetFlags(set *flag.FlagSet) {
	return
}

func (o OutlineCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	fmt.Println("execute outline cmd")
	return subcommands.ExitSuccess
}
