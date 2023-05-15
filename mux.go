package main

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/redis/go-redis/v9"
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

func NewMux(dsn string) Muxier {
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

func NewA(name, ip string, ttl uint) *dns.A {
	a := &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(ttl)},
		A:   net.ParseIP(ip),
	}
	return a
}
