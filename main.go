package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	redis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type RedisClient = redis.UniversalClient

func getRC(uri string) RedisClient {
	opt, err := redis.ParseURL(uri)
	if err != nil {
		log.Panic().Str("uri", uri).Err(err)
	}
	rcu := redis.NewClient(opt)
	pingStatus := rcu.Ping(context.Background())
	if err = pingStatus.Err(); err != nil {
		log.Panic().Str("host", opt.Addr).Int("db", opt.DB).Err(err)
	}

	return rcu
}

const (
	prefix = "dns-"
)

func getKey(name string, rtype uint16) string {
	return prefix + typeToString(rtype) + "-" + strings.TrimRight(name, ".")
}

func typeToString(rtype uint16) string {
	return strings.ToLower(dns.Type(rtype).String())
}

type Muxier interface {
	dns.Handler
	http.Handler

	Get(name string, rtype uint16) (dns.RR, error)
	Set(rr dns.RR, expiration time.Duration) error
}

type mux struct {
	rc RedisClient
}

func newMux(dsn string) Muxier {
	return &mux{getRC(dsn)}
}

func (h *mux) Get(name string, rtype uint16) (dns.RR, error) {
	key := getKey(name, rtype)
	res := h.rc.Get(context.Background(), key)
	s, err := res.Result()
	if err != nil {
		return nil, err
	}
	return dns.NewRR(s)
}

func (h *mux) Set(rr dns.RR, expiration time.Duration) error {
	key := getKey(rr.Header().Name, rr.Header().Rrtype)
	return h.rc.Set(context.Background(), key, rr.String(), expiration).Err()
}

func (h *mux) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)
	switch r.Opcode {
	case dns.OpcodeQuery:
		msg.Authoritative = true
		for _, q := range msg.Question {
			rr, err := h.Get(q.Name, q.Qtype)
			if err == nil {
				msg.Answer = append(msg.Answer, rr)
			} else {
				log.Info().Err(err).Msg("query dns")
			}
		}

	}
	if err := w.WriteMsg(&msg); err != nil {
		log.Warn().Err(err).Send()
	}
}

func (h *mux) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// TODO: put a new A
}

func newA(name, ip string, ttl uint) *dns.A {
	a := &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(ttl)},
		A:   net.ParseIP(ip),
	}
	return a
}

var (
	version = "dev"
)

func init() {
	zerolog.TimestampFieldName = "t"
	zerolog.LevelFieldName = "l"
	zerolog.MessageFieldName = "m"
	if version == "dev" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}
func main() {
	var (
		port int
		dsn  string
		name string
		ip   string
		ttl  uint
		days uint
		serv bool
	)
	flag.IntVar(&port, "port", 1353, "listen port")
	flag.StringVar(&dsn, "dsn", envOr("CIDNS_REDIS_DSN", "redis://localhost:6379/0"), "redis connection string")
	flag.StringVar(&name, "name", "", "host for add")
	flag.StringVar(&ip, "ip", "", "ip for add")
	flag.UintVar(&ttl, "ttl", 60, "time to live of renew cache")
	flag.UintVar(&days, "days", 7, "expire in some days")
	flag.BoolVar(&serv, "serv", false, "run as dns server")
	flag.Parse()

	log.Info().Str("ver", version).Int("port", port).Msg("starting")
	if serv {
		srv := &dns.Server{Addr: ":" + strconv.Itoa(port), Net: "udp"}
		srv.Handler = newMux(dsn)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal().Err(err).Msg("fail to udp listen")
		}
	} else if len(name) == 0 || len(ip) == 0 {
		flag.Usage()
	} else {
		h := newMux(dsn)
		expiration := time.Hour * 24 * time.Duration(days)
		err := h.Set(newA(name, ip, ttl), expiration)
		if err != nil {
			log.Error().Err(err).Msg("add record fail")
		} else {
			log.Info().Str("name", name).Str("ip", ip).Msg("add record ok")
		}
	}
}

func envOr(key, dft string) string {
	v := os.Getenv(key)
	if v == "" {
		return dft
	}
	return v
}
