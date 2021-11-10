package main

import (
	"context"
	"flag"
	"github.com/google/subcommands"
	"leespider/cmd"
	"net/http"
	_ "net/http/pprof"
	"os"
)

func main() {
	subcommands.Register(&cmd.OutlineCmd{}, "")
	subcommands.Register(&cmd.FetchCmd{}, "")
	flag.Parse()
	ctx := context.Background()
	go func() {
		err := http.ListenAndServe("0.0.0.0:6060", nil)
		if err != nil {
			panic(err)
		}
	}()
	os.Exit(int(subcommands.Execute(ctx)))
	//eg := engine.NewImageEngine()
	//eg.SubmitJob(engine.ImgJob{
	//	ImageURL:  "admin/edit/attached/image/20210112/20210112141215_1937.png",
	//	Directory: ".",
	//})
	//eg.Run()
}
