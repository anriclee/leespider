package main

import (
	"context"
	"flag"
	"github.com/google/subcommands"
	"leespider/cmd"
	"os"
)

func main() {
	subcommands.Register(&cmd.OutlineCmd{}, "")
	subcommands.Register(&cmd.FetchCmd{}, "")
	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
	//eg := engine.NewImageEngine()
	//eg.SubmitJob(engine.ImgJob{
	//	ImageURL:  "admin/edit/attached/image/20210112/20210112141215_1937.png",
	//	Directory: ".",
	//})
	//eg.Run()
}
