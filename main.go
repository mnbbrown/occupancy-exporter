package main

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/j-keck/arping"
	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus-community/pro-bing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type config struct {
	Port int      `envconfig:"PORT" default:"2113"`
	IPs  []string `envconfig:"IPS"`
}

var (
	// room
	occupancy = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "occupancy_arp",
		Help: "1 if the device responded to a ping, 0 if not",
	}, []string{"ip"})
)

func arpPing(ip net.IP) (bool, error) {
	_, _, err := arping.Ping(ip)
	if err != nil {
		// either timeout, or a worse error
		return false, err
	} else {
		return true, nil
	}
}

func icmpPing(ip net.IP) (bool, error) {
	pinger, err := probing.NewPinger(ip.String())
	if err != nil {
		return false, err
	}
	pinger.Count = 1
	pinger.Timeout = 500 * time.Millisecond
	pinger.OnSend = func(p *probing.Packet) {
		fmt.Println(p)
	}
	slog.Info("ICMP ping")
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		return false, err
	}
	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats
	return stats.PacketsRecv > 0, nil
}

func anybodyHome(ip net.IP) (bool, error) {
	result, err := arpPing(ip)
	if err == nil {
		return result, nil
	}

	slog.Info("falling back to ICMP ping", "error", err)
	return icmpPing(ip)
}

func wrapHandler(handler http.Handler, ips ...net.IP) func(rw http.ResponseWriter, req *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		for _, ip := range ips {
			home, err := anybodyHome(ip)
			if err != nil {
				slog.Warn("No response from ping", "error", err, "ip", ip.String())
				continue
			}

			if home {
				occupancy.WithLabelValues(ip.String()).Set(1)
			} else {
				occupancy.WithLabelValues(ip.String()).Set(0)
			}
		}
		handler.ServeHTTP(rw, req)
	}
}

func main() {
	c := config{}

	err := envconfig.Process("", &c)
	if err != nil {
		log.Fatal(err.Error())
	}

	ips := []net.IP{}
	for _, t := range c.IPs {
		ips = append(ips, net.ParseIP(t))
	}

	promHandler := promhttp.Handler()
	http.HandleFunc("/metrics", wrapHandler(promHandler, ips...))
	addr := fmt.Sprintf(":%d", c.Port)
	slog.Info("listening", "address", addr, "ips", fmt.Sprintf("%s", ips))
	http.ListenAndServe(addr, nil)
}
