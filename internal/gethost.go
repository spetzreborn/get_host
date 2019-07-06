package gethost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/miekg/dns"

	opentracing "github.com/opentracing/opentracing-go"
	jaeger "github.com/uber/jaeger-client-go"
	config "github.com/uber/jaeger-client-go/config"
)

// SOAwithRR is an data structure for selected dns.RR and corresponding SOA
type SOAwithRR struct {
	SOA *dns.SOA
	RR  map[string][]dns.RR
}

// Config should be populated from an TOML configuration file. Se example.toml in root of this repo.
type Config struct {
	Zones      []string
	NS         string // TODO may be able to use nameserver from configfile
	Resolver   string // TODO may be able to use resolver from configfile
	TTL        int    // Timeout in seconds
	ServerPort int    // Port for server to bind to
	ServerURL  string // Url to server, used by client
	Tracing    bool   // Use jaeger tracing
	Verbose    bool   // Print more verbose information
}

// NewConfig returns default configuration with consideration to configuration file.
func NewConfig(configFile *string) (*Config, error) {
	config := &Config{
		TTL:        900,
		ServerPort: 8080,
		ServerURL:  "http://localhost",
		Tracing:    false,
	}
	if _, err := toml.DecodeFile(*configFile, config); err != nil {
		return nil, errors.New("toml decoding failed: " + err.Error())
	}
	return config, nil
}

// Zones returns a pointer to an slice with dns.SOA RR type for the zones to get AXFR from.
func Zones(config *Config) []dns.SOA {
	var soas = []dns.SOA{}
	zones := config.Zones
	for _, z := range zones {
		isFqdn := dns.IsFqdn(z)
		if isFqdn == false {
			log.Fatalln("zone " + z + " Is not fully qualified. Maybe missing tailing '.'?")
		}
		soa := dns.SOA{}
		soa.Header().Name = z
		soas = append(soas, soa)
	}
	return soas
}

// GetRRforZoneResult is the return struct for GetRRforZone
type GetRRforZoneResult struct {
	SOA SOAwithRR
	Err error
}

// GetRRforZone send all CNAME and A records that match 'hostToGet' over channel c.
// If 'hostToGet' is empty all CNAME and A records for zone z will be returned.
// This function is well suited to be started in parallel as an go routine.
func GetRRforZone(ctx context.Context, zone string, hostToGet string, c chan GetRRforZoneResult, config *Config) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetRRforZone")
	span.SetTag("zone", zone)
	defer span.Finish()

	t := &dns.Transfer{}
	m := &dns.Msg{}
	m.SetAxfr(zone)

	var ns string
	var err error
	if config.NS != "" {
		ns = config.NS
	} else {
		ns, err = GetNSforZone(ctx, zone)
	}
	if err != nil {
		log.Println("GetRRforZone: Got error in NS from GetNSforZone:", err)
		c <- GetRRforZoneResult{Err: err}
		return
	}
	if config.Verbose == true {
		log.Printf("Name server for zone %s: %s\n", zone, ns)
	}

	e, err := t.In(m, ns+":53")
	if err != nil {
		if config.Verbose == true {
			log.Printf("GetRRforZone: Got error for zone %s from %s:%s ", zone, ns, err)
		}
		c <- GetRRforZoneResult{Err: err}
		return
	}

	dnsRR := SOAwithRR{}
	dnsRR.RR = make(map[string][]dns.RR)
	for envelope := range e { // Range read from channel e
		if envelope.Error != nil {
			if config.Verbose == true {
				log.Printf("GetRRforZone: Got error for zone %s from %s:%s", zone, ns, envelope.Error) // TODO: Use RcodeToString to transform rcode int to human readable error?
			}
			c <- GetRRforZoneResult{Err: envelope.Error}
			return
		}
		for _, rr := range envelope.RR { // Iterate over all Resource Records
			name := strings.TrimRight(rr.Header().Name, ".")
			rrtype := rr.Header().Rrtype

			if rrtype == dns.TypeSOA {
				dnsRR.SOA = rr.(*dns.SOA)
			}

			if rrtype == dns.TypeA || rrtype == dns.TypeCNAME { // TODO should we save AAAA records also?
				if hostToGet != "" {
					if strings.Contains(name, hostToGet) {
						tempSlice := dnsRR.RR[name]
						dnsRR.RR[name] = append(tempSlice, rr)
					}
				} else {
					tempSlice := dnsRR.RR[name]
					dnsRR.RR[name] = append(tempSlice, rr)
				}
			}
		}
	}
	ret := GetRRforZoneResult{SOA: dnsRR}
	c <- ret
	if config.Verbose == true {
		log.Println("Done writing zone", zone)
	}
}

//GetNSforZone returns NameServer for zone by doing NS query to the resolver configured in resolv.conf
func GetNSforZone(ctx context.Context, zone string) (ns string, err error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "GetNSforZone")
	span.SetTag("zone", zone)
	defer span.Finish()

	// Get local resolver
	conf, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if conf == nil {
		log.Fatalf("Cannot initialize the local resolver: %s\n", err)
	}

	m := new(dns.Msg)
	m.SetQuestion(zone, dns.TypeNS)

	in, err := dns.Exchange(m, conf.Servers[0]+":"+conf.Port)
	if err != nil {
		return "", err
	}
	if n, ok := in.Answer[0].(*dns.NS); ok {
		ns = n.Ns
	} else {
		return "", errors.New("Did not get any NS record")
	}
	return ns, nil
}

// JaegerInit initialises jaeger object.
func JaegerInit(service string) (opentracing.Tracer, io.Closer) {
	cfg := &config.Configuration{
		Sampler: &config.SamplerConfig{
			Type:  "const",
			Param: 1,
		},
		Reporter: &config.ReporterConfig{
			LogSpans: true,
		},
	}
	tracer, closer, err := cfg.New(service, config.Logger(jaeger.StdLogger))
	if err != nil {
		panic(fmt.Sprintf("ERROR: cannot init Jaeger: %v\n", err))
	}
	return tracer, closer
}
