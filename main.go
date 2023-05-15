package main

import (
	"flag"
	"os"
	"strconv"
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

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
	flag.StringVar(&dsn, "dsn", envOr("CICADA_REDIS_DSN", "redis://localhost:6379/0"), "redis connection string")
	flag.StringVar(&name, "name", "", "host for add")
	flag.StringVar(&ip, "ip", "", "ip for add")
	flag.UintVar(&ttl, "ttl", 60, "time to live of renew cache")
	flag.UintVar(&days, "days", dftDays, "expire in some days")
	flag.BoolVar(&serv, "serv", false, "run as dns server")
	flag.Parse()

	log.Info().Str("ver", version).Int("port", port).Msg("starting")
	if serv {
		srv := &dns.Server{Addr: ":" + strconv.Itoa(port), Net: "udp"}
		srv.Handler = NewMux(dsn)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal().Err(err).Msg("fail to udp listen")
		}
	} else if len(name) == 0 || len(ip) == 0 {
		flag.Usage()
	} else {
		h := NewMux(dsn)
		expiration := time.Hour * 24 * time.Duration(days)
		err := h.Set(NewA(name, ip, ttl), expiration)
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
