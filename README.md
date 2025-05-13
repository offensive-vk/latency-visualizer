# 🌐 Network Latency Visualizer

A full-fledged terminal-based tool to monitor real-time network latency and packet loss across multiple hosts. Supports ICMP and TCP ping modes, YAML config, live terminal graphs (`termui`), and logs results to JSON.

<!-- ![screenshot](https://user-images.githubusercontent.com/placeholder/latency-visualizer-demo.gif) -->

---

## 🚀 Features

* 📊 **Real-time latency graphs** in terminal
* 🌍 Supports **ICMP (ping)** and **TCP fallback**
* 🧠 Smart **packet loss tracking**
* ⚙️ **YAML configuration** for easy customization
* 📁 Saves **latency logs to JSON**
* 📉 Live sorted display by latency
* 🖥️ Minimal dependencies, portable, fast

---

## 🛠️ Installation

```bash
go build -o latency-visualizer
```

---

## 🧾 Configuration

Create a `config.yaml` in the same directory:

```yaml
hosts:
  - "8.8.8.8:53"
  - "1.1.1.1:53"
  - "google.com:80"
interval: 1s         # Time between pings
timeout: 2s          # Timeout for each ping
use_icmp: false      # Set true to use ICMP, false for TCP
```

---

## 📈 Usage

```bash
./latency-visualizer
```

* Press `q` to quit.
* Logs saved to `latency_log.json` on exit.

---

## 📦 Dependencies

* [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3)
* [golang.org/x/net/icmp](https://pkg.go.dev/golang.org/x/net/icmp)
* [github.com/gizak/termui/v3](https://github.com/gizak/termui)

Install dependencies:

```bash
go get github.com/gizak/termui/v3 gopkg.in/yaml.v3 golang.org/x/net/icmp
```

---

## 📊 Roadmap

* [x] Terminal graph UI
* [ ] Web dashboard (with Chart.js)
* [ ] CSV export
* [ ] Prometheus metrics endpoint

---

## 📄 License

MIT License
