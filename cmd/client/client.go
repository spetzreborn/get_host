package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/miekg/dns"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/stockholmuniversity/goversionflag"

	gethost "github.com/spetzreborn/get_host/internal"
)

func main() {

	useTracing := flag.Bool("tracing", false, "Enable tracing of calls.")
	useNC := flag.Bool("nc", false, "No Cache. Force reload of cache")
	getAllHosts := flag.Bool("a", false, "Get all hosts")
	configFile := flag.String("configfile", "", "Configuation file")
	goversionflag.PrintVersionAndExit()

	if *configFile == "" {
		log.Fatalln("Need configuration file.")
	}

	config, err := gethost.NewConfig(configFile)
	if err != nil {
		log.Fatalln("Got error when parsing configuration file: " + err.Error())
	}

	var hostToGet string
	flagsLeftover := flag.Args()
	if len(flagsLeftover) > 0 {
		hostToGet = flagsLeftover[0]
	}

	if hostToGet == "" && *getAllHosts == false {
		log.Fatalln("Need part of hostname to match against")
	}

	var tracer opentracing.Tracer
	var closer io.Closer

	if *useTracing == true {
		tracer, closer = gethost.JaegerInit("gethost-client")
		defer closer.Close()
	} else {
		tracer = opentracing.GlobalTracer()
	}
	opentracing.SetGlobalTracer(tracer)

	span := tracer.StartSpan("Get-hosts")
	defer span.Finish()
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	// Server uses hostname/nc to force reload of cache.
	if *useNC == true {
		hostToGet = hostToGet + "/nc"
	}
	r, err := getFromServer(ctx, hostToGet, config)
	if err != nil {
		log.Println(err)
	}
	// No match from server, do lookup ourself
	if r == nil {
		r = getFromDNS(ctx, hostToGet, config)
	}

	for _, i := range r {
		fmt.Println(i)
	}

}

func getFromDNS(ctx context.Context, hostToGet string, config *gethost.Config) []string {
	span, ctx := opentracing.StartSpanFromContext(ctx, "getFromDNS")
	defer span.Finish()

	dnsRR := map[string][]dns.RR{}

	zones := gethost.Zones(config)
	c := make(chan gethost.GetRRforZoneResult)

	for _, s := range zones {
		z := s.Header().Name
		go gethost.GetRRforZone(ctx, z, hostToGet, c, config)
	}

	for range zones {
		m := <-c
		for k, v := range m.SOA.RR {
			dnsRR[k] = v
		}
	}

	keys := []string{}
	for k := range dnsRR {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	return keys

}

func getFromServer(ctx context.Context, hostToGet string, config *gethost.Config) ([]string, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "getFromServer")
	defer span.Finish()

	url := config.ServerURL + ":" + strconv.Itoa(config.ServerPort) + "/hosts/" + hostToGet
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err.Error())
	}

	ext.SpanKindRPCClient.Set(span)
	ext.HTTPUrl.Set(span, url)
	ext.HTTPMethod.Set(span, "GET")
	err := span.Tracer().Inject(
		span.Context(),
		opentracing.HTTPHeaders,
		opentracing.HTTPHeadersCarrier(req.Header),
	)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 2000 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	slice := []string{}
	err = json.Unmarshal(body, &slice)
	if err != nil {
		return nil, err
	}

	return slice, nil
}
