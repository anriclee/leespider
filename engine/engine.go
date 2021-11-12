package engine

import (
	"bufio"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/juju/ratelimit"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	imgEngine               *ImageEngine
	outputPath              string
	outlinePath             string
	parseURLWorkersCnt      int
	fetchWorkersCnt         int
	fetchQPS                int
	parseResponseWorkersCnt int
}

func New(output, outline string, fetchWorksCnt, parseResponseWorkersCnt, parseURLWorkersCnt, fetchQPS int, engine *ImageEngine) *Engine {
	return &Engine{
		parseURLWorkersCnt:      parseURLWorkersCnt,
		fetchWorkersCnt:         fetchWorksCnt,
		fetchQPS:                fetchQPS,
		parseResponseWorkersCnt: parseResponseWorkersCnt,
		imgEngine:               engine,
		outputPath:              output,
		outlinePath:             outline,
	}
}

func (g *Engine) Run() {
	fd, err := os.Open(g.outlinePath)
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.TrimSpace(text) != "" {
			task := &CrawlTask{
				urlChan:     make(chan URLJob, 1000000),
				derivedURLs: make(chan URLJob, 1000000),
				textChan:    make(chan TextJob, 1000000),
				respChan:    make(chan RespJob, 1000000),
				site:        text,
				parentDir:   g.outputPath,
				doneChan:    make(chan struct{}),
				engine:      g,
			}
			task.Begin()
		}
	}
	err = scanner.Err()
	if err != nil {
		return
	}
}

type RespJob struct {
	Directory string
	Response  string
}

type TextJob struct {
	Text      string
	FileName  string
	Directory string
}

type ImgJob struct {
	FileName  string
	ImageURL  string
	Directory string
}

type URLJob struct {
	URL       string
	Directory string
}

type CrawlTask struct {
	site        string
	parentDir   string
	textChan    chan TextJob
	respChan    chan RespJob
	urlChan     chan URLJob
	derivedURLs chan URLJob // 页面派生出来的 url
	doneChan    chan struct{}
	engine      *Engine
}

func (t *CrawlTask) Begin() {
	go t.generateURLWork()
	go t.fetchWork()
	go t.parseResponseWork()
	go t.processOutput()
	t.Wait()
}

func (t *CrawlTask) generateURLWork() {
	parts := strings.Split(t.site, ",")
	if len(parts) != 4 {
		log.Printf("invalid format line:%v", t.site)
		return
	}
	directory := t.parentDir + strings.Join([]string{parts[0], parts[1], parts[2]}, "/")
	url := parts[3]
	if url == "" || strings.HasSuffix(url, "/#") {
		return
	}
	t.urlChan <- URLJob{URL: url, Directory: directory}
	for {
		select {
		case job := <-t.derivedURLs:
			t.urlChan <- job
		case <-time.After(10 * time.Second):
			// 超时，说明已经没有派生 url
			log.Println("generate url work done ")
			close(t.urlChan)
			return
		}
	}
}

func (t *CrawlTask) fetchWork() {
	// qps 控制
	limiter := ratelimit.NewBucketWithQuantum(time.Second, int64(t.engine.fetchQPS), int64(t.engine.fetchQPS))
	var wg sync.WaitGroup
	for i := 0; i < t.engine.fetchWorkersCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range t.urlChan {
				limiter.Wait(1)
				content, err := fetchPage(job.URL)
				if err != nil {
					log.Printf("fetchPage  error:%+v", err)
					continue
				}
				t.respChan <- RespJob{
					Response:  content,
					Directory: job.Directory,
				}
			}
		}()
	}
	wg.Wait()
	close(t.respChan)
	log.Println("fetch work done")
}

func (t *CrawlTask) parseResponseWork() {
	var wg sync.WaitGroup
	for i := 0; i < t.engine.parseResponseWorkersCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range t.respChan {
				title, text, nextLink := parseText(job.Response)
				if text == "" && nextLink == "" {
					continue
				}
				if title == "" {
					title = fmt.Sprintf("unkown:%v", time.Now().UnixNano())
				}
				t.textChan <- TextJob{
					Text:      text,
					FileName:  title,
					Directory: job.Directory,
				}
				if nextLink != "" && !strings.HasSuffix(nextLink, "/#") {
					t.derivedURLs <- URLJob{Directory: job.Directory, URL: "http://116.113.96.251:8080/Search/" + nextLink}
				}
				imgURLs := extractImages(job.Response)
				for _, imgURL := range imgURLs {
					if imgURL != "" {
						t.engine.imgEngine.SubmitJob(ImgJob{
							FileName:  title,
							ImageURL:  imgURL,
							Directory: job.Directory,
						}, 60)
					}
				}
			}
		}()
	}
	wg.Wait()
	close(t.textChan)
	close(t.derivedURLs)
	log.Println("parse response work done")
}

func (t *CrawlTask) processOutput() {
	var wg sync.WaitGroup
	for i := 0; i < 1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range t.textChan {
				err := os.MkdirAll(job.Directory, os.ModePerm)
				if err != nil {
					log.Printf("create save directory failed:%+v", err)
				}
				fileName := job.FileName
				text := job.Text
				name := job.Directory + "/" + fileName + ".txt"
				fd, err := os.Create(name)
				if err != nil {
					log.Printf("create  file failed:%+v,filename:%v", err, fileName)
					continue
				}
				wr := bufio.NewWriter(fd)
				_, err = wr.WriteString(text)
				if err != nil {
					log.Printf(" save file failed:%+v,filename:%v,text:%v", err, fileName, text)
				}
				fmt.Println("file saved:", name)
				wr.Flush()
			}
		}()
	}
	wg.Wait()
	close(t.doneChan)
	log.Println("parse output work done")
}

func (t *CrawlTask) Wait() {
	<-t.doneChan
}

func extractImages(content string) []string {
	var imageURLs []string
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		log.Printf("create doc failed:%+v", err)
		return nil
	}
	doc.Find("img").Each(func(_ int, sec *goquery.Selection) {
		imageURL, ok := sec.Attr("src")
		if ok {
			if strings.HasSuffix(imageURL, ".png") ||
				strings.HasSuffix(imageURL, ".jpg") {
				imageURLs = append(imageURLs, imageURL)
			}
		}
	})
	return imageURLs
}

func parseText(s string) (string, string, string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(s))
	if err != nil {
		log.Printf("create doc failed:%+v", err)
		return "", "", ""
	}
	// 标题
	title := strings.TrimSpace(doc.Find("#ContentDiv > h2").Text())
	// 正文
	// #ContentDiv > div:nth-child(8) > table > tbody > tr:nth-child(1) > td:nth-child(2) > p > span > span:nth-child(1)
	var content string
	doc.Find("#ContentDiv").ChildrenFiltered("p").Each(func(i int, selection *goquery.Selection) {
		text := selection.Find("span").Text()
		text = strings.ReplaceAll(text, "\u00a0", " ")
		content += text + "\n"
	})
	// 下页链接
	nextLink, ok := doc.Find("#ContentDiv > div > a:nth-child(2)").Attr("href")
	if ok {
		return title, content, nextLink
	}
	return title, content, ""
}

func fetchPage(url string) (string, error) {
	if url == "" || strings.HasSuffix(url, "/#") {
		return "", nil
	}
	log.Println("begin to fetch page by url", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {
			log.Printf("close body failed:%+v", err)
		}
	}(resp.Body)
	if resp.StatusCode == 200 {
		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrapf(err, "read response body failed")
		}
		return string(content), nil
	}
	return "", errors.Errorf("invalid request,response code:%v,url:%v", resp.StatusCode, url)
}
