package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type HostStats struct {
	Host      string
	Latency   time.Duration
	PacketLoss float64
	Timestamps []time.Time
	RTTs       []time.Duration
	mutex      sync.Mutex
}

var (
	hosts      = []string{"1.1.1.1", "8.8.8.8", "google.com"}
	interval   = 1 * time.Second
	run        = true
)

func resolveHost(host string) (string, error) {
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", err
	}
	return ips[0].String(), nil
}

func ping(host string, stats *HostStats) {
	ip, err := resolveHost(host)
	if err != nil {
		log.Printf("Failed to resolve %s: %v\n", host, err)
		return
	}

	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		log.Fatalf("ListenPacket error: %v", err)
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
		b, err := msg.Marshal(nil)
		if err != nil {
			log.Printf("Marshal error: %v\n", err)
			continue
		}

		start := time.Now()
		_, err = conn.WriteTo(b, &net.IPAddr{IP: net.ParseIP(ip)})
		sent++
		if err != nil {
			log.Printf("WriteTo error: %v\n", err)
			continue
		}

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		resp := make([]byte, 1500)
		n, peer, err := conn.ReadFrom(resp)
		if err != nil {
			stats.mutex.Lock()
			stats.PacketLoss = float64(sent-received) / float64(sent) * 100
			stats.mutex.Unlock()
			time.Sleep(interval)
			continue
		}
		rtt := time.Since(start)

		received++
		m, err := icmp.ParseMessage(1, resp[:n])
		if err == nil && m.Type == ipv4.ICMPTypeEchoReply {
			stats.mutex.Lock()
			stats.RTTs = append(stats.RTTs, rtt)
			stats.Timestamps = append(stats.Timestamps, time.Now())
			stats.Latency = rtt
			stats.PacketLoss = float64(sent-received) / float64(sent) * 100
			stats.mutex.Unlock()
			log.Printf("Ping %s (%s): %v from %s\n", host, ip, rtt, peer.String())
		}
		time.Sleep(interval)
	}
}

func displayLoop(allStats []*HostStats) {
	for run {
		fmt.Print("\033[H\033[2J") // clear screen
		fmt.Println("Real-time Network Latency Visualizer")
		fmt.Println(strings.Repeat("=", 40))

		sort.Slice(allStats, func(i, j int) bool {
			allStats[i].mutex.Lock()
			allStats[j].mutex.Lock()
			defer allStats[i].mutex.Unlock()
			defer allStats[j].mutex.Unlock()
			return allStats[i].Latency < allStats[j].Latency
		})

		for _, s := range allStats {
			s.mutex.Lock()
			fmt.Printf("%s | Latency: %v | Loss: %.1f%%\n", s.Host, s.Latency, s.PacketLoss)
			s.mutex.Unlock()
		}

		time.Sleep(2 * time.Second)
	}
}

func saveLog(stats []*HostStats) {
	f, err := os.Create("latency_log.json")
	if err != nil {
		log.Printf("Error saving log: %v\n", err)
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
	var wg sync.WaitGroup
	allStats := []*HostStats{}

	for _, host := range hosts {
		stat := &HostStats{Host: host}
		allStats = append(allStats, stat)
		wg.Add(1)
		go func(h string, s *HostStats) {
			defer wg.Done()
			ping(h, s)
		}(host, stat)
	}

	go displayLoop(allStats)

	// Handle CTRL+C
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("\nExiting...")
	run = false

	wg.Wait()
	saveLog(allStats)
}
