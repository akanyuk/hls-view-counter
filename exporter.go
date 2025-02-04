package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"

	"github.com/gorilla/mux"
)

type ViewCountExporter interface {
	updateViewCount(newViews map[string]int, updatedAt int64)
	export(config string)
}

// stdout exporter
type stdoutExporter struct {
}

func (s *stdoutExporter) export(_ string) {
	return
}

func (s *stdoutExporter) updateViewCount(newViews map[string]int, updatedAt int64) {
	fmt.Printf("updatedAt=%d; ", updatedAt)
	for stream, views := range newViews {
		fmt.Printf("stream=%s, views=%d; ", stream, views)
	}
	fmt.Println()
}

// HTTP exporter
type httpViewsData struct {
	Streams   map[string]int `json:"streams"`
	UpdatedAt int64          `json:"updatedAt"`
}

type HttpViewCountExporter struct {
	views []byte
}

func (h *HttpViewCountExporter) updateViewCount(newViews map[string]int, updatedAt int64) {
	httpViews := httpViewsData{
		Streams:   newViews,
		UpdatedAt: updatedAt,
	}
	h.views, _ = json.Marshal(httpViews)
}

func (h *HttpViewCountExporter) export(config string) {
	fmt.Printf("Binding HTTP export on %s\n", config)
	r := mux.NewRouter()
	r.HandleFunc(
		"/views",
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(h.views)
		},
	)
	_ = http.ListenAndServe(config, r)
}

// Collectd exporter

const collectdSocketType = "unix"
const collectdPluginName = "nginx_rtmp"
const collectdDataType = "gauge"
const collectdValueName = "hls_viewers"

type CollectdExporter struct {
	hostname string
	socket   net.Conn
	interval int
}

func (c *CollectdExporter) getHostname() string {
	cmd := exec.Command("/bin/hostname", "-f")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Printf(err.Error())
	}
	fqdn := out.String()
	fqdn = fqdn[:len(fqdn)-1] // removing EOL
	return fqdn
}

func (c *CollectdExporter) export(sockAddr string) {
	var err error
	c.hostname = c.getHostname()
	c.interval = int(interval.Seconds())
	c.socket, err = net.Dial(collectdSocketType, sockAddr)
	if err != nil {
		log.Printf("%s\n", err.Error())
	}
}

func (c *CollectdExporter) updateViewCount(newViews map[string]int, updatedAt int64) {
	for streamName, viewCount := range newViews {
		statLine := fmt.Sprintf(
			"PUTVAL %s/%s-%s/%s-%s interval=%d %d:%d\n",
			c.hostname,
			collectdPluginName, streamName,
			collectdDataType, collectdValueName,
			c.interval,
			updatedAt, viewCount,
		)
		fmt.Println(statLine)
		_, _ = c.socket.Write([]byte(statLine))
	}
}
