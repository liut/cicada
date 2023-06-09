package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
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

type Config struct {
	port int
	ttl  uint
	days uint
	serv bool
	net  string
	dsn  string
	name string
	ip   string
}

func main() {
	var cfg Config
	flag.IntVar(&cfg.port, "port", 1353, "listen port")
	flag.StringVar(&cfg.dsn, "dsn", envOr("CICADA_REDIS_DSN", "redis://localhost:6379/0"), "redis connection string")
	flag.StringVar(&cfg.name, "name", "", "host for add")
	flag.StringVar(&cfg.ip, "ip", "", "ip for add")
	flag.UintVar(&cfg.ttl, "ttl", 60, "time to live of renew cache")
	flag.UintVar(&cfg.days, "days", dftDays, "expire in some days")
	flag.BoolVar(&cfg.serv, "serv", false, "run as dns server")
	flag.StringVar(&cfg.net, "net", "udp", "network: udp|tcp")
	flag.Parse()

	log.Info().Str("ver", version).
		Str("net", cfg.net).Int("port", cfg.port).
		Msg("starting")
	if cfg.serv {
		mux := NewMux(cfg.dsn)
		srv := &dns.Server{
			Addr:    ":" + strconv.Itoa(cfg.port),
			Net:     cfg.net,
			Handler: mux,
		}
		hs := &http.Server{
			Addr:           ":" + strconv.Itoa(cfg.port+1),
			Handler:        mux,
			ReadTimeout:    6 * time.Second,
			WriteTimeout:   6 * time.Second,
			MaxHeaderBytes: 1 << 18,
		}
		go func() {
			if err := srv.ListenAndServe(); err != nil {
				log.Warn().Err(err).Msg("fail to dns listen")
			}
		}()
		go func() {
			if err := hs.ListenAndServe(); err != nil {
				log.Warn().Err(err).Msg("fail to http listen")
			}
		}()
		sig := make(chan os.Signal, 2)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		s := <-sig
		go srv.Shutdown()                    //nolint
		go hs.Shutdown(context.Background()) //nolint
		log.Info().Msgf("signal (%d) received, stopping ", s)
		<-time.After(time.Second * 2)

	} else if len(cfg.name) == 0 || len(cfg.ip) == 0 {
		flag.Usage()
	} else {
		h := NewMux(cfg.dsn)
		expiration := time.Hour * 24 * time.Duration(cfg.days)
		err := h.Set(NewA(cfg.name, cfg.ip, cfg.ttl), expiration)
		if err != nil {
			log.Error().Err(err).Msg("add record fail")
		} else {
			log.Info().Str("name", cfg.name).Str("ip", cfg.ip).Msg("add record ok")
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
