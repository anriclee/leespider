package engine

import (
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type ImageEngine struct {
	imgChan chan ImgJob
}

func NewImageEngine() *ImageEngine {
	return &ImageEngine{
		imgChan: make(chan ImgJob, 100000),
	}
}

func (i *ImageEngine) SubmitJob(job ImgJob, seconds int) {
	select {
	case i.imgChan <- job:
	case <-time.After(time.Duration(seconds) * time.Second):
		log.Println("timeout to submit job:", job)
	}
}

func (i *ImageEngine) Run() {
	for job := range i.imgChan {
		time.Sleep(time.Second)
		url := "http://116.113.96.251:8080" + job.ImageURL
		log.Printf("begin to download pic:%v", url)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("err to download image:%+v", err)
			continue
		}
		saveImage(job.Directory, job.FileName, job.ImageURL, resp)
	}
}

func saveImage(directory, jobName, imageURL string, resp *http.Response) {
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("close body failed:%+v", err)
		}
	}(resp.Body)
	splits := strings.Split(imageURL, "/")
	if len(splits) == 0 {
		return
	}
	imageName := splits[len(splits)-1]
	err := os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		log.Printf("create save directory failed:%+v", err)
		return
	}
	fd, err := os.Create(directory + "/" + jobName + imageName)
	if err != nil {
		log.Printf("create file failed:%+v,filename:%v", err, imageName)
		return
	}
	defer fd.Close()
	// Use io.Copy to just dump the response body to the file. This supports huge files
	_, err = io.Copy(fd, resp.Body)
	if err != nil {
		log.Printf("copy file failed:%+v,filename:%v", err, imageName)
	}
}
