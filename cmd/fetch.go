package cmd

import (
	"context"
	"flag"
	"github.com/google/subcommands"
	"leespider/engine"
	"log"
	"strings"
)

type FetchCmd struct {
	OutlinePath     string
	OutputDirectory string
	FetchWorkersCnt int
	ParseWorkersCnt int
}

func (f *FetchCmd) Name() string {
	return "fetch"
}

func (f *FetchCmd) Synopsis() string {
	return "fetch data from website"
}

func (f *FetchCmd) Usage() string {
	return "-fetch"
}

func (f *FetchCmd) SetFlags(set *flag.FlagSet) {
	set.StringVar(&f.OutlinePath, "outline", "", "outline file path")
	set.StringVar(&f.OutputDirectory, "output", "", "output directory of data")
	set.IntVar(&f.FetchWorkersCnt, "fetchWorkers", 1, "num of fetch workers")
	set.IntVar(&f.ParseWorkersCnt, "parseWorkers", 1, "num of parse response workers")
}

func (f *FetchCmd) Execute(ctx context.Context, flag *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	if f.OutputDirectory == "" || !strings.HasSuffix(f.OutputDirectory, "/") {
		log.Println("output directory illegal",f.OutputDirectory)
		return subcommands.ExitFailure
	}
	if f.OutlinePath == "" {
		log.Println("no outline file")
		return subcommands.ExitFailure
	}
	imgEngine := engine.NewImageEngine()
	go imgEngine.Run()
	e := engine.New(f.OutputDirectory, f.OutlinePath, f.FetchWorkersCnt, f.ParseWorkersCnt, imgEngine)
	e.Run()
	return subcommands.ExitSuccess
}
