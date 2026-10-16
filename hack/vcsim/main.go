// Command vcsim runs a simulated vCenter (govmomi) advertising a given version,
// to test the exporter without any infrastructure (e.g. -version 8.0.3 -api 8.0.3.0).
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/simulator/vpx"
)

func main() {
	version := flag.String("version", "9.0.0", "simulated vCenter version")
	api := flag.String("api", "9.0.0.0", "simulated vim25 API version")
	addr := flag.String("l", "127.0.0.1:8989", "listen address")
	flag.Parse()
	m := simulator.VPX()
	m.Datacenter, m.Cluster, m.ClusterHost, m.Host, m.Machine, m.Portgroup, m.Pod = 1, 2, 3, 1, 4, 2, 1
	sc := vpx.ServiceContent
	a := sc.About
	a.Version, a.ApiVersion, a.FullName = *version, *api, "VMware vCenter Server "+*version+" (simulated)"
	sc.About = a
	m.ServiceContent = sc
	if err := m.Create(); err != nil {
		panic(err)
	}
	m.Service.TLS = new(tls.Config)
	m.Service.Listen = mustURL(*addr)
	s := m.Service.NewServer()
	fmt.Println(s.URL.String())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	s.Close()
}

func mustURL(addr string) *url.URL {
	u, err := url.Parse("https://" + addr)
	if err != nil {
		panic(err)
	}
	return u
}
