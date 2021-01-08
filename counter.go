package main

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nxadm/tail"
)

const viewerLifetime = 10

var streamNameRegex = regexp.MustCompile(`/hls/(?P<streamName>.*)-\d+\.ts`)

type viewCounter struct {
	m               sync.Mutex
	logFile         string
	xmlStatsURL     string
	interval        time.Duration
	exporters       []ViewCountExporter
	streamViewers   map[string]map[string]time.Time
	exportViews     map[string]int
	exportUpdatedAt int64
	http            *http.Client
}

func newViewCounter(logFile string, interval time.Duration, xmlStatsURL string) *viewCounter {
	counter := &viewCounter{
		logFile:       logFile,
		xmlStatsURL:   xmlStatsURL,
		interval:      interval,
		streamViewers: map[string]map[string]time.Time{},
		exportViews:   map[string]int{},
	}

	if counter.xmlStatsURL != "" {
		counter.http = &http.Client{
			Timeout: 1 * time.Second,
		}
	}

	return counter
}

func (v *viewCounter) addExporter(exporter ViewCountExporter) {
	v.exporters = append(v.exporters, exporter)
}

func (v *viewCounter) updateExporters() {
	for _, exporter := range v.exporters {
		exporter.updateViewCount(v.exportViews, v.exportUpdatedAt)
	}
}

func (v *viewCounter) readLines(out chan string) {
	t, err := tail.TailFile(
		v.logFile,
		tail.Config{
			Follow: true,
			ReOpen: true,
			Location: &tail.SeekInfo{
				Whence: io.SeekEnd,
			},
		},
	)
	if err != nil {
		log.Print(err.Error())
		return
	}
	for line := range t.Lines {
		out <- line.Text
	}
}

func (v *viewCounter) processLine(line string) {
	parts := strings.Split(line, " ")
	if len(parts) < 7 {
		return
	}

	ip := parts[0]
	url := parts[6]

	match := streamNameRegex.FindStringSubmatch(url)
	if len(match) == 0 {
		return
	}
	streamName := match[1]

	v.m.Lock()
	defer v.m.Unlock()

	streamViewersPerStream, ok := v.streamViewers[streamName]
	if !ok {
		streamViewersPerStream = map[string]time.Time{}
		v.streamViewers[streamName] = streamViewersPerStream
	}
	streamViewersPerStream[ip] = time.Now()
}

const rtmpStatStreamStartMarker = "<live>"
const rtmpStatStreamEndMarker = "</live>"

var rtmpStreamNameRegex = regexp.MustCompile(`<name>(?P<streamName>.*)</name>`)
var rtmpStreamViewersRegex = regexp.MustCompile(`<nclients>(?P<viewerCount>.*)</nclients>`)

func (v *viewCounter) getRTMPStreamData() map[string]int {
	streamData := map[string]int{}
	if v.xmlStatsURL == "" {
		return streamData
	}

	resp, err := v.http.Get(v.xmlStatsURL)
	if err != nil {
		return map[string]int{}
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := ioutil.ReadAll(resp.Body)

	isStreamList := false
	lastStreamName := ""
	for _, line := range strings.Split(string(body), "\n") {
		if !isStreamList {
			if strings.Contains(line, rtmpStatStreamStartMarker) {
				isStreamList = true
			}
			continue
		} else {
			if strings.Contains(line, rtmpStatStreamEndMarker) {
				return streamData
			}
		}

		match := rtmpStreamNameRegex.FindStringSubmatch(line)
		if len(match) != 0 {
			streamName := match[1]
			streamData[streamName] = 0
			lastStreamName = streamName
			continue
		}

		if lastStreamName != "" {
			match = rtmpStreamViewersRegex.FindStringSubmatch(line)
			if len(match) != 0 {
				viewersStr := match[1]
				viewers, err := strconv.ParseInt(viewersStr, 10, 32)
				if err != nil {
					lastStreamName = ""
					continue
				}
				streamData[lastStreamName] = int(viewers) - 1
				lastStreamName = ""
			}
			continue
		}

	}

	return streamData

}

func (v *viewCounter) countViews() {
	lineChan := make(chan string, 1000)
	go v.readLines(lineChan)

	ticker := time.NewTicker(interval)
	for {
		select {
		case line := <-lineChan:
			v.processLine(line)
		case <-ticker.C:
			v.exportViews = v.getRTMPStreamData()
			for streamName, hlsViewers := range v.streamViewers {
				if _, ok := v.exportViews[streamName]; ok {
					v.exportViews[streamName] += len(hlsViewers)
				} else {
					v.exportViews[streamName] = len(hlsViewers)
				}
			}
			v.exportUpdatedAt = time.Now().Unix()

			v.m.Lock()
			v.streamViewers = removeExpired(v.streamViewers)
			v.m.Unlock()

			v.updateExporters()
		}
	}
}

func removeExpired(viewers map[string]map[string]time.Time) map[string]map[string]time.Time {
	expireTime := time.Now().Add(-viewerLifetime * time.Second)
	result := make(map[string]map[string]time.Time)

	for streamName, streams := range viewers {
		result[streamName] = make(map[string]time.Time)
		for ip, t := range streams {
			if t.After(expireTime) {
				result[streamName][ip] = t
			}
		}
	}

	return result
}
