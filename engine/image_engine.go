package engine

import (
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type ImageEngine struct {
	imgChan chan ImgJob
}

func NewImageEngine() *ImageEngine {
	return &ImageEngine{
		imgChan: make(chan ImgJob, 100000),
	}
}

func (i *ImageEngine) SubmitJob(job ImgJob) {
	i.imgChan <- job
}

func (i *ImageEngine) Run() {
	for job := range i.imgChan {
		url := "http://116.113.96.251:8080" + job.ImageURL
		log.Printf("begin to download pic:%v", url)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("err to download image:%+v", err)
			continue
		}
		defer resp.Body.Close()
		err = os.MkdirAll(job.Directory, os.ModePerm)
		if err != nil {
			log.Printf("create save directory failed:%+v", err)
			continue
		}
		splits := strings.Split(job.ImageURL, "/")
		if len(splits) == 0 {
			continue
		}
		imageName := splits[len(splits)-1]
		fd, err := os.Create(job.Directory + "/" + job.FileName + imageName)
		if err != nil {
			log.Printf("create file failed:%+v,filename:%v", err, imageName)
			continue
		}
		defer fd.Close()
		// Use io.Copy to just dump the response body to the file. This supports huge files
		_, err = io.Copy(fd, resp.Body)
		if err != nil {
			log.Printf("copy file failed:%+v,filename:%v", err, imageName)
			continue
		}
	}
}
