package main

import (
	"context"
	"encoding/json"
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
	prefix  = "dns-"
	dftDays = 7
)

func getKey(name string, rtype uint16) string {
	return prefix + typeToString(rtype) + "-" + strings.ToLower(strings.TrimRight(name, "."))
}

func typeToString(rtype uint16) string {
	return strings.ToLower(dns.Type(rtype).String())
}

type Muxier interface {
	dns.Handler
	http.Handler

	Get(name string, rtype uint16) (dns.RR, error)
	Set(rr dns.RR, expiration time.Duration) error
	Del(name string, rtype uint16) error
	// update(rr dns.RR, q *dns.Question) error
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
	val := rr.String()
	log.Info().Str("key", key).Str("val", val).Msg("set")
	return h.rc.Set(context.Background(), key, val, expiration).Err()
}
func (h *mux) Del(name string, rtype uint16) error {
	key := getKey(name, rtype)
	err := h.rc.Del(context.Background(), key).Err()
	if err != nil {
		log.Warn().Err(err).Str("name", name).Uint16("rtyp", rtype).Msg("del fail")
		return err
	}
	return nil
}

func (h *mux) update(rr dns.RR, q *dns.Question) error {
	hdr := rr.Header()
	log.Info().Str("rr", rr.String()).
		Str("quest", q.String()).Uint16("class", hdr.Class).
		Msg("update")
	if hdr.Class == dns.ClassANY && hdr.Rdlength == 0 { // delete record
		return h.Del(hdr.Name, hdr.Rrtype)
	}
	// add record
	expiration := time.Hour * 24 * time.Duration(dftDays)
	return h.Set(rr, expiration)
}

func (h *mux) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	// log.Info().Int("opcode", r.Opcode).Str("msg", r.String()).Send()
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
				log.Info().Err(err).
					Uint16("qtyp", q.Qtype).
					Str("name", q.Name).
					Msg("query dns")
			}
		}
	case dns.OpcodeUpdate:
		msg.Authoritative = true
		for _, q := range r.Question {
			log.Info().Str("quest", q.String()).Send()
			for _, rr := range r.Ns {
				if err := h.update(rr, &q); err != nil {
					log.Info().Err(err).
						Str("qtyp", dns.Type(q.Qtype).String()).
						Str("name", q.Name).
						Msg("update dns")
				}
			}
		}

	}
	if err := w.WriteMsg(&msg); err != nil {
		log.Warn().Err(err).Send()
	}
}

type record struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

func (h *mux) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodHead || req.Method == http.MethodGet {
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	if req.Method != http.MethodPut {
		http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	var records []record
	err := json.NewDecoder(req.Body).Decode(&records)
	if err != nil {
		log.Info().Err(err).Msg("decode json fail")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	expiration := time.Hour * 24 * time.Duration(dftDays)
	ttl := uint(60)
	for _, rec := range records {
		err := h.Set(NewA(rec.Name, rec.IP, ttl), expiration)
		if err != nil {
			log.Info().Err(err).Str("name", rec.Name).Str("ip", rec.IP).Msg("fail")
		} else {
			log.Info().Str("name", rec.Name).Str("ip", rec.IP).Msg("ok")
		}
	}
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.Write([]byte("ok\n"))
}

func NewA(name, ip string, ttl uint) *dns.A {
	a := &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(ttl)},
		A:   net.ParseIP(ip),
	}
	return a
}

func init() {
	dns.DefaultMsgAcceptFunc = defaultMsgAcceptFunc
}

const (
	_QR = 1 << 15 // query/response (response=1)
)

// custom for github.com/miekg/dns/acceptfunc.go
func defaultMsgAcceptFunc(dh dns.Header) dns.MsgAcceptAction {
	if isResponse := dh.Bits&_QR != 0; isResponse {
		return dns.MsgIgnore
	}

	// Don't allow dynamic updates, because then the sections can contain a whole bunch of RRs.
	opcode := int(dh.Bits>>11) & 0xF
	if opcode != dns.OpcodeQuery && opcode != dns.OpcodeNotify && opcode != dns.OpcodeUpdate {
		return dns.MsgRejectNotImplemented
	}
	// log.Info().
	// 	Uint16("bits", dh.Bits).
	// 	Int("opcode", opcode).
	// 	Uint16("qdcount", dh.Qdcount).
	// 	Uint16("ancount", dh.Ancount).
	// 	Uint16("nscount", dh.Nscount).
	// 	Uint16("arcount", dh.Arcount).
	// 	Send()

	if dh.Qdcount != 1 {
		return dns.MsgReject
	}
	// NOTIFY requests can have a SOA in the ANSWER section. See RFC 1996 Section 3.7 and 3.11.
	if dh.Ancount > 1 {
		return dns.MsgReject
	}
	// IXFR request could have one SOA RR in the NS section. See RFC 1995, section 3.
	if dh.Nscount > 1 {
		return dns.MsgReject
	}
	if dh.Arcount > 2 {
		return dns.MsgReject
	}
	return dns.MsgAccept
}
