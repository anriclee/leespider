package engine

import (
	"bufio"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/juju/ratelimit"
	"github.com/pkg/errors"
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
	textChan                chan TextJob
	respChan                chan RespJob
	urlChan                 chan URLJob
	outlineChan             chan string
	inputChan               chan string
	outputPath              string
	outlinePath             string
	parseURLWorkersCnt      int
	fetchWorkersCnt         int
	fetchQPS                int
	parseResponseWorkersCnt int
	doneChan                chan struct{}
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

func New(output, outline string, fetchWorksCnt, parseResponseWorkersCnt, parseURLWorkersCnt, fetchQPS int, engine *ImageEngine) *Engine {
	return &Engine{
		inputChan:               make(chan string, 10000),
		outlineChan:             make(chan string),
		urlChan:                 make(chan URLJob, 1000000),
		textChan:                make(chan TextJob, 1000000),
		respChan:                make(chan RespJob, 10000),
		outputPath:              output,
		outlinePath:             outline,
		parseURLWorkersCnt:      parseURLWorkersCnt,
		fetchWorkersCnt:         fetchWorksCnt,
		fetchQPS:                fetchQPS,
		parseResponseWorkersCnt: parseResponseWorkersCnt,
		imgEngine:               engine,
		doneChan:                make(chan struct{}),
	}
}

func (e *Engine) Run() {
	go e.processOutline()
	go e.parseURLWork()
	go e.fetchWork()
	go e.parseResponseWork()
	go e.processOutput()
	e.Wait()
}

func (e *Engine) Run1() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.processOutline()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		e.parseURLWork()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		e.fetchWork()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		e.parseResponseWork()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		e.processOutput()
	}()

	wg.Wait()
}

func (e *Engine) processOutline() {
	fd, err := os.Open(e.outlinePath)
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.TrimSpace(text) != "" {
			e.outlineChan <- text
		}
	}
	err = scanner.Err()
	if err != nil {
		panic(err)
	}
	close(e.outlineChan)
	log.Println("outline work input done")
}

func (e *Engine) parseURLWork() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case line := <-e.outlineChan:
				if line == "" {
					continue
				}
				parts := strings.Split(line, ",")
				if len(parts) != 4 {
					log.Printf("invalid format line:%v", line)
					continue
				}
				directory := e.outputPath + strings.Join([]string{parts[0], parts[1], parts[2]}, "/")
				url := parts[3]
				if strings.TrimSpace(url) == "" {
					continue
				}
				select {
				case e.urlChan <- URLJob{URL: url, Directory: directory}:
				case <-time.After(1 * time.Minute):
					log.Println("no new outline")
					continue
				}
			case <-time.After(30 * time.Second):
				log.Println("outline work timeout")
				return
			}
		}
	}()

	for i := 0; i < e.parseURLWorkersCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case line := <-e.inputChan:
					parts := strings.Split(line, ",")
					if len(parts) != 4 {
						log.Printf("invalid format line:%v", line)
						continue
					}
					directory := e.outputPath + strings.Join([]string{parts[0], parts[1], parts[2]}, "/")
					url := parts[3]
					if strings.HasSuffix(url, "/#") {
						log.Printf("no more pages,url:%s", url)
						continue
					}
					select {
					case e.urlChan <- URLJob{URL: url, Directory: directory}:
					case <-time.After(1 * time.Minute):
						log.Println("no new input")
						continue
					}
				case <-time.After(30 * time.Second):
					log.Println("input chan work timeout")
					return
				}
			}
		}()
	}
	wg.Wait()
	log.Println("parse url work done ")
}

func (e *Engine) fetchWork() {
	// qps 控制
	limiter := ratelimit.NewBucketWithQuantum(time.Second, int64(e.fetchQPS), int64(e.fetchQPS))
	var wg sync.WaitGroup
	for i := 0; i < e.fetchWorkersCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case job := <-e.urlChan:
					limiter.Wait(1)
					content, err := fetchPage(job.URL)
					if err != nil {
						log.Printf("fetchPage  error:%+v", err)
						continue
					}
					e.respChan <- RespJob{
						Response:  content,
						Directory: job.Directory,
					}
				case <-time.After(30 * time.Second):
					log.Println("fetch work timeout")
					return
				}
			}
		}()
	}
	wg.Wait()
	log.Println("fetch work done")
}

func (e *Engine) parseResponseWork() {
	var wg sync.WaitGroup
	for i := 0; i < e.parseResponseWorkersCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case job := <-e.respChan:
					title, text, nextLink := parseText(job.Response)
					if text == "" && nextLink == "" {
						continue
					}
					if title == "" {
						title = fmt.Sprintf("unkown:%v", time.Now().UnixNano())
					}
					e.textChan <- TextJob{
						Text:      text,
						FileName:  title,
						Directory: job.Directory,
					}
					if nextLink != "" && !strings.HasSuffix(nextLink, "/#") {
						c := strings.TrimLeft(job.Directory, e.outputPath)
						c = strings.ReplaceAll(c, "/", ",") + ",http://116.113.96.251:8080/Search/" + nextLink
						e.inputChan <- c
					}
					imgURLs := extractImages(job.Response)
					for _, imgURL := range imgURLs {
						if imgURL != "" {
							e.imgEngine.SubmitJob(ImgJob{
								FileName:  title,
								ImageURL:  imgURL,
								Directory: job.Directory,
							}, 60)
						}
					}
				case <-time.After(30 * time.Second):
					log.Println("parse response timeout")
					return
				}
			}
		}()
	}
	wg.Wait()
	log.Println("parse response work done")
}

func (e *Engine) processOutput() {
	var wg sync.WaitGroup
	for i := 0; i < 1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case job := <-e.textChan:
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
				case <-time.After(time.Second * 30):
					log.Println("consume text chan time out")
					return
				}
			}
		}()
	}
	wg.Wait()
	log.Println("parse output work done")
	close(e.doneChan)
}

func (e *Engine) Wait() {
	<-e.doneChan
	return
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
			log.Println(imageURL)
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
	log.Println("begin to fetch page by url", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == 200 {
		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrapf(err, "read response body failed")
		}
		return string(content), nil
	}
	return "", errors.Errorf("invalid request,response code:%v,url:%v", resp.StatusCode, url)
}
