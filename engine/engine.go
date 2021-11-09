package engine

import (
	"bufio"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	imgEngine       *ImageEngine
	textChan        chan TextJob
	respChan        chan RespJob
	inputChan       chan string
	outputPath      string
	outlinePath     string
	fetchWorksCnt   int
	parseWorkersCnt int
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

func New(output, outline string, fetchWorksCnt, parseWorkersCnt int, engine *ImageEngine) *Engine {
	return &Engine{
		inputChan:       make(chan string, 10000),
		textChan:        make(chan TextJob, 10000),
		respChan:        make(chan RespJob, 1),
		outputPath:      output,
		outlinePath:     outline,
		fetchWorksCnt:   fetchWorksCnt,
		parseWorkersCnt: parseWorkersCnt,
		imgEngine:       engine,
	}
}

func (e *Engine) Run() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := e.processInput()
		if err != nil {
			log.Printf("err:%+v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		e.buildFetchWorkers()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		e.buildParseWorkers()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		e.processOutput()
	}()

	wg.Wait()
}

func (e *Engine) processInput() error {
	fd, err := os.Open(e.outlinePath)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.TrimSpace(text) != "" {
			e.inputChan <- text
		}
	}
	err = scanner.Err()
	if err != nil {
		return err
	}
	log.Println("input process finished !!!!!!")
	return nil
}

func (e *Engine) buildFetchWorkers() {
	var wg sync.WaitGroup
	for i := 0; i < e.fetchWorksCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for line := range e.inputChan {
				parts := strings.Split(line, ",")
				if len(parts) != 4 {
					log.Printf("invalid format line:%v", line)
					continue
				}
				directory := e.outputPath + strings.Join([]string{parts[0], parts[1], parts[2]}, "/")
				url := parts[3]
				if strings.HasSuffix(url, "/#") {
					break
				}
				content, err := fetchPage(url)
				if err != nil {
					log.Printf("fetchPage  error:%+v", err)
					continue
				}
				e.respChan <- RespJob{
					Response:  content,
					Directory: directory,
				}
				time.Sleep(1 * time.Second)
			}
		}()
	}
	wg.Wait()
}

func (e *Engine) buildParseWorkers() {
	var wg sync.WaitGroup
	for i := 0; i < e.parseWorkersCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range e.respChan {
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
						})
					}
				}
			}
		}()
	}
	wg.Wait()
}

func (e *Engine) processOutput() {
	for {
		select {
		case job, ok := <-e.textChan:
			if ok {
				go func() {
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
						return
					}
					wr := bufio.NewWriter(fd)
					_, err = wr.WriteString(text)
					if err != nil {
						log.Printf(" save file failed:%+v,filename:%v,text:%v", err, fileName, text)
					}
					fmt.Println("file saved:", name)
					wr.Flush()
				}()
			}
		default:

		}
	}
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

func mockFetchPage(url string) (string, error) {
	time.Sleep(time.Duration(rand.Int63n(3)) * time.Second)
	return "", nil
}
