package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Hosts    []string      `yaml:"hosts"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	UseICMP  bool          `yaml:"use_icmp"`
}

type HostStats struct {
	Host       string
	Latency    time.Duration
	PacketLoss float64
	Timestamps []time.Time
	RTTs       []time.Duration
	mutex      sync.Mutex
}

var (
	run      = true
	cfg      Config
	showHelp = flag.Bool("help", false, "Show usage instructions")
	config   = flag.String("config", "config.yaml", "Path to YAML config file")
)

func printHelp() {
	fmt.Println(`Usage: go run main.go [OPTIONS]

A real-time network latency visualizer with terminal UI.

Options:
  -config string   Path to YAML config file (default "config.yaml")
  -help            Show this help manual

YAML Config Example:
  hosts:
    - google.com
    - github.com:443
  interval: 1s
  timeout: 1s
  use_icmp: true

Controls:
  q        Quit the graph interface

On exit, latency data is saved to "latency_log.json".
`)
}

func loadConfig(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}
	var c Config
	err = yaml.Unmarshal(data, &c)
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}
	return c
}

func resolveHost(host string) (string, error) {
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", err
	}
	return ips[0].String(), nil
}

func pingICMP(host string, stats *HostStats) {
	ip, err := resolveHost(host)
	if err != nil {
		log.Printf("Resolve error for %s: %v\n", host, err)
		return
	}

	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		log.Printf("ICMP ListenPacket error: %v\n", err)
		return
	}
	defer conn.Close()

	seq := 0
	sent := 0
	received := 0

	for run {
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  seq,
				Data: []byte("PING"),
			},
		}
		seq++
		b, _ := msg.Marshal(nil)
		start := time.Now()
		conn.SetDeadline(time.Now().Add(cfg.Timeout))
		_, err = conn.WriteTo(b, &net.IPAddr{IP: net.ParseIP(ip)})
		sent++
		if err != nil {
			time.Sleep(cfg.Interval)
			continue
		}

		resp := make([]byte, 1500)
		_, _, err = conn.ReadFrom(resp)
		if err == nil {
			received++
			rtt := time.Since(start)
			stats.mutex.Lock()
			stats.RTTs = append(stats.RTTs, rtt)
			stats.Latency = rtt
			stats.Timestamps = append(stats.Timestamps, time.Now())
			stats.PacketLoss = float64(sent-received) / float64(sent) * 100
			stats.mutex.Unlock()
		}
		time.Sleep(cfg.Interval)
	}
}

func pingTCP(host string, stats *HostStats) {
	sent := 0
	received := 0
	for run {
		start := time.Now()
		sent++
		conn, err := net.DialTimeout("tcp", host, cfg.Timeout)
		if err == nil {
			received++
			rtt := time.Since(start)
			conn.Close()
			stats.mutex.Lock()
			stats.RTTs = append(stats.RTTs, rtt)
			stats.Latency = rtt
			stats.Timestamps = append(stats.Timestamps, time.Now())
			stats.PacketLoss = float64(sent-received) / float64(sent) * 100
			stats.mutex.Unlock()
		}
		time.Sleep(cfg.Interval)
	}
}

func displayGraph(stats []*HostStats) {
	if err := termui.Init(); err != nil {
		log.Printf("Graph display error, falling back to CLI: %v", err)
		displayLoop(stats)
		return
	}
	defer termui.Close()

	plot := widgets.NewPlot()
	plot.Title = "Latency (ms)"
	plot.SetRect(0, 0, 100, 20)
	plot.Data = make([][]float64, len(stats))
	plot.AxesColor = termui.ColorWhite
	plot.LineColors[0] = termui.ColorGreen
	plot.Marker = widgets.MarkerBraille

	legend := widgets.NewParagraph()
	legend.Title = "Hosts"
	legend.SetRect(0, 20, 100, 25)
	legend.TextStyle.Fg = termui.ColorCyan

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	uiEvents := termui.PollEvents()
	for run {
		select {
		case <-ticker.C:
			legendText := ""
			for i, s := range stats {
				s.mutex.Lock()
				if len(s.RTTs) > 50 {
					s.RTTs = s.RTTs[len(s.RTTs)-50:]
				}
				slice := make([]float64, len(s.RTTs))
				for j, d := range s.RTTs {
					slice[j] = float64(d.Milliseconds())
				}
				plot.Data[i] = slice
				legendText += fmt.Sprintf("%d. %s\n", i+1, s.Host)
				s.mutex.Unlock()
			}
			legend.Text = legendText
			termui.Render(plot, legend)
		case e := <-uiEvents:
			if e.Type == termui.KeyboardEvent && e.ID == "q" {
				run = false
			}
		}
	}
}

func displayLoop(stats []*HostStats) {
	screen, err := tcell.NewScreen()
	if err != nil {
		log.Printf("Fallback display failed: %v", err)
		return
	}
	screen.Init()
	defer screen.Fini()

	for run {
		screen.Clear()
		drawText(screen, 0, 0, tcell.StyleDefault.Bold(true), "Host", "Latency", "PacketLoss")

		sort.Slice(stats, func(i, j int) bool {
			stats[i].mutex.Lock()
			stats[j].mutex.Lock()
			defer stats[i].mutex.Unlock()
			defer stats[j].mutex.Unlock()
			return stats[i].Latency < stats[j].Latency
		})

		for i, s := range stats {
			s.mutex.Lock()
			line := fmt.Sprintf("%s %v %.1f%%", s.Host, s.Latency, s.PacketLoss)
			drawText(screen, 0, i+2, tcell.StyleDefault, line)
			s.mutex.Unlock()
		}
		screen.Show()
		time.Sleep(1 * time.Second)
	}
}

func drawText(s tcell.Screen, x, y int, style tcell.Style, txt ...string) {
	line := strings.Join(txt, "    ")
	for i, c := range line {
		s.SetContent(x+i, y, c, nil, style)
	}
}

func saveLog(stats []*HostStats) {
	f, err := os.Create("latency_log.json")
	if err != nil {
		log.Printf("Log save error: %v\n", err)
		return
	}
	defer f.Close()

	data := make(map[string][]time.Duration)
	for _, s := range stats {
		s.mutex.Lock()
		data[s.Host] = s.RTTs
		s.mutex.Unlock()
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

func main() {
	flag.Parse()
	if *showHelp {
		printHelp()
		return
	}

	cfg = loadConfig(*config)

	var wg sync.WaitGroup
	allStats := []*HostStats{}

	for _, host := range cfg.Hosts {
		stat := &HostStats{Host: host}
		allStats = append(allStats, stat)
		wg.Add(1)
		go func(h string, s *HostStats) {
			defer wg.Done()
			if cfg.UseICMP {
				pingICMP(h, s)
			} else {
				pingTCP(h, s)
			}
		}(host, stat)
	}

	go displayGraph(allStats)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	run = false

	wg.Wait()
	saveLog(allStats)
}
