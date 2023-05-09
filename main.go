package main

import (
	"context"
	"flag"
	"net"
	"os"
	"strconv"

	"github.com/miekg/dns"
	redis "github.com/redis/go-redis/v9"
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
	prefix = "dns-a-"
)

func getKey(name string) string {
	return prefix + name
}

type handler struct {
	rc RedisClient
}

func (h *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)
	switch r.Question[0].Qtype {
	case dns.TypeA:
		msg.Authoritative = true
		domain := msg.Question[0].Name
		key := getKey(domain)
		res := h.rc.Get(context.Background(), key)
		address, err := res.Result()
		if err == nil {
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(address),
			})
		} else {
			log.Info().Err(err).Msg("query dns")
		}
	}
	w.WriteMsg(&msg)
}

var (
	version = "dev"
)

func main() {
	var port int
	var dsn string
	flag.IntVar(&port, "port", 1353, "listen port")
	flag.StringVar(&dsn, "dsn", envOr("CIDNS_REDIS_DSN", "redis://localhost:6379/0"), "redis connection string")
	srv := &dns.Server{Addr: ":" + strconv.Itoa(port), Net: "udp"}
	srv.Handler = &handler{getRC(dsn)}
	log.Info().Str("version", version).Msg("starting")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal().Err(err).Msg("fail to udp listen")
	}
}

func envOr(key, dft string) string {
	v := os.Getenv(key)
	if v == "" {
		return dft
	}
	return v
}
