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
	OutlinePath             string
	OutputDirectory         string
	FetchWorkersCnt         int
	ParseResponseWorkersCnt int
	ParseURLWorkersCnt      int
	FetchQPS                int
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
	set.IntVar(&f.FetchWorkersCnt, "fetchWorkers", 5, "num of fetch web page workers")
	set.IntVar(&f.ParseResponseWorkersCnt, "parseResponseWorkers", 20, "num of parse response workers")
	set.IntVar(&f.ParseURLWorkersCnt, "parseURLWorkers", 20, "num of parse url from input workers")
	set.IntVar(&f.FetchQPS, "fetchQPS", 2, "QPS of fetch web page")
}

func (f *FetchCmd) Execute(ctx context.Context, flag *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	if f.OutputDirectory == "" || !strings.HasSuffix(f.OutputDirectory, "/") {
		log.Println("output directory illegal", f.OutputDirectory)
		return subcommands.ExitFailure
	}
	if f.OutlinePath == "" {
		log.Println("no outline file")
		return subcommands.ExitFailure
	}
	imgEngine := engine.NewImageEngine()
	go imgEngine.Run()
	e := engine.New(f.OutputDirectory, f.OutlinePath, f.FetchWorkersCnt, f.ParseResponseWorkersCnt, f.ParseURLWorkersCnt, f.FetchQPS, imgEngine)
	e.Run()
	return subcommands.ExitSuccess
}
