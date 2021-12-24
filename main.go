package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"golang.org/x/net/html"
)

const (
	UserAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36"
	Referer      = "https://gogoplay1.com/"
	BrightRed    = "\u001b[31;1m"
	BrightGreen  = "\u001b[32;1m"
	BrightYellow = "\u001b[33;1m"
	BrightWhite  = "\u001b[37;1m"
	Reset        = "\u001b[0m"
)

type Episode struct {
	PageLink         string
	DownloadPageLink string
	Resolution       string
	Size             uint64
	AllDownloadLinks map[string]string
}

func (e *Episode) isRes(res string) bool {
	_, exists := e.AllDownloadLinks[res]
	return exists
}

func (e *Episode) SetDownloadPageLink(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "li" && n.FirstChild != nil && n.FirstChild.Data == "a" {
		for _, liAttr := range n.Attr {
			if liAttr.Key == "class" && liAttr.Val == "dowloads" {
				for _, aAttr := range n.FirstChild.Attr {
					if aAttr.Key == "href" {
						e.DownloadPageLink = aAttr.Val
					}
				}
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		e.SetDownloadPageLink(c)
	}
}

func (e *Episode) SetDownloadLinks(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "a" {
		for _, attr := range n.Attr {
			if attr.Key == "download" {
				var quality string
				for _, char := range n.FirstChild.Data {
					if string(char) == "P" {
						break
					}
					charNum, err := strconv.ParseInt(string(char), 10, 64)
					if err != nil {
						continue
					}
					quality += strconv.Itoa(int(charNum))
				}
				e.AllDownloadLinks[quality] = n.Attr[0].Val
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		e.SetDownloadLinks(c)
	}
}

func GetRequest(targetURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("GetRequest(): %v", err)
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Referer", Referer)
	req.Header.Add("Accept", "*/*")

	fetchErr := fmt.Errorf("")
	for tries := 0; tries < 3; tries++ {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			return resp, nil
		}

		fetchErr = err
		fmt.Printf("[%vERROR%v] Failed to fetch URL: %v\n\tRetrying in %v2%v seconds.\n", BrightRed, Reset, targetURL, BrightWhite, Reset)
		time.Sleep(time.Second * 2)
	}

	return nil, fmt.Errorf("GetRequest(): %v", fetchErr)
}

func bytesToMB(size uint64) float64 {
	return float64(size) / float64(1024*1024)
}

type WriteCounter struct {
	Total uint64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.PrintProgress()
	return n, nil
}

func (wc *WriteCounter) PrintProgress() {
	fmt.Printf("\r")
	fmt.Printf("\t[%vDOWNLOADING%v] %.2f MB complete", BrightYellow, Reset, bytesToMB(wc.Total))
}

func DownloadFile(fileName string, resp *http.Response) error {
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	counter := &WriteCounter{}
	_, err = io.Copy(file, io.TeeReader(resp.Body, counter))
	if err != nil {
		return err
	}

	return nil
}

func main() {
	// CLI flags
	urlPtr := flag.String("url", "", "URL of episode page but without episode number.\n(ex: 'https://gogoanime.wiki/dragon-ball-super-dub-episode-' Note that link does not contain\nepisode number at the end )")
	fromPtr := flag.Int("from", 1, "episode number you want to download from.")
	toPtr := flag.Int("to", 1, "episode number you want to download to.")
	flag.Parse()

	// Flag input validation
	u, err := url.Parse(*urlPtr)
	switch {
	case *urlPtr == "":
		log.Fatal(fmt.Sprintf("[%vERROR%v] URL is required.\n", BrightRed, Reset))
	case err != nil, u.Scheme != "https", u.Host != "gogoanime.wiki", string(u.Path[len(u.Path)-1]) != "-":
		log.Fatal(fmt.Sprintf("[%vERROR%v] Invalid URL: %v\n\fpass -h or --help argument for help menu.", BrightRed, Reset, *urlPtr))
	case *fromPtr < 1, *toPtr < *fromPtr:
		log.Fatal(fmt.Sprintf("[%vERROR%v] From: %v and To: %v values don't make sense.\n", BrightRed, Reset, *fromPtr, *toPtr))
	}

	// Log file creation/opening
	logFile, err := os.OpenFile("gogoanime-errors.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(fmt.Sprintf("[%vFATAL%v] Failed to open/create log file.\n%v", BrightRed, Reset, err))
	}
	defer logFile.Close()

	// Log configuration
	log.SetOutput(logFile)
	log.SetPrefix("main(): ")

	// main loop
	for i := *fromPtr; i <= *toPtr; i++ {
		var episode Episode
		episode.PageLink = *urlPtr + strconv.Itoa(i)

		fmt.Printf("[%v+%v] EPISODE %v\n", BrightGreen, Reset, i)
		body, err := GetRequest(episode.PageLink)
		if err != nil {
			log.Print(err)
			fmt.Printf("[%vERROR%v] Failed to fetch URL: %v\n", BrightRed, Reset, episode.PageLink)
			continue
		}
		defer body.Body.Close()
		fmt.Printf("\t[%v*%v] Connection to episode page succeeded.\n", BrightGreen, Reset)

		bodyDoc, _ := html.Parse(body.Body)
		episode.SetDownloadPageLink(bodyDoc)

		dowPgBody, err := GetRequest(episode.DownloadPageLink)
		if err != nil {
			log.Print(err)
			fmt.Printf("[%vERROR%v] Failed to fetch URL: %v\n", BrightRed, Reset, episode.DownloadPageLink)
			continue
		}
		defer dowPgBody.Body.Close()
		fmt.Printf("\t[%v*%v] Connection to download page succeeded.\n", BrightGreen, Reset)

		dowPgBodyDoc, _ := html.Parse(dowPgBody.Body)
		episode.AllDownloadLinks = make(map[string]string)
		episode.SetDownloadLinks(dowPgBodyDoc)

		var dnldLink string
		switch {
		case episode.isRes("1080"):
			episode.Resolution = "1080P"
			dnldLink = episode.AllDownloadLinks["1080"]
		case episode.isRes("720"):
			episode.Resolution = "720P"
			dnldLink = episode.AllDownloadLinks["720"]
		case episode.isRes("480"):
			episode.Resolution = "480P"
			dnldLink = episode.AllDownloadLinks["480"]
		case episode.isRes("360"):
			episode.Resolution = "360P"
			dnldLink = episode.AllDownloadLinks["360"]
		}

		fmt.Printf("\t[%v*%v] Episode Resolution: %v\n", BrightGreen, Reset, episode.Resolution)

		fileResp, err := GetRequest(dnldLink)
		if err != nil {
			log.Print(err)
			fmt.Printf("[%vERROR%v] Failed to fetch URL: %v\n", BrightRed, Reset, dnldLink)
			continue
		}
		defer fileResp.Body.Close()

		episode.Size = uint64(fileResp.ContentLength)
		fmt.Printf("\t[%v*%v] Episode Size: %.2f MB\n", BrightGreen, Reset, bytesToMB(episode.Size))

		if episode.Size == 0 {
			log.Print(fmt.Errorf("content length is zero for episode %v (episode link: %v) (download link: %v)", i, episode.PageLink, dnldLink))
			fmt.Printf("\t[%vERROR%v] Failed to download episode :(\n", BrightRed, Reset)
			continue
		}

		err = DownloadFile(fmt.Sprintf("%v.mp4", i), fileResp)
		if err != nil {
			log.Print(fmt.Errorf("DownloadFile(): Failed to download file %v. %v", fmt.Sprintf("%v.mp4", i), err))
			fmt.Printf("\n\t[%vERROR%v] Failed to download episode :(\n", BrightRed, Reset)
			continue
		}
		fmt.Printf("\n\t[%vDONE%v] Episode download succeeded.\n", BrightGreen, Reset)
	}
}
