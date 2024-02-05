package main

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"

	"github.com/j-keck/arping"
	"github.com/kelseyhightower/envconfig"
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

func wrapHandler(handler http.Handler, ips ...net.IP) func(rw http.ResponseWriter, req *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		for _, ip := range ips {
			_, _, err := arping.Ping(ip)
			if err == arping.ErrTimeout {
				occupancy.WithLabelValues(fmt.Sprintf("%s", ip)).Set(0)
			} else if err != nil {
				slog.Error("failed to ping", "address", ip, "error", err)
			} else {
				occupancy.WithLabelValues(fmt.Sprintf("%s", ip)).Set(1)
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
